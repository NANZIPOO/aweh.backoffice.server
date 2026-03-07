package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/aweh-pos/gateway/internal/models"
)

// GroupFirebird implements GroupRepository against Firebird 3.0.
type GroupFirebird struct {
	BaseRepository
}

func NewGroupRepository(tm *TenantManager) *GroupFirebird {
	return &GroupFirebird{BaseRepository{TM: tm}}
}

func normalizeUOM(raw string) string {
	uom := strings.ToUpper(strings.TrimSpace(raw))
	if uom == "" {
		uom = "UNIT"
	}
	if len(uom) > 50 {
		uom = uom[:50]
	}
	return uom
}

// ─── CreateGroup ─────────────────────────────────────────────────────────────

func (r *GroupFirebird) CreateGroup(ctx context.Context, groupName string, baseUOM string, variants []models.InventoryItem) (int64, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant db: %w", err)
	}
	baseUOM = normalizeUOM(baseUOM)
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Step 1: Generate GROUP_ID
	var groupID int64
	err = tx.QueryRowx("SELECT GEN_ID(GroupIdGen, 1) FROM RDB$DATABASE").Scan(&groupID)
	if err != nil {
		return 0, fmt.Errorf("failed to generate GROUP_ID: %w", err)
	}

	// Step 2: Pre-generate ITEMPARTNOs for all variants
	itemPartNos := make([]int64, len(variants))
	for i := range variants {
		err = tx.QueryRowx("SELECT GEN_ID(DMASTER_GEN, 1) FROM RDB$DATABASE").Scan(&itemPartNos[i])
		if err != nil {
			return 0, fmt.Errorf("failed to generate ITEMPARTNO for variant %d: %w", i, err)
		}
	}

	// Step 3: INSERT into INVENTORY_GROUPS first (required by FK_DMASTER_GROUP)
	insertGroupSQL := `
		INSERT INTO INVENTORY_GROUPS (GROUP_ID, BASE_ITEMPARTNO, GROUP_NAME, BASE_UOM)
		VALUES (?, ?, ?, ?)
	`
	_, err = tx.ExecContext(ctx, insertGroupSQL, groupID, itemPartNos[0], groupName, baseUOM)
	if err != nil {
		return 0, fmt.Errorf("failed to insert INVENTORY_GROUPS: %w", err)
	}

	// Step 4: INSERT variants into DMASTER
	for i, variant := range variants {
		itemPartNo := itemPartNos[i]

		eachCost := variant.EachCost
		if eachCost <= 0 && variant.Pack > 0 {
			eachCost = variant.PackCost / variant.Pack
		}
		unitsPerPack := variant.Units
		if unitsPerPack <= 0 {
			unitsPerPack = 1
		}
		unitCost := eachCost / unitsPerPack
		mPartNo := variant.MPartNo
		if mPartNo == "" {
			mPartNo = fmt.Sprintf("%d", itemPartNo)
		}
		uom := ""
		if variant.UOM.Valid {
			uom = variant.UOM.String
		}
		uom = normalizeUOM(uom)

		// INSERT into DMASTER
		insertSQL := `
			INSERT INTO DMASTER (
				ITEMPARTNO, MPARTNO, SUPPLIERNO, DESCRIPTION, CATOGORY, BIN, INVFORM,
				COSTCATEGORY, PACK, PACKUNIT, PACKCOST, EACHCOST, UNITS, EACHUNIT, UNITCOST,
				SELLINGPRICE, TAXRATE, MARKUP, MINSTOCKLEVEL, MAXSTOCKLEVEL, REORDERLEVE,
				AUTOYIELD, BULKSELLINGPRICE, DISCOUNT, BUCOS, ONORDER, WEIGHT, TARE,
				FRONTOPENINGSTOCK, BACKOPENINGSTOCK, FRONTCLOSINGSTOCK, BACKCLOSINGSTOCK,
				PURCHASES, SALES, UOM, GROUP_ID, IS_SELLABLE, IS_ORDERING_ALLOWED
			) VALUES (
				?, ?, ?, ?, ?, ?, ?,
				?, ?, ?, ?, ?, ?, ?, ?,
				?, ?, ?, ?, ?, ?,
				?, ?, ?, ?, ?, ?, ?,
				0, 0, 0, 0,
				0, 0, ?, ?, ?, ?
			)
		`
		_, err = tx.ExecContext(ctx, insertSQL,
			itemPartNo,
			mPartNo,
			variant.SupplierNo,
			variant.Description,
			"DEFAULT",
			"A1",
			"",
			"",
			variant.Pack,
			variant.PackUnit,
			variant.PackCost,
			eachCost,
			unitsPerPack,
			variant.EachUnit,
			unitCost,
			variant.SellingPrice,
			variant.TaxRate,
			variant.Markup,
			0,
			0,
			0,
			"F",
			0.0,
			0.0,
			0.0,
			0.0,
			0.0,
			0.0,
			uom,
			groupID,
			variant.IsSellable,
			variant.IsOrderingAllowed,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to insert variant %d into DMASTER: %w", i, err)
		}
	}

	// Step 5: Mark first variant as base
	_, err = tx.ExecContext(ctx, "UPDATE DMASTER SET IS_BASE_VARIANT = TRUE WHERE ITEMPARTNO = ?", itemPartNos[0])
	if err != nil {
		return 0, fmt.Errorf("failed to mark base variant: %w", err)
	}

	// Commit
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return groupID, nil
}

