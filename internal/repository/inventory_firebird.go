package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/aweh-pos/gateway/internal/middleware"
	"github.com/aweh-pos/gateway/internal/models"
	"github.com/jmoiron/sqlx"
)

// ─── Repository struct ────────────────────────────────────────────────────────

// InventoryFirebird implements InventoryRepository against Firebird 3.0 (DMASTER).
type InventoryFirebird struct {
	BaseRepository
}

func NewInventoryRepository(tm *TenantManager) *InventoryFirebird {
	return &InventoryFirebird{BaseRepository{TM: tm}}
}

// ─── Full column list for DMASTER SELECT ─────────────────────────────────────

const inventorySelectCols = `
	ITEMPARTNO, MPARTNO, BARCODE,
	SUPPLIERNO, DESCRIPTION, BRAND, BIN, CATEGORYNO, COSTCATEGORY, COSTGROUP,
	CATOGORY, INVFORM, PACKAGING, ZONEID,
	PACK, PACKUNIT, PACKCOST, EACHCOST, UNITS, EACHUNIT, UNITCOST,
	MARKUP, SELLINGPRICE, BULKSELLINGPRICE, TAXRATE, DISCOUNT, BUCOS,
	MINSTOCKLEVEL, MAXSTOCKLEVEL, REORDERLEVE, ONORDER, AUTOYIELD, WEIGHT, TARE,
	FRONTOPENINGSTOCK, BACKOPENINGSTOCK, FRONTCLOSINGSTOCK, BACKCLOSINGSTOCK,
	PURCHASES, SALES, GROUP_ID, UOM, ITEM_IMAGE,
	CASE
		WHEN UPPER(TRIM(COALESCE(CAST(IS_BASE_VARIANT AS VARCHAR(20)), ''))) IN ('1', 'T', 'TRUE', 'Y', 'YES') THEN TRUE
		ELSE FALSE
	END AS IS_BASE_VARIANT,
	CASE
		WHEN UPPER(TRIM(COALESCE(CAST(IS_SELLABLE AS VARCHAR(20)), ''))) IN ('1', 'T', 'TRUE', 'Y', 'YES') THEN TRUE
		ELSE FALSE
	END AS IS_SELLABLE,
	CASE
		WHEN UPPER(TRIM(COALESCE(CAST(PURCHASES AS VARCHAR(20)), ''))) IN ('1', 'T', 'TRUE', 'Y', 'YES') THEN TRUE
		ELSE FALSE
	END AS IS_ORDERING_ALLOWED`

// List endpoint projection — same as full select for now (can optimize later)
const inventoryListSelectCols = `
	ITEMPARTNO, MPARTNO, BARCODE,
	SUPPLIERNO, DESCRIPTION, BRAND, BIN, CATEGORYNO, COSTCATEGORY, COSTGROUP,
	CATOGORY, INVFORM, PACKAGING, ZONEID,
	PACK, PACKUNIT, PACKCOST, EACHCOST, UNITS, EACHUNIT, UNITCOST,
	MARKUP, SELLINGPRICE, BULKSELLINGPRICE, TAXRATE, DISCOUNT, BUCOS,
	MINSTOCKLEVEL, MAXSTOCKLEVEL, REORDERLEVE, ONORDER, AUTOYIELD, WEIGHT, TARE,
	FRONTOPENINGSTOCK, BACKOPENINGSTOCK, FRONTCLOSINGSTOCK, BACKCLOSINGSTOCK,
	PURCHASES, SALES, GROUP_ID, UOM, ITEM_IMAGE,
	CASE
		WHEN UPPER(TRIM(COALESCE(CAST(IS_BASE_VARIANT AS VARCHAR(20)), ''))) IN ('1', 'T', 'TRUE', 'Y', 'YES') THEN TRUE
		ELSE FALSE
	END AS IS_BASE_VARIANT,
	CASE
		WHEN UPPER(TRIM(COALESCE(CAST(IS_SELLABLE AS VARCHAR(20)), ''))) IN ('1', 'T', 'TRUE', 'Y', 'YES') THEN TRUE
		ELSE FALSE
	END AS IS_SELLABLE,
	CASE
		WHEN UPPER(TRIM(COALESCE(CAST(PURCHASES AS VARCHAR(20)), ''))) IN ('1', 'T', 'TRUE', 'Y', 'YES') THEN TRUE
		ELSE FALSE
	END AS IS_ORDERING_ALLOWED`

// ─── Margin calculation helper (Go-side: no DB division) ─────────────────────
//
// Project Constitution Appendix A §4: Never use PACK as a divisor in Firebird SQL
// without NULLIF(PACK, 0). All margin computation happens in Go.

