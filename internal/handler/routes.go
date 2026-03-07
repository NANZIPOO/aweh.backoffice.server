package handler

import (
	"net/http"

	"github.com/aweh-pos/gateway/internal/middleware"
	"github.com/aweh-pos/gateway/internal/repository"
)

// Type alias for convenience
type GroupRepository = repository.GroupRepository

// RegisterInventoryRoutes wires all inventory endpoints to the default ServeMux.
// Access-level enforcement follows the plan §1.4 table:
//
//	READ          ≥ 1  (all GET endpoints)
//	STOCK-TAKE    ≥ 2  (closing stock, record wastage)
//	CRUD          ≥ 3  (create/update item, clone, barcode, GRV)
//	DELETE/FINAL  ≥ 5  (delete item, finalize period, post wastage)
func RegisterInventoryRoutes(mux *http.ServeMux, h *InventoryHandler, gr GroupRepository, jwtSecret []byte) {
	auth := middleware.AuthMiddleware(jwtSecret)
	lvl := func(level int, next http.HandlerFunc) http.Handler {
		return auth(middleware.RequireLevel(level)(http.HandlerFunc(next)))
	}

	// ── Lookups ──────────────────────────────────────────────────────────
	mux.Handle("GET /api/v1/inventory/lookups", lvl(1, h.GetInventoryLookups))

	// ── Inventory Items ───────────────────────────────────────────────────
	mux.Handle("GET /api/v1/inventory/items", lvl(1, h.ListInventoryItems))
	mux.Handle("GET /api/v1/inventory/items/{id}", lvl(1, h.GetInventoryItem))
	mux.Handle("POST /api/v1/inventory/items", lvl(3, h.CreateInventoryItem))
	mux.Handle("PUT /api/v1/inventory/items/{id}", lvl(3, h.UpdateInventoryItem))
	mux.Handle("DELETE /api/v1/inventory/items/{id}", lvl(5, h.DeleteInventoryItem))
	mux.Handle("POST /api/v1/inventory/items/{id}/clone", lvl(3, h.CloneInventoryItem))
	mux.Handle("POST /api/v1/inventory/items/{id}/barcode", lvl(3, h.AssignBarcode))
	mux.Handle("POST /api/v1/inventory/items/{id}/barcode/bulk", lvl(3, h.AddLinkedBarcode))

	// ── Product Groups ───────────────────────────────────────────────────────
	groupHandlers := NewProductGroupHandlers(gr, h.inventory)
	mux.Handle("POST /api/v1/inventory/groups", lvl(3, groupHandlers.CreateGroup))
	mux.Handle("GET /api/v1/inventory/groups/{group_id}", lvl(1, groupHandlers.GetGroup))
	mux.Handle("POST /api/v1/inventory/groups/{group_id}/link-item", lvl(3, groupHandlers.LinkItem))
	mux.Handle("POST /api/v1/inventory/groups/{group_id}/add-variant", lvl(3, groupHandlers.AddVariant))
	mux.Handle("POST /api/v1/inventory/items/{item_id}/add-variant", lvl(3, groupHandlers.AddVariantFromItem))
	mux.Handle("DELETE /api/v1/inventory/groups/{group_id}/unlink/{item_id}", lvl(3, groupHandlers.UnlinkItem))
	mux.Handle("POST /api/v1/inventory/groups/{group_id}/change-base-unit", lvl(5, groupHandlers.ChangeBaseUnit))
	mux.Handle("GET /api/v1/inventory/groups/{group_id}/movements", lvl(1, groupHandlers.GetMovements))
	mux.Handle("DELETE /api/v1/inventory/groups/{group_id}", lvl(5, groupHandlers.DeleteGroup))

	// ── Stock-Take ────────────────────────────────────────────────────────
	mux.Handle("GET /api/v1/inventory/stock-take", lvl(1, h.GetStockTakeSheet))
	mux.Handle("PUT /api/v1/inventory/stock-take/closing", lvl(2, h.UpdateClosingStock))
	mux.Handle("POST /api/v1/inventory/stock-take/finalize", lvl(5, h.FinalizeStockPeriod))

	// ── GRV ───────────────────────────────────────────────────────────────
	mux.Handle("POST /api/v1/inventory/grv", lvl(3, h.CreateGrv))

	// ── Wastage ───────────────────────────────────────────────────────────
	mux.Handle("POST /api/v1/inventory/wastage", lvl(2, h.RecordWastage))
	mux.Handle("POST /api/v1/inventory/wastage/post-pending", lvl(5, h.PostPendingWastage))

	// ── Reports ───────────────────────────────────────────────────────────
	mux.Handle("GET /api/v1/inventory/reports/value", lvl(1, h.GetInventoryValue))
	mux.Handle("GET /api/v1/inventory/reports/variance", lvl(1, h.GetStockVariance))
}

