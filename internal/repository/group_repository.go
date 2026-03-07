package repository

import (
	"context"

	"github.com/aweh-pos/gateway/internal/models"
)

// GroupRepository defines all operations for product grouping system.
type GroupRepository interface {
	// ─── Group Management ────────────────────────────────────────────────────

	// CreateGroup creates a new product group with initial variants.
	// Mandatory DB Sequence: BeginTxx → GEN_ID → Insert items → Insert group → Update linkages → Commit
	CreateGroup(ctx context.Context, groupName string, baseUOM string, variants []models.InventoryItem) (groupID int64, err error)

	// GetGroup retrieves group metadata (not variants).
	GetGroup(ctx context.Context, groupID int64) (*models.ProductGroup, error)

	// GetGroupWithVariants retrieves group + all linked variants.
	GetGroupWithVariants(ctx context.Context, groupID int64) (*models.ProductGroup, []models.InventoryItem, error)

	// DeleteGroup removes a product group. Cascades to unlink variants.
	DeleteGroup(ctx context.Context, groupID int64) error

	// ─── Variant Linkage ────────────────────────────────────────────────────

	// LinkItemToGroup links an existing item to a group.
	LinkItemToGroup(ctx context.Context, itemPartNo int64, groupID int64) error

	// AddVariantToGroup creates a new item and assigns it to the group.
	// Returns the new item's ITEMPARTNO.
	AddVariantToGroup(ctx context.Context, groupID int64, req models.AddVariantRequest) (int64, error)

	// EnsureGroupForItem guarantees that an existing item has a GROUP_ID.
	// If item already has a group, returns that group ID.
	// If not, creates a new group with this item as base and returns new group ID.
	EnsureGroupForItem(ctx context.Context, itemPartNo int64) (int64, error)

	// UnlinkItemFromGroup removes a variant from group (GROUP_ID = NULL).
	UnlinkItemFromGroup(ctx context.Context, itemPartNo int64) error

	// ─── Base Unit Management ──────────────────────────────────────────────

	// ChangeBaseUnit switches the base variant and migrates stock.
	// Mandatory DB Sequence: BeginTxx → Fetch conversion → Migrate data → Update group → Commit
	ChangeBaseUnit(ctx context.Context, groupID int64, newBaseItemPartNo int64) error

	// ─── Stock Information ─────────────────────────────────────────────────

	// GetGroupBaseQty retrieves current stock for a group's base item.
	// Returns: base_qty = PURCHASES - SALES
	GetGroupBaseQty(ctx context.Context, groupID int64) (float64, error)

	// GetVariantCalculatedQty calculates displayed quantity for a variant.
	// Returns: (base_qty × base_conversion_factor) ÷ variant_conversion_factor,
	// where conversion factor uses UNITS only (fallback 1 when UNITS <= 0).
	GetVariantCalculatedQty(ctx context.Context, itemPartNo int64, baseQty float64) (float64, error)

	// ─── Stock Movements (Audit) ──────────────────────────────────────────

	// CreateMovement logs a stock movement (GRV, SALE, etc.)
	CreateMovement(ctx context.Context, movement models.StockMovement) error

	// GetMovements retrieves movement history for a group.
	GetMovements(ctx context.Context, groupID int64, limit int, offset int) ([]models.StockMovement, error)
}