func calculateMargins(item models.InventoryItem) models.ComputedMargins {
	pack := item.Pack
	if pack <= 0 {
		pack = 1
	}
	units := normalizeUnits(item.Units)

	eachCost := item.EachCost
	if eachCost <= 0 {
		eachCost = item.PackCost / pack
	}
	unitCost := eachCost / units

	taxRate := item.TaxRate / 100
	var taxAmount, nettSelling float64
	if item.SellingPrice > 0 && taxRate > 0 {
		taxAmount = item.SellingPrice * taxRate / (1 + taxRate)
		nettSelling = item.SellingPrice - taxAmount
	} else {
		nettSelling = item.SellingPrice
	}

	grossProfit := item.SellingPrice - eachCost // before VAT
	nettProfit := nettSelling - eachCost        // after VAT

	var gpMarginPct float64
	if nettSelling > 0 {
		gpMarginPct = (nettProfit / nettSelling) * 100
	}

	recSellingPrice := eachCost * (1 + item.Markup/100)

	return models.ComputedMargins{
		EachCost:        round4dp(eachCost),
		UnitCost:        round4dp(unitCost),
		Tax:             round2dp(taxAmount),
		NettSelling:     round2dp(nettSelling),
		GrossProfit:     round2dp(grossProfit),
		NettProfit:      round2dp(nettProfit),
		GPMargin:        round2dp(gpMarginPct),
		RecSellingPrice: round2dp(recSellingPrice),
		AutoYield:       item.AutoYield == "T",
	}
}

func round2dp(v float64) float64 { return math.Round(v*100) / 100 }
func round4dp(v float64) float64 { return math.Round(v*10000) / 10000 }

func normalizeUnits(units float64) float64 {
	if units > 0 {
		return units
	}
	return 1
}

func deriveEachCostFromBase(baseEachCost float64, baseUnits float64, targetUnits float64) float64 {
	baseUnits = normalizeUnits(baseUnits)
	targetUnits = normalizeUnits(targetUnits)
	baseAtomicCost := baseEachCost / baseUnits
	return baseAtomicCost * targetUnits
}

// enrichItem populates Go-computed fields on an InventoryItem after DB scanning.
func enrichItem(item *models.InventoryItem) {
	m := calculateMargins(*item)
	item.GPMarginPct = m.GPMargin
	item.AutoYieldBool = m.AutoYield
}

// ─── ListItems ────────────────────────────────────────────────────────────────

// ListItems fetches a paginated, filtered list of inventory items from DMASTER.
// GP margin is computed in Go after scanning (not in SQL — see Project Constitution Appendix A §4).
func (r *InventoryFirebird) ListItems(ctx context.Context, filter models.ItemFilter) ([]models.InventoryItem, int, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Build dynamic WHERE clause — never accept raw column names from request.
	where, args := buildItemWhere(filter)

	// COUNT query
	countQ := "SELECT COUNT(*) FROM DMASTER " + where
	var total int
	if err := db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, fmt.Errorf("ListItems count: %w", err)
	}

	// Pagination defaults
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}

	// Sort column whitelist (never interpolate client strings directly)
	sortMap := map[string]string{
		"description":   "DESCRIPTION",
		"each_cost":     "EACHCOST",
		"selling_price": "SELLINGPRICE",
		"gp_margin_pct": "EACHCOST", // fallback: sort by cost as proxy; real margin computed in Go
	}
	orderCol := "DESCRIPTION"
	if col, ok := sortMap[filter.SortBy]; ok {
		orderCol = col
	}
	dir := "ASC"
	if strings.ToUpper(filter.SortDir) == "DESC" {
		dir = "DESC"
	}

	// Firebird 3.0 row-limiting syntax: ROWS start TO end (1-based, inclusive).
	// Project Constitution Appendix A §1: use ROWS not LIMIT/OFFSET.
	start := (filter.Page-1)*filter.Limit + 1
	end := filter.Page * filter.Limit

	dataQ := fmt.Sprintf(
		"SELECT %s FROM DMASTER %s ORDER BY %s %s ROWS %d TO %d",
		inventoryListSelectCols, where, orderCol, dir, start, end,
	)

	var items []models.InventoryItem
	if err := db.SelectContext(ctx, &items, dataQ, args...); err != nil {
		return nil, 0, fmt.Errorf("ListItems data: %w", err)
	}

	for i := range items {
		enrichItem(&items[i])
	}

	return items, total, nil
}

// buildItemWhere constructs the WHERE clause and positional args for ItemFilter.
func buildItemWhere(filter models.ItemFilter) (string, []interface{}) {
	clause := "WHERE 1=1"
	var args []interface{}
	// Soft delete: exclude deleted items (DELETED IS NULL or DELETED != 'Y')
	clause += " AND (DELETED IS NULL OR DELETED <> 'Y')"
	if filter.Search != "" {
		search := strings.TrimSpace(filter.Search)
		if search != "" {
			clause += ` AND (
				CAST(DESCRIPTION AS VARCHAR(255)) CONTAINING ?
				OR CAST(BARCODE AS VARCHAR(64)) CONTAINING ?
				OR CAST(MPARTNO AS VARCHAR(64)) CONTAINING ?
				OR CAST(SUPPLIERNO AS VARCHAR(64)) CONTAINING ?
			)`
			args = append(args, search, search, search, search)
		}
	}
	if filter.StockSheet != "" {
		clause += " AND INVFORM = ?"
		args = append(args, filter.StockSheet)
	}
	if filter.Category != "" {
		clause += " AND CATOGORY = ?"
		args = append(args, filter.Category)
	}
	if filter.Bin != "" {
		clause += " AND BIN = ?"
		args = append(args, filter.Bin)
	}
	if filter.SupplierNo != "" {
		clause += " AND SUPPLIERNO = ?"
		args = append(args, filter.SupplierNo)
	}
	return clause, args
}