// RegisterDashboardRoutes wires dashboard analytics endpoint
// Dashboard is read-only, requires auth but no special access level (≥ 1)
func RegisterDashboardRoutes(mux *http.ServeMux, h *DashboardHandler, jwtSecret []byte) {
	auth := middleware.AuthMiddleware(jwtSecret)
	lvl := func(level int, next http.HandlerFunc) http.Handler {
		return auth(middleware.RequireLevel(level)(http.HandlerFunc(next)))
	}

	// ── Dashboard ─────────────────────────────────────────────────────────
	mux.Handle("GET /api/v1/dashboard/summary", lvl(1, h.GetDashboardSummary))
}

// RegisterSettingsRoutes wires all settings endpoints
// Access level enforcement:
//   READ/WRITE  ≥ 3 (Manager can view/edit most settings)
//   SECURITY    ≥ 5 (Admin/Owner only for AppKey changes)
func RegisterSettingsRoutes(mux *http.ServeMux, h *SettingsHandler, jwtSecret []byte) {
	auth := middleware.AuthMiddleware(jwtSecret)
	lvl := func(level int, next http.HandlerFunc) http.Handler {
		return auth(middleware.RequireLevel(level)(http.HandlerFunc(next)))
	}

	// ── Core Setup (Database & Paths) ────────────────────────────────────
	mux.Handle("GET /api/v1/settings/core-setup", lvl(5, h.GetCoreSetup))           // Admin only
	mux.Handle("PUT /api/v1/settings/core-setup", lvl(5, h.SaveCoreSetup))          // Admin only

	// ── Business Profile (Company Info + Receipt Templates) ──────────────
	mux.Handle("GET /api/v1/settings/business-profile", lvl(3, h.GetBusinessProfile))   // Manager+
	mux.Handle("PUT /api/v1/settings/business-profile", lvl(3, h.SaveBusinessProfile))  // Manager+

	// ── Financial Control (Tax + Commissions + Cashup) ───────────────────
	mux.Handle("GET /api/v1/settings/financial-control", lvl(5, h.GetFinancialControl)) // Owner/Admin
	mux.Handle("PUT /api/v1/settings/financial-control", lvl(5, h.SaveFinancialControl))// Owner/Admin

	// ── Security & Access (UserMode + AppKey) ─────────────────────────────
	mux.Handle("GET /api/v1/settings/security-access", lvl(5, h.GetSecurityAccess))     // Owner/Admin
	mux.Handle("PUT /api/v1/settings/security-access", lvl(5, h.SaveSecurityAccess))    // Owner/Admin
	mux.Handle("PUT /api/v1/settings/security-access/appkey", lvl(5, h.ChangeAppKey))   // Owner/Admin

	// ── Device & Terminal (Hardware + Operations) ─────────────────────────
	mux.Handle("GET /api/v1/settings/device-terminal", lvl(5, h.GetDeviceTerminal))     // Admin only
	mux.Handle("PUT /api/v1/settings/device-terminal", lvl(5, h.SaveDeviceTerminal))    // Admin only
}

// RegisterSalesMenuRoutes wires Sales Menu endpoints.
// Access level enforcement:
//   READ          ≥ 1  (all GET endpoints)
//   CRUD          ≥ 3  (POST, PUT)
//   DELETE        ≥ 5  (DELETE)
func RegisterSalesMenuRoutes(mux *http.ServeMux, h *SalesMenuHandler, jwtSecret []byte) {
	auth := middleware.AuthMiddleware(jwtSecret)
	lvl := func(level int, next http.HandlerFunc) http.Handler {
		return auth(middleware.RequireLevel(level)(http.HandlerFunc(next)))
	}

	// ── Groups ────────────────────────────────────────────────────────────
	mux.Handle("GET /api/v1/sales/menu/groups", lvl(1, h.GetGroups))
	mux.Handle("POST /api/v1/sales/menu/groups", lvl(3, h.CreateGroup))
	mux.Handle("PUT /api/v1/sales/menu/groups/{id}", lvl(3, h.UpdateGroup))
	mux.Handle("DELETE /api/v1/sales/menu/groups/{id}", lvl(5, h.DeleteGroup))

	// ── Items ─────────────────────────────────────────────────────────────
	mux.Handle("GET /api/v1/sales/menu/items", lvl(1, h.GetItems))
	mux.Handle("GET /api/v1/sales/menu/items/{id}", lvl(1, h.GetItem))
	mux.Handle("POST /api/v1/sales/menu/items", lvl(3, h.CreateItem))
	mux.Handle("PUT /api/v1/sales/menu/items/{id}", lvl(3, h.UpdateItem))
	mux.Handle("DELETE /api/v1/sales/menu/items/{id}", lvl(5, h.DeleteItem))
}