// ─── GetGroup ────────────────────────────────────────────────────────────────

func (r *GroupFirebird) GetGroup(ctx context.Context, groupID int64) (*models.ProductGroup, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant db: %w", err)
	}
	group := &models.ProductGroup{}
	err = db.GetContext(ctx, group, "SELECT * FROM INVENTORY_GROUPS WHERE GROUP_ID = ?", groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch group: %w", err)
	}
	return group, nil
}

// ─── GetGroupWithVariants ───────────────────────────────────────────────────

func (r *GroupFirebird) GetGroupWithVariants(ctx context.Context, groupID int64) (*models.ProductGroup, []models.InventoryItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get tenant db: %w", err)
	}

	// Fetch group
	group, err := r.GetGroup(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}

	// Fetch all variants
	var variants []models.InventoryItem
	err = db.SelectContext(ctx, &variants,
		`SELECT `+inventorySelectCols+` 
		 FROM DMASTER WHERE GROUP_ID = ? ORDER BY IS_BASE_VARIANT DESC`,
		groupID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch variants: %w", err)
	}

	return group, variants, nil
}

// ─── DeleteGroup ────────────────────────────────────────────────────────────

func (r *GroupFirebird) DeleteGroup(ctx context.Context, groupID int64) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return fmt.Errorf("failed to get tenant db: %w", err)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Unlink all variants
	_, err = tx.ExecContext(ctx, "UPDATE DMASTER SET GROUP_ID = NULL WHERE GROUP_ID = ?", groupID)
	if err != nil {
		return fmt.Errorf("failed to unlink variants: %w", err)
	}

	// Delete group
	_, err = tx.ExecContext(ctx, "DELETE FROM INVENTORY_GROUPS WHERE GROUP_ID = ?", groupID)
	if err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// ─── LinkItemToGroup ────────────────────────────────────────────────────────

func (r *GroupFirebird) LinkItemToGroup(ctx context.Context, itemPartNo int64, groupID int64) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return fmt.Errorf("failed to get tenant db: %w", err)
	}
	_, err = db.ExecContext(ctx, "UPDATE DMASTER SET GROUP_ID = ? WHERE ITEMPARTNO = ?", groupID, itemPartNo)
	if err != nil {
		return fmt.Errorf("failed to link item to group: %w", err)
	}
	return nil
}

// ─── EnsureGroupForItem ─────────────────────────────────────────────────────

