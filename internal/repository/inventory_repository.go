package repository

import (
	"context"
	"time"

	"github.com/aweh-pos/gateway/internal/models"
)

// ─── InventoryRepository ──────────────────────────────────────────────────────
//
// Tenancy note (D-01): single-tenant per install. ctx is passed for cancellation
// and future expansion only — no WHERE zone_id clause required.
//
// Access level enforcement is done in handlers, NOT repositories.
//
// ALL write operations MUST follow the Mandatory DB Sequence (Project Constitution §4):
//  1. db.BeginTxx(ctx)
//  2. SELECT GEN_ID(generator, 1) FROM RDB$DATABASE
//  3. Apply business logic in Go
//  4. INSERT/UPDATE with fetched ID
//  5. tx.Commit()  — defer tx.Rollback() on any error
type InventoryRepository interface {
	// Item CRUD
	ListItems(ctx context.Context, filter models.ItemFilter) ([]models.InventoryItem, int, error)
	GetItem(ctx context.Context, itemPartNo int64) (*models.InventoryItemDetail, error)
	CreateItem(ctx context.Context, req models.CreateItemRequest) (*models.InventoryItem, error)
	UpdateItem(ctx context.Context, itemPartNo int64, req models.UpdateItemRequest) (*models.InventoryItem, error)
	DeleteItem(ctx context.Context, itemPartNo int64) error

	// Guard checks — must be called before DeleteItem
	ItemExistsInMenu(ctx context.Context, mPartNo string) (bool, error)
	ItemExistsInRecipe(ctx context.Context, mPartNo string) (bool, error)
	DescriptionExists(ctx context.Context, description string, excludeID int64) (bool, error)
	BarcodeExists(ctx context.Context, barcode string, excludeID int64) (bool, error)

	// Generator
	NextItemPartNo(ctx context.Context) (int64, error) // SELECT GEN_ID(ItempartnoGen, 1) FROM RDB$DATABASE

	// Clone
	CloneItem(ctx context.Context, sourceID int64, newDescription string) (*models.InventoryItem, error)

	// Barcode
	AssignBarcode(ctx context.Context, itemPartNo int64, barcode string) error
	AddLinkedBarcode(ctx context.Context, itemPartNo int64, barcode string) error

	// Reports
	GetInventoryValue(ctx context.Context, stockSheet string) (float64, error)
	GetStockVariance(ctx context.Context, stockSheet string) ([]models.StockVarianceLine, error)
}

// ─── StockTakeRepository ──────────────────────────────────────────────────────
//
// GetStockTakeSheet uses dynamic SQL to alias the per-day columns per Appendix B.
// UpdateClosingStock maps dayOfWeek string → correct FCS column name via the
// dayColumnMap defined in stock_take_firebird.go.
type StockTakeRepository interface {
	GetStockTakeSheet(ctx context.Context, stockSheet string, dayOfWeek string) ([]models.StockTakeDayRow, error)
	UpdateClosingStock(ctx context.Context, req models.UpdateClosingStockRequest) (int, error)
	IsPeriodFinalized(ctx context.Context, stockSheet string, periodDate time.Time) (bool, error)
	// FinalizeStockPeriod resets opening=closing, zeroes closing/purchases/sales for the sheet.
	// Returns the count of DMASTER rows reset.
	// Returns error wrapping "ERR_PERIOD_ALREADY_FINALIZED" if period already exists.
	FinalizeStockPeriod(ctx context.Context, req models.FinalizeStockRequest) (int64, error)
}

// ─── GrvRepository ────────────────────────────────────────────────────────────
//
// CreateGrv follows the MANDATORY DB SEQUENCE (Project Constitution §4):
//  1. db.BeginTxx(ctx)
//  2. SELECT GEN_ID(ORDERS_GEN, 1) FROM RDB$DATABASE         ← fetch ORDERNO
//  3. Compute totals + VAT in Go
//  4. INSERT INTO CREDITORSLEDGER (with fetched ORDERNO)
//  5. For each line: INSERT INTO ORDERITEMS
//  6. For each line: UPDATE DMASTER SET PURCHASES=PURCHASES+qty, {day}REC={day}REC+qty
//  7. UPDATE CREDITORSLEDGER SET RECEIVED='Y'
//  8. tx.Commit()
//
// supplier_name in GrvResponse is resolved via a SEPARATE single-table SELECT
// from CREDITORS AFTER commit — NEVER via a JOIN in the write transaction.
type GrvRepository interface {
	CreateGrv(ctx context.Context, req models.CreateGrvRequest) (*models.GrvHeader, error)
}

// ─── WastageRepository ────────────────────────────────────────────────────────
//
// RecordWastage inserts a single ORDERITEMS row with:
//   - SUPPLIER = 'Wastage Control'
//   - POSTED   = 'F'
//
// It does NOT update DMASTER (see Decision D-03 in implementation plan).
//
// PostPendingWastage is an admin-only endpoint that batch-posts ORDERITEMS rows
// where SUPPLIER='Wastage Control' AND POSTED='F' into DMASTER.PURCHASES.
type WastageRepository interface {
	RecordWastage(ctx context.Context, req models.RecordWastageRequest) (*models.WastageLine, error)
	PostPendingWastage(ctx context.Context, stockSheet string) (int, error)
}

// ─── LookupRepository ────────────────────────────────────────────────────────
//
// Phase 1: derives lookup values from DISTINCT DMASTER queries.
// Replaces the legacy INI file reads from the Delphi app.
type LookupRepository interface {
	GetStockSheets(ctx context.Context) ([]models.LookupItem, error)
	GetBins(ctx context.Context) ([]models.LookupItem, error)
	GetCategories(ctx context.Context) ([]models.LookupItem, error)
	GetCostCategories(ctx context.Context) ([]models.LookupItem, error)
	GetSuppliers(ctx context.Context) ([]models.InventorySupplier, error)
}