// ─── GetItem ──────────────────────────────────────────────────────────────────

// GetItem returns a single DMASTER item with full ComputedMargins.
func (r *InventoryFirebird) GetItem(ctx context.Context, itemPartNo int64) (*models.InventoryItemDetail, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}
	item, err := fetchItemByID(ctx, db, itemPartNo)
	if err != nil {
		return nil, err
	}
	margins := calculateMargins(*item)
	return &models.InventoryItemDetail{InventoryItem: *item, Margins: margins}, nil
}

// fetchItemByID performs the raw SELECT of a single DMASTER row.
func fetchItemByID(ctx context.Context, db *sqlx.DB, itemPartNo int64) (*models.InventoryItem, error) {
	q := fmt.Sprintf("SELECT %s FROM DMASTER WHERE ITEMPARTNO = ? AND (DELETED IS NULL OR DELETED <> 'Y')", inventorySelectCols)
	var item models.InventoryItem
	if err := db.GetContext(ctx, &item, q, itemPartNo); err != nil {
		return nil, fmt.Errorf("item %d not found: %w", itemPartNo, err)
	}
	enrichItem(&item)
	return &item, nil
}

// ─── Guard checks ─────────────────────────────────────────────────────────────

// DescriptionExists checks whether another DMASTER row has the same DESCRIPTION.
func (r *InventoryFirebird) DescriptionExists(ctx context.Context, description string, excludeID int64) (bool, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return false, err
	}
	var count int
	if err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM DMASTER WHERE UPPER(DESCRIPTION) = UPPER(?) AND ITEMPARTNO <> ?", description, excludeID); err != nil {
		return false, fmt.Errorf("DescriptionExists: %w", err)
	}
	return count > 0, nil
}

// BarcodeExists checks whether a barcode is assigned to another DMASTER row.
func (r *InventoryFirebird) BarcodeExists(ctx context.Context, barcode string, excludeID int64) (bool, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return false, err
	}
	var count int
	if err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM DMASTER WHERE BARCODE = ? AND ITEMPARTNO <> ?", barcode, excludeID); err != nil {
		return false, fmt.Errorf("BarcodeExists: %w", err)
	}
	return count > 0, nil
}

// ItemExistsInMenu checks if the item (by MPARTNO) is linked to a POS menu button.
// Degrades gracefully: returns (false, nil) if the menu table does not yet exist.
// TODO: confirm exact table name from Module 3 (Menu Management) docs.
func (r *InventoryFirebird) ItemExistsInMenu(ctx context.Context, mPartNo string) (bool, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return false, err
	}
	var count int
	err = db.GetContext(ctx, &count, "SELECT COUNT(*) FROM MENUBTNS WHERE MPARTNO = ?", mPartNo)
	if err != nil {
		// Table may not exist in all deployments — degrade gracefully.
		return false, nil
	}
	return count > 0, nil
}

// ItemExistsInRecipe checks if the item (by MPARTNO) is used as a recipe component.
// Degrades gracefully: returns (false, nil) if the recipe table does not yet exist.
// TODO: confirm exact table name from Module 3 (Menu Management) docs.
func (r *InventoryFirebird) ItemExistsInRecipe(ctx context.Context, mPartNo string) (bool, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return false, err
	}
	var count int
	err = db.GetContext(ctx, &count, "SELECT COUNT(*) FROM RECIPE WHERE MPARTNO = ?", mPartNo)
	if err != nil {
		// Table may not exist in all deployments — degrade gracefully.
		return false, nil
	}
	return count > 0, nil
}

// ─── NextItemPartNo ───────────────────────────────────────────────────────────

// NextItemPartNo fetches the next ID from the Firebird ItempartnoGen generator.
// Project Constitution Appendix A §2: always call GEN_ID on RDB$DATABASE.
func (r *InventoryFirebird) NextItemPartNo(ctx context.Context) (int64, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, err
	}
	var nextID int64
	if err := db.GetContext(ctx, &nextID, "SELECT GEN_ID(ItempartnoGen, 1) FROM RDB$DATABASE"); err != nil {
		return 0, fmt.Errorf("NextItemPartNo: %w", err)
	}
	return nextID, nil
}

// ─── CreateItem ───────────────────────────────────────────────────────────────
//
// Mandatory DB Sequence (Project Constitution §4):
//  1. BeginTxx
//  2. GEN_ID(ItempartnoGen, 1) inside tx
//  3. Go business logic (validate, compute each_cost / unit_cost)
//  4. INSERT INTO DMASTER
//  5. Commit