func (r *GroupFirebird) EnsureGroupForItem(ctx context.Context, itemPartNo int64) (int64, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant db: %w", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var currentGroupID sql.NullInt64
	var description string
	var uom sql.NullString

	err = tx.QueryRowx(
		"SELECT GROUP_ID, DESCRIPTION, UOM FROM DMASTER WHERE ITEMPARTNO = ?",
		itemPartNo,
	).Scan(&currentGroupID, &description, &uom)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch item for grouping: %w", err)
	}

	if currentGroupID.Valid && currentGroupID.Int64 > 0 {
		if err = tx.Commit(); err != nil {
			return 0, fmt.Errorf("failed to commit existing group transaction: %w", err)
		}
		return currentGroupID.Int64, nil
	}

	var groupID int64
	err = tx.QueryRowx("SELECT GEN_ID(GroupIdGen, 1) FROM RDB$DATABASE").Scan(&groupID)
	if err != nil {
		return 0, fmt.Errorf("failed to generate GROUP_ID: %w", err)
	}

	groupName := description
	if groupName == "" {
		groupName = fmt.Sprintf("Group %d", groupID)
	}

	baseUOM := ""
	if uom.Valid {
		baseUOM = uom.String
	}
	baseUOM = normalizeUOM(baseUOM)

	_, err = tx.ExecContext(
		ctx,
		"INSERT INTO INVENTORY_GROUPS (GROUP_ID, BASE_ITEMPARTNO, GROUP_NAME, BASE_UOM) VALUES (?, ?, ?, ?)",
		groupID,
		itemPartNo,
		groupName,
		baseUOM,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create group for item: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		"UPDATE DMASTER SET GROUP_ID = ?, UOM = ?, IS_BASE_VARIANT = TRUE WHERE ITEMPARTNO = ?",
		groupID,
		baseUOM,
		itemPartNo,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to link base item to new group: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit ensure-group transaction: %w", err)
	}

	return groupID, nil
}

// ─── AddVariantToGroup ──────────────────────────────────────────────────────
// Creates a new DMASTER item and assigns it to an existing product group.
// The new item is marked as IS_BASE_VARIANT = FALSE and assigned the group's GROUP_ID.

func (r *GroupFirebird) AddVariantToGroup(ctx context.Context, groupID int64, req models.AddVariantRequest) (int64, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant db: %w", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Inherit shared item metadata from the group's base item so variants default
	// to the same supplier/category/location context as the main item.
	var baseItemPartNo int64
	err = tx.QueryRowx(
		"SELECT BASE_ITEMPARTNO FROM INVENTORY_GROUPS WHERE GROUP_ID = ?",
		groupID,
	).Scan(&baseItemPartNo)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch group base item: %w", err)
	}

	var templateSupplierNo string
	var templateCategory sql.NullString
	var templateBin sql.NullString
	var templateInvForm sql.NullString
	var templateCostCategory sql.NullString
	err = tx.QueryRowx(
		"SELECT SUPPLIERNO, CATOGORY, BIN, INVFORM, COSTCATEGORY FROM DMASTER WHERE ITEMPARTNO = ?",
		baseItemPartNo,
	).Scan(
		&templateSupplierNo,
		&templateCategory,
		&templateBin,
		&templateInvForm,
		&templateCostCategory,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch base item template fields: %w", err)
	}

	category := ""
	if templateCategory.Valid {
		category = templateCategory.String
	}
	bin := ""
	if templateBin.Valid {
		bin = templateBin.String
	}
	invForm := ""
	if templateInvForm.Valid {
		invForm = templateInvForm.String
	}
	costCategory := ""
	if templateCostCategory.Valid {
		costCategory = templateCostCategory.String
	}

	// Step 1: Generate ITEMPARTNO for new variant
	var itemPartNo int64
	err = tx.QueryRowx("SELECT GEN_ID(DMASTER_GEN, 1) FROM RDB$DATABASE").Scan(&itemPartNo)
	if err != nil {
		return 0, fmt.Errorf("failed to generate ITEMPARTNO: %w", err)
	}

	// Step 2: Calculate derived costs (units-first, pack fallback for legacy payloads)
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

	mPartNo := fmt.Sprintf("%d", itemPartNo)
	variantUOM := normalizeUOM(req.UOM)

	// Step 3: Insert into DMASTER with GROUP_ID set
	insertSQL := `
		INSERT INTO DMASTER (
			ITEMPARTNO, MPARTNO, SUPPLIERNO, DESCRIPTION, CATOGORY, BIN, INVFORM,
			COSTCATEGORY, PACK, PACKUNIT, PACKCOST, EACHCOST, UNITS, EACHUNIT, UNITCOST,
			SELLINGPRICE, TAXRATE, MARKUP, MINSTOCKLEVEL, MAXSTOCKLEVEL, REORDERLEVE,
			AUTOYIELD, BULKSELLINGPRICE, DISCOUNT, BUCOS, ONORDER, WEIGHT, TARE,
			FRONTOPENINGSTOCK, BACKOPENINGSTOCK, FRONTCLOSINGSTOCK, BACKCLOSINGSTOCK,
			PURCHASES, SALES, UOM, GROUP_ID, IS_BASE_VARIANT, IS_SELLABLE, IS_ORDERING_ALLOWED
		) VALUES (
			?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?,
			0, 0, 0, 0,
			0, 0, ?, ?, ?, ?, ?
		)
	`
	_, err = tx.ExecContext(ctx, insertSQL,
		itemPartNo,
		mPartNo,
		templateSupplierNo,
		req.Description,
		category,
		bin,
		invForm,
		costCategory,
		req.PackSize,
		req.PackUnit,
		req.PackCost,
		eachCost,
		unitsPerPack,
		req.EachUnit,
		unitCost,
		req.SellingPrice,
		req.TaxRate,
		req.Markup,
		0,   // MINSTOCKLEVEL
		0,   // MAXSTOCKLEVEL
		0,   // REORDERLEVE
		"F", // AUTOYIELD
		0.0, // BULKSELLINGPRICE
		0.0, // DISCOUNT
		0.0, // BUCOS
		0.0, // ONORDER
		0.0, // WEIGHT
		0.0, // TARE
		variantUOM,
		groupID,
		false, // IS_BASE_VARIANT (new variants are never base by default)
		req.IsSellable,
		orderingAllowed,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert variant into DMASTER: %w", err)
	}

	// Commit
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return itemPartNo, nil
}

// ─── UnlinkItemFromGroup ────────────────────────────────────────────────────

func (r *GroupFirebird) UnlinkItemFromGroup(ctx context.Context, itemPartNo int64) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return fmt.Errorf("failed to get tenant db: %w", err)
	}
	_, err = db.ExecContext(ctx, "UPDATE DMASTER SET GROUP_ID = NULL, IS_BASE_VARIANT = FALSE WHERE ITEMPARTNO = ?", itemPartNo)
	if err != nil {
		return fmt.Errorf("failed to unlink item from group: %w", err)
	}
	return nil
}

// ─── ChangeBaseUnit ─────────────────────────────────────────────────────────