func (r *InventoryFirebird) CreateItem(ctx context.Context, req models.CreateItemRequest) (*models.InventoryItem, error) {
	if req.PackSize <= 0 {
		return nil, errors.New("ERR_VALIDATION: pack_size must be greater than 0")
	}
	if req.Description == "" {
		return nil, errors.New("ERR_VALIDATION: description is required")
	}

	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	// Check duplicate description
	dup, err := r.DescriptionExists(ctx, req.Description, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, fmt.Errorf("ERR_DUPLICATE_DESCRIPTION: An item with the description '%s' already exists", req.Description)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateItem BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Step 2: Fetch next ID inside transaction
	var newID int64
	if err := tx.GetContext(ctx, &newID, "SELECT GEN_ID(ItempartnoGen, 1) FROM RDB$DATABASE"); err != nil {
		return nil, fmt.Errorf("CreateItem GEN_ID: %w", err)
	}

	// Step 3: Go business logic — compute derived cost fields (units-first, pack fallback for legacy payloads)
	eachCost := req.EachCost
	if eachCost <= 0 && req.PackSize > 0 {
		eachCost = req.PackCost / req.PackSize
	}
	unitsPerPack := req.UnitsPerPack
	if unitsPerPack <= 0 {
		unitsPerPack = 1
	}
	unitCost := eachCost / unitsPerPack
	orderingAllowed := true
	if req.OrderingAllowed != nil {
		orderingAllowed = *req.OrderingAllowed
	}
	mPartNo := strconv.FormatInt(newID, 10)
	ayStr := "F"
	if req.AutoYield {
		ayStr = "T"
	}

	// Step 4: INSERT
	const insertQ = `INSERT INTO DMASTER (
		ITEMPARTNO, MPARTNO, SUPPLIERNO, DESCRIPTION, CATOGORY, BIN, INVFORM,
		COSTCATEGORY, PACK, PACKUNIT, PACKCOST, EACHCOST, UNITS, EACHUNIT, UNITCOST,
		SELLINGPRICE, TAXRATE, MARKUP, MINSTOCKLEVEL, MAXSTOCKLEVEL, REORDERLEVE,
		AUTOYIELD, BULKSELLINGPRICE, DISCOUNT, BUCOS, ONORDER, WEIGHT, TARE,
		FRONTOPENINGSTOCK, BACKOPENINGSTOCK, FRONTCLOSINGSTOCK, BACKCLOSINGSTOCK,
		PURCHASES, SALES, IS_ORDERING_ALLOWED
	) VALUES (
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?,
		0, 0, 0, 0,
		0, 0, ?
	)`

	if _, err := tx.ExecContext(ctx, insertQ,
		newID, mPartNo, req.SupplierNo, req.Description, req.Category, req.Bin, req.StockSheet,
		req.CostCategory, req.PackSize, req.PackUnit, req.PackCost, eachCost, unitsPerPack, req.EachUnit, unitCost,
		req.SellingPrice, req.TaxRate, req.Markup, req.MinStockLevel, req.MaxStockLevel, req.ReorderLevel,
		ayStr, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0,
		orderingAllowed,
	); err != nil {
		return nil, fmt.Errorf("CreateItem INSERT: %w", err)
	}

	// Step 5: Commit
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("CreateItem Commit: %w", err)
	}

	// Return persisted item via fresh SELECT (outside tx)
	item, err := fetchItemByID(ctx, db, newID)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// ─── UpdateItem ───────────────────────────────────────────────────────────────

// UpdateItem builds a dynamic SET clause from non-nil fields and executes a full UPDATE.
// Wrapped in BeginTxx/Commit per Project Constitution §4.
func (r *InventoryFirebird) UpdateItem(ctx context.Context, itemPartNo int64, req models.UpdateItemRequest) (*models.InventoryItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("UpdateItem BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var currentUnits float64
	var currentEachCost float64
	var groupID sql.NullInt64
	err = tx.QueryRowxContext(
		ctx,
		"SELECT UNITS, EACHCOST, GROUP_ID FROM DMASTER WHERE ITEMPARTNO = ?",
		itemPartNo,
	).Scan(&currentUnits, &currentEachCost, &groupID)
	if err != nil {
		return nil, fmt.Errorf("ERR_NOT_FOUND: Inventory item %d not found", itemPartNo)
	}

	var setClauses []string
	var args []interface{}

	targetUnits := normalizeUnits(currentUnits)
	if req.UnitsPerPack != nil {
		targetUnits = normalizeUnits(*req.UnitsPerPack)
	}

	targetEachCost := currentEachCost
	if req.EachCost != nil {
		targetEachCost = *req.EachCost
	}

	// Group-aware cost conversion: always anchor to base variant
	var baseItemPartNoForSync int64
	var needsGroupResync bool
	if groupID.Valid && groupID.Int64 > 0 && req.EachCost != nil {
		var baseItemPartNo int64
		if err := tx.GetContext(ctx, &baseItemPartNo, "SELECT BASE_ITEMPARTNO FROM INVENTORY_GROUPS WHERE GROUP_ID = ?", groupID.Int64); err == nil {
			var baseEachCost float64
			var baseUnits float64
			if err := tx.QueryRowxContext(ctx, "SELECT EACHCOST, UNITS FROM DMASTER WHERE ITEMPARTNO = ?", baseItemPartNo).Scan(&baseEachCost, &baseUnits); err == nil {
				if baseItemPartNo == itemPartNo {
					// Editing the base variant directly — use new cost as-is
					// Flag to sync all other variants
					baseItemPartNoForSync = baseItemPartNo
					needsGroupResync = true
				} else {
					// Editing a variant (e.g., case) — work backwards to get base cost
					// Formula: base_cost = (variant_cost × base_units) / variant_units
					derivedBaseCost := round4dp((targetEachCost * baseUnits) / targetUnits)
					
					// Update the base item first
					_, err := tx.ExecContext(
						ctx,
						"UPDATE DMASTER SET EACHCOST = ?, UNITCOST = ? WHERE ITEMPARTNO = ?",
						derivedBaseCost,
						round4dp(derivedBaseCost/baseUnits),
						baseItemPartNo,
					)
					if err != nil {
						return nil, fmt.Errorf("UpdateItem backward base sync: %w", err)
					}
					
					// Now derive this variant's cost from the updated base
					targetEachCost = deriveEachCostFromBase(derivedBaseCost, baseUnits, targetUnits)
					
					// Flag to sync all other variants
					baseItemPartNoForSync = baseItemPartNo
					needsGroupResync = true
				}
			}
		}
	}

	if req.Description != nil {
		setClauses = append(setClauses, "DESCRIPTION = ?")
		args = append(args, *req.Description)
	}
	if req.Barcode != nil {
		setClauses = append(setClauses, "BARCODE = ?")
		args = append(args, *req.Barcode)
	}
	if req.SupplierNo != nil {
		setClauses = append(setClauses, "SUPPLIERNO = ?")
		args = append(args, *req.SupplierNo)
	}
	if req.Category != nil {
		setClauses = append(setClauses, "CATOGORY = ?")
		args = append(args, *req.Category)
	}
	if req.Bin != nil {
		setClauses = append(setClauses, "BIN = ?")
		args = append(args, *req.Bin)
	}
	if req.StockSheet != nil {
		setClauses = append(setClauses, "INVFORM = ?")
		args = append(args, *req.StockSheet)
	}
	if req.CostCategory != nil {
		setClauses = append(setClauses, "COSTCATEGORY = ?")
		args = append(args, *req.CostCategory)
	}
	if req.ItemImage != nil {
		setClauses = append(setClauses, "ITEM_IMAGE = ?")
		trimmed := strings.TrimSpace(*req.ItemImage)
		if trimmed == "" {
			args = append(args, nil)
		} else {
			args = append(args, trimmed)
		}
	}
	if req.UOM != nil {
		setClauses = append(setClauses, "UOM = ?")
		args = append(args, normalizeUOM(*req.UOM))
	}
	if req.IsSellable != nil {
		setClauses = append(setClauses, "IS_SELLABLE = ?")
		args = append(args, *req.IsSellable)
	}
	if req.OrderingAllowed != nil {
		setClauses = append(setClauses, "IS_ORDERING_ALLOWED = ?")
		args = append(args, *req.OrderingAllowed)
	}
	if req.Brand != nil {
		setClauses = append(setClauses, "BRAND = ?")
		args = append(args, *req.Brand)
	}
	if req.Packaging != nil {
		setClauses = append(setClauses, "PACKAGING = ?")
		args = append(args, *req.Packaging)
	}
	if req.PackSize != nil {
		setClauses = append(setClauses, "PACK = ?")
		args = append(args, *req.PackSize)
	}
	if req.PackUnit != nil {
		setClauses = append(setClauses, "PACKUNIT = ?")
		args = append(args, *req.PackUnit)
	}
	if req.PackCost != nil {
		setClauses = append(setClauses, "PACKCOST = ?")
		args = append(args, *req.PackCost)
	}
	if req.EachCost != nil {
		setClauses = append(setClauses, "EACHCOST = ?")
		args = append(args, *req.EachCost)
	}
	if req.UnitsPerPack != nil {
		setClauses = append(setClauses, "UNITS = ?")
		args = append(args, targetUnits)
	}
	if req.EachUnit != nil {
		setClauses = append(setClauses, "EACHUNIT = ?")
		args = append(args, *req.EachUnit)
	}
	if req.SellingPrice != nil {
		setClauses = append(setClauses, "SELLINGPRICE = ?")
		args = append(args, *req.SellingPrice)
	}
	if req.BulkSellingPrice != nil {
		setClauses = append(setClauses, "BULKSELLINGPRICE = ?")
		args = append(args, *req.BulkSellingPrice)
	}
	if req.TaxRate != nil {
		setClauses = append(setClauses, "TAXRATE = ?")
		args = append(args, *req.TaxRate)
	}
	if req.Markup != nil {
		setClauses = append(setClauses, "MARKUP = ?")
		args = append(args, *req.Markup)
	}
	if req.Discount != nil {
		setClauses = append(setClauses, "DISCOUNT = ?")
		args = append(args, *req.Discount)
	}
	if req.MinStockLevel != nil {
		setClauses = append(setClauses, "MINSTOCKLEVEL = ?")
		args = append(args, *req.MinStockLevel)
	}
	if req.MaxStockLevel != nil {
		setClauses = append(setClauses, "MAXSTOCKLEVEL = ?")
		args = append(args, *req.MaxStockLevel)
	}
	if req.ReorderLevel != nil {
		setClauses = append(setClauses, "REORDERLEVE = ?")
		args = append(args, *req.ReorderLevel)
	}
	if req.AutoYield != nil {
		ay := "F"
		if *req.AutoYield {
			ay = "T"
		}
		setClauses = append(setClauses, "AUTOYIELD = ?")
		args = append(args, ay)
	}

	if req.EachCost != nil || req.UnitsPerPack != nil || (groupID.Valid && groupID.Int64 > 0) {
		setClauses = append(setClauses, "EACHCOST = ?")
		args = append(args, round4dp(targetEachCost))

		setClauses = append(setClauses, "UNITCOST = ?")
		args = append(args, round4dp(targetEachCost/targetUnits))
	}

	if len(setClauses) == 0 {
		// Nothing to update — return current state
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("UpdateItem Commit: %w", err)
		}
		return fetchItemByID(ctx, db, itemPartNo)
	}

	args = append(args, itemPartNo) // WHERE ITEMPARTNO = ?

	query := "UPDATE DMASTER SET " + strings.Join(setClauses, ", ") + " WHERE ITEMPARTNO = ?"

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("UpdateItem EXEC: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("ERR_NOT_FOUND: Inventory item %d not found", itemPartNo)
	}

	// Sync all other variants in the group if cost changed
	if needsGroupResync && baseItemPartNoForSync > 0 {
		// Fetch current base cost/units (may have been updated in backward sync)
		var baseEachCost float64
		var baseUnits float64
		if err := tx.QueryRowxContext(ctx, "SELECT EACHCOST, UNITS FROM DMASTER WHERE ITEMPARTNO = ?", baseItemPartNoForSync).Scan(&baseEachCost, &baseUnits); err != nil {
			return nil, fmt.Errorf("UpdateItem fetch base for resync: %w", err)
		}

		type groupVariantUnits struct {
			ItemPartNo int64   `db:"ITEMPARTNO"`
			Units      float64 `db:"UNITS"`
		}

		var allVariants []groupVariantUnits
		if err := tx.SelectContext(
			ctx,
			&allVariants,
			"SELECT ITEMPARTNO, UNITS FROM DMASTER WHERE GROUP_ID = ? AND ITEMPARTNO <> ?",
			groupID.Int64,
			baseItemPartNoForSync,
		); err != nil {
			return nil, fmt.Errorf("UpdateItem group variant fetch: %w", err)
		}

		for _, variant := range allVariants {
			variantUnits := normalizeUnits(variant.Units)
			variantEachCost := round4dp(deriveEachCostFromBase(baseEachCost, baseUnits, variantUnits))
			variantUnitCost := round4dp(variantEachCost / variantUnits)
			if _, err := tx.ExecContext(
				ctx,
				"UPDATE DMASTER SET EACHCOST = ?, UNITCOST = ? WHERE ITEMPARTNO = ?",
				variantEachCost,
				variantUnitCost,
				variant.ItemPartNo,
			); err != nil {
				return nil, fmt.Errorf("UpdateItem group variant sync: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("UpdateItem Commit: %w", err)
	}

	return fetchItemByID(ctx, db, itemPartNo)
}

// ─── DeleteItem ───────────────────────────────────────────────────────────────

// DeleteItem performs guard checks then SOFT DELETES the DMASTER row (UPDATE, not DELETE).
// Returns errors with ERR_ITEM_IN_MENU or ERR_ITEM_IN_RECIPE prefixes for 409 handling.
func (r *InventoryFirebird) DeleteItem(ctx context.Context, itemPartNo int64) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return err
	}

	// Fetch mPartNo for guard checks
	var mPartNo string
	if err := db.GetContext(ctx, &mPartNo, "SELECT MPARTNO FROM DMASTER WHERE ITEMPARTNO = ? AND (DELETED IS NULL OR DELETED <> 'Y')", itemPartNo); err != nil {
		return fmt.Errorf("ERR_NOT_FOUND: Inventory item %d not found or already deleted", itemPartNo)
	}

	inMenu, err := r.ItemExistsInMenu(ctx, mPartNo)
	if err != nil {
		return fmt.Errorf("DeleteItem guard check: %w", err)
	}
	if inMenu {
		return errors.New("ERR_ITEM_IN_MENU: This item is linked to POS menu button(s). Remove the menu link before deleting")
	}

	inRecipe, err := r.ItemExistsInRecipe(ctx, mPartNo)
	if err != nil {
		return fmt.Errorf("DeleteItem guard check: %w", err)
	}
	if inRecipe {
		return errors.New("ERR_ITEM_IN_RECIPE: This item is used as a component in one or more recipes. Remove the recipe link before deleting")
	}

	// Get username from context for audit trail
	username := "system"
	if u, err := middleware.GetUsername(ctx); err == nil && u != "" {
		username = u
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("DeleteItem BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Soft delete: UPDATE instead of DELETE
	if _, err := tx.ExecContext(ctx,
		"UPDATE DMASTER SET DELETED = 'Y', DELETED_DATE = CURRENT_TIMESTAMP, DELETED_BY = ? WHERE ITEMPARTNO = ?",
		username, itemPartNo); err != nil {
		return fmt.Errorf("DeleteItem UPDATE: %w", err)
	}

	return tx.Commit()
}

// ─── CloneItem ────────────────────────────────────────────────────────────────
//
// Mandatory DB Sequence (Project Constitution §4):
//  1. Fetch source via GetItem (outside tx — read-only)
//  2. BeginTxx
//  3. GEN_ID(ItempartnoGen, 1) inside tx
//  4. INSERT DMASTER copy with new ITEMPARTNO, new DESCRIPTION, BARCODE=NULL, EACHCOST=1, UNITCOST=1
//  5. Commit

func (r *InventoryFirebird) CloneItem(ctx context.Context, sourceID int64, newDescription string) (*models.InventoryItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	// Step 1: Fetch source (outside tx)
	src, err := fetchItemByID(ctx, db, sourceID)
	if err != nil {
		return nil, fmt.Errorf("CloneItem source: %w", err)
	}

	// Check duplicate description
	dup, err := r.DescriptionExists(ctx, newDescription, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, fmt.Errorf("ERR_DUPLICATE_DESCRIPTION: An item with the description '%s' already exists", newDescription)
	}

	// Step 2 + 3: Begin tx and fetch new ID
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("CloneItem BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var newID int64
	if err := tx.GetContext(ctx, &newID, "SELECT GEN_ID(ItempartnoGen, 1) FROM RDB$DATABASE"); err != nil {
		return nil, fmt.Errorf("CloneItem GEN_ID: %w", err)
	}

	newMPartNo := strconv.FormatInt(newID, 10)
	category := ""
	if src.Category.Valid {
		category = src.Category.String
	}
	invForm := ""
	if src.InvForm.Valid {
		invForm = src.InvForm.String
	}
	costCategory := ""
	if src.CostCategory.Valid {
		costCategory = src.CostCategory.String
	}

	// Step 4: INSERT copy — BARCODE=NULL, EACHCOST=1, UNITCOST=1 per plan
	const cloneQ = `INSERT INTO DMASTER (
		ITEMPARTNO, MPARTNO, SUPPLIERNO, DESCRIPTION, CATOGORY, BIN, INVFORM,
		COSTCATEGORY, PACK, PACKUNIT, PACKCOST, EACHCOST, UNITS, EACHUNIT, UNITCOST,
		SELLINGPRICE, TAXRATE, MARKUP, MINSTOCKLEVEL, MAXSTOCKLEVEL, REORDERLEVE,
		AUTOYIELD, BULKSELLINGPRICE, DISCOUNT, BUCOS, ONORDER, WEIGHT, TARE,
		ITEM_IMAGE,
		FRONTOPENINGSTOCK, BACKOPENINGSTOCK, FRONTCLOSINGSTOCK, BACKCLOSINGSTOCK,
		PURCHASES, SALES, IS_ORDERING_ALLOWED
	) VALUES (
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, 1, ?, ?, 1,
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?,
		?,
		0, 0, 0, 0,
		0, 0, ?
	)`

	if _, err := tx.ExecContext(ctx, cloneQ,
		newID, newMPartNo, src.SupplierNo, newDescription, category, src.Bin, invForm,
		costCategory, src.Pack, src.PackUnit, src.PackCost, src.Units, src.EachUnit,
		src.SellingPrice, src.TaxRate, src.Markup, src.MinStockLevel, src.MaxStockLevel, src.ReorderLevel,
		src.AutoYield, src.BulkSellingPrice, src.Discount, src.Bucos, src.OnOrder, src.Weight, src.Tare,
		src.ItemImage,
		src.IsOrderingAllowed,
	); err != nil {
		return nil, fmt.Errorf("CloneItem INSERT: %w", err)
	}

	// Step 5: Commit
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("CloneItem Commit: %w", err)
	}

	return fetchItemByID(ctx, db, newID)
}

// ─── AssignBarcode ────────────────────────────────────────────────────────────

// AssignBarcode sets BARCODE and COSTGROUP='DIR' on a DMASTER row.
func (r *InventoryFirebird) AssignBarcode(ctx context.Context, itemPartNo int64, barcode string) error {
	dup, err := r.BarcodeExists(ctx, barcode, itemPartNo)
	if err != nil {
		return err
	}
	if dup {
		return fmt.Errorf("ERR_DUPLICATE_BARCODE: Barcode '%s' is already assigned to another item", barcode)
	}

	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return err
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AssignBarcode BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, "UPDATE DMASTER SET BARCODE = ?, COSTGROUP = 'DIR' WHERE ITEMPARTNO = ?", barcode, itemPartNo); err != nil {
		return fmt.Errorf("AssignBarcode UPDATE: %w", err)
	}
	return tx.Commit()
}

// ─── AddLinkedBarcode ─────────────────────────────────────────────────────────

// AddLinkedBarcode inserts a row into LINKED_BARCODES and updates DMASTER.LINKEDSKU.
// TODO: Disabled until LINKEDSKU column and LINKED_BARCODES table are added via migration
/*
func (r *InventoryFirebird) AddLinkedBarcode(ctx context.Context, itemPartNo int64, barcode string) error {
	// Fetch mPartNo
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return err
	}
	var mPartNo string
	if err := db.GetContext(ctx, &mPartNo, "SELECT MPARTNO FROM DMASTER WHERE ITEMPARTNO = ?", itemPartNo); err != nil {
		return fmt.Errorf("AddLinkedBarcode: item %d not found", itemPartNo)
	}

	dup, err := r.BarcodeExists(ctx, barcode, itemPartNo)
	if err != nil {
		return err
	}
	if dup {
		return fmt.Errorf("ERR_DUPLICATE_BARCODE: Barcode '%s' is already assigned to another item", barcode)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AddLinkedBarcode BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, "INSERT INTO LINKED_BARCODES (BARCODE, SKU) VALUES (?, ?)", barcode, mPartNo); err != nil {
		return fmt.Errorf("AddLinkedBarcode INSERT: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE DMASTER SET LINKEDSKU = ? WHERE ITEMPARTNO = ?", barcode, itemPartNo); err != nil {
		return fmt.Errorf("AddLinkedBarcode UPDATE DMASTER: %w", err)
	}

	return tx.Commit()
}
*/

// AddLinkedBarcode - Temporary stub until migration is created
func (r *InventoryFirebird) AddLinkedBarcode(ctx context.Context, itemPartNo int64, barcode string) error {
	return fmt.Errorf("AddLinkedBarcode: not implemented - requires LINKEDSKU column migration")
}

// ─── Reports ──────────────────────────────────────────────────────────────────

// GetInventoryValue returns the total stock value for a given stock sheet.
// Stage 8.1: SUM(UNITCOST * (FRONTOPENINGSTOCK + BACKOPENINGSTOCK))
func (r *InventoryFirebird) GetInventoryValue(ctx context.Context, stockSheet string) (float64, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, err
	}
	var totalValue float64
	q := "SELECT COALESCE(SUM(UNITCOST * (FRONTOPENINGSTOCK + BACKOPENINGSTOCK)), 0) AS total_value FROM DMASTER WHERE INVFORM = ?"
	if stockSheet == "" {
		q = "SELECT COALESCE(SUM(UNITCOST * (FRONTOPENINGSTOCK + BACKOPENINGSTOCK)), 0) AS total_value FROM DMASTER WHERE 1=1"
		if err := db.GetContext(ctx, &totalValue, q); err != nil {
			return 0, fmt.Errorf("GetInventoryValue: %w", err)
		}
		return round2dp(totalValue), nil
	}
	if err := db.GetContext(ctx, &totalValue, q, stockSheet); err != nil {
		return 0, fmt.Errorf("GetInventoryValue: %w", err)
	}
	return round2dp(totalValue), nil
}

// GetStockVariance returns the variance report for each item in a stock sheet.
// Stage 8.2: composite opening/closing/purchases/sales from DMASTER.
func (r *InventoryFirebird) GetStockVariance(ctx context.Context, stockSheet string) ([]models.StockVarianceLine, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	var where string
	var args []interface{}
	if stockSheet != "" {
		where = "WHERE INVFORM = ?"
		args = append(args, stockSheet)
	} else {
		where = "WHERE 1=1"
	}

	q := fmt.Sprintf(`
		SELECT MPARTNO, DESCRIPTION, BIN,
		       (FRONTOPENINGSTOCK + BACKOPENINGSTOCK)          AS opening,
		       PURCHASES                                        AS received,
		       (FRONTCLOSINGSTOCK + BACKCLOSINGSTOCK)          AS closing,
		       SALES                                           AS sales,
		       ((FRONTOPENINGSTOCK + BACKOPENINGSTOCK) + PURCHASES
		        - (FRONTCLOSINGSTOCK + BACKCLOSINGSTOCK) - SALES) AS variance,
		       UNITCOST
		FROM DMASTER %s
		ORDER BY BIN, DESCRIPTION`, where)

	var lines []models.StockVarianceLine
	if err := db.SelectContext(ctx, &lines, q, args...); err != nil {
		return nil, fmt.Errorf("GetStockVariance: %w", err)
	}

	// Compute variance_value in Go (variance * unit_cost)
	for i := range lines {
		lines[i].VarianceValue = round2dp(lines[i].Variance * lines[i].UnitCost)
	}
	return lines, nil
}