func (r *GroupFirebird) ChangeBaseUnit(ctx context.Context, groupID int64, newBaseItemPartNo int64) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return fmt.Errorf("failed to get tenant db: %w", err)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Step 1: Fetch current base item
	var oldBaseItemPartNo int64
	err = tx.QueryRowx("SELECT BASE_ITEMPARTNO FROM INVENTORY_GROUPS WHERE GROUP_ID = ?", groupID).Scan(&oldBaseItemPartNo)
	if err != nil {
		return fmt.Errorf("failed to fetch current base item: %w", err)
	}

	// Step 2: Fetch old base PURCHASES/SALES
	var oldPurchases, oldSales float64
	err = tx.QueryRowx("SELECT PURCHASES, SALES FROM DMASTER WHERE ITEMPARTNO = ?", oldBaseItemPartNo).
		Scan(&oldPurchases, &oldSales)
	if err != nil {
		return fmt.Errorf("failed to fetch old base stock: %w", err)
	}

	// Step 3: Fetch old/new base conversion fields.
	// Conversion factor uses UNITS only (legacy PACK columns ignored in modern software).
	toFactor := func(units float64) float64 {
		if units > 0 {
			return units
		}
		return 1
	}

	var oldUnits float64
	err = tx.QueryRowx("SELECT UNITS FROM DMASTER WHERE ITEMPARTNO = ?", oldBaseItemPartNo).
		Scan(&oldUnits)
	if err != nil {
		return fmt.Errorf("failed to fetch old base conversion fields: %w", err)
	}

	var newUnits float64
	err = tx.QueryRowx("SELECT UNITS FROM DMASTER WHERE ITEMPARTNO = ?", newBaseItemPartNo).
		Scan(&newUnits)
	if err != nil {
		return fmt.Errorf("failed to fetch new base conversion fields: %w", err)
	}

	oldFactor := toFactor(oldUnits)
	newFactor := toFactor(newUnits)

	var newBaseUOM sql.NullString
	err = tx.QueryRowx("SELECT UOM FROM DMASTER WHERE ITEMPARTNO = ?", newBaseItemPartNo).Scan(&newBaseUOM)
	if err != nil {
		return fmt.Errorf("failed to fetch new base uom: %w", err)
	}
	normalizedNewBaseUOM := ""
	if newBaseUOM.Valid {
		normalizedNewBaseUOM = newBaseUOM.String
	}
	normalizedNewBaseUOM = normalizeUOM(normalizedNewBaseUOM)

	// Step 4: Convert stock through atomic units:
	// atomic = old_qty * old_factor
	// new_qty = atomic / new_factor
	newPurchases := (oldPurchases * oldFactor) / newFactor
	newSales := (oldSales * oldFactor) / newFactor

	// Step 5: Update new base
	_, err = tx.ExecContext(ctx,
		"UPDATE DMASTER SET PURCHASES = ?, SALES = ? WHERE ITEMPARTNO = ?",
		newPurchases, newSales, newBaseItemPartNo)
	if err != nil {
		return fmt.Errorf("failed to update new base: %w", err)
	}

	// Step 6: Zero out old base
	_, err = tx.ExecContext(ctx,
		"UPDATE DMASTER SET PURCHASES = 0, SALES = 0 WHERE ITEMPARTNO = ?",
		oldBaseItemPartNo)
	if err != nil {
		return fmt.Errorf("failed to zero old base: %w", err)
	}

	// Step 7: Update group
	_, err = tx.ExecContext(ctx,
		"UPDATE INVENTORY_GROUPS SET BASE_ITEMPARTNO = ? WHERE GROUP_ID = ?",
		newBaseItemPartNo, groupID)
	if err != nil {
		return fmt.Errorf("failed to update group: %w", err)
	}

	// Step 8: Update base variant flags
	_, err = tx.ExecContext(ctx, "UPDATE DMASTER SET IS_BASE_VARIANT = FALSE WHERE ITEMPARTNO = ?", oldBaseItemPartNo)
	if err != nil {
		return fmt.Errorf("failed to unmark old base: %w", err)
	}

	_, err = tx.ExecContext(ctx, "UPDATE DMASTER SET UOM = ?, IS_BASE_VARIANT = TRUE WHERE ITEMPARTNO = ?", normalizedNewBaseUOM, newBaseItemPartNo)
	if err != nil {
		return fmt.Errorf("failed to mark new base: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// ─── GetGroupBaseQty ────────────────────────────────────────────────────────

func (r *GroupFirebird) GetGroupBaseQty(ctx context.Context, groupID int64) (float64, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant db: %w", err)
	}
	var baseItemPartNo int64
	var purchases, sales float64

	// Get base item
	err = db.QueryRowx("SELECT BASE_ITEMPARTNO FROM INVENTORY_GROUPS WHERE GROUP_ID = ?", groupID).
		Scan(&baseItemPartNo)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch base item: %w", err)
	}

	// Get stock
	err = db.QueryRowx("SELECT PURCHASES, SALES FROM DMASTER WHERE ITEMPARTNO = ?", baseItemPartNo).
		Scan(&purchases, &sales)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch stock: %w", err)
	}

	return purchases - sales, nil
}

// ─── GetVariantCalculatedQty ──────────────────────────────────────────────

func (r *GroupFirebird) GetVariantCalculatedQty(ctx context.Context, itemPartNo int64, baseQty float64) (float64, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant db: %w", err)
	}
	var variantUnits float64
	var groupID sql.NullInt64

	err = db.QueryRowx(
		"SELECT UNITS, GROUP_ID FROM DMASTER WHERE ITEMPARTNO = ?",
		itemPartNo,
	).Scan(&variantUnits, &groupID)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch variant conversion fields: %w", err)
	}

	// Conversion factor uses UNITS only (legacy PACK columns ignored in modern software).
	toFactor := func(units float64) float64 {
		if units > 0 {
			return units
		}
		return 1
	}

	variantFactor := toFactor(variantUnits)
	baseFactor := 1.0

	if groupID.Valid {
		var baseItemPartNo int64
		err = db.QueryRowx(
			"SELECT BASE_ITEMPARTNO FROM INVENTORY_GROUPS WHERE GROUP_ID = ?",
			groupID.Int64,
		).Scan(&baseItemPartNo)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch base item for group: %w", err)
		}

		var baseUnits float64
		err = db.QueryRowx(
			"SELECT UNITS FROM DMASTER WHERE ITEMPARTNO = ?",
			baseItemPartNo,
		).Scan(&baseUnits)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch base conversion fields: %w", err)
		}

		baseFactor = toFactor(baseUnits)
	}

	baseQtyInAtomicUnits := baseQty * baseFactor
	return math.Round((baseQtyInAtomicUnits/variantFactor)*100) / 100, nil // Round to 2 decimals
}

// ─── CreateMovement ────────────────────────────────────────────────────────

func (r *GroupFirebird) CreateMovement(ctx context.Context, movement models.StockMovement) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return fmt.Errorf("failed to get tenant db: %w", err)
	}

	// Generate MOVEMENT_ID
	var movementID int64
	err = db.QueryRowx("SELECT GEN_ID(MovementIdGen, 1) FROM RDB$DATABASE").Scan(&movementID)
	if err != nil {
		return fmt.Errorf("failed to generate MOVEMENT_ID: %w", err)
	}

	insertSQL := `
		INSERT INTO STOCK_MOVEMENTS 
		(MOVEMENT_ID, GROUP_ID, VARIANT_ITEMPARTNO, BASE_ITEMPARTNO, MOVEMENT_TYPE, QTY_VARIANT, QTY_BASE, REFERENCE)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = db.ExecContext(ctx, insertSQL,
		movementID, movement.GroupID, movement.VariantItemPartNo, movement.BaseItemPartNo,
		movement.MovementType, movement.QtyVariant, movement.QtyBase, movement.Reference.String)
	if err != nil {
		return fmt.Errorf("failed to create movement: %w", err)
	}

	return nil
}

// ─── GetMovements ──────────────────────────────────────────────────────────

func (r *GroupFirebird) GetMovements(ctx context.Context, groupID int64, limit int, offset int) ([]models.StockMovement, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant db: %w", err)
	}
	var movements []models.StockMovement

	err = db.SelectContext(ctx, &movements,
		`SELECT * FROM STOCK_MOVEMENTS WHERE GROUP_ID = ? 
		 ORDER BY CREATED_AT DESC ROWS ? TO ?`,
		groupID, offset+1, offset+limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch movements: %w", err)
	}

	return movements, nil
}
