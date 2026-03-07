package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aweh-pos/gateway/internal/middleware"
	"github.com/aweh-pos/gateway/internal/models"
	"github.com/aweh-pos/gateway/internal/repository"
)

// ─── Handler struct ───────────────────────────────────────────────────────────

// InventoryHandler wires the five inventory repositories to HTTP handlers.
// All routes require JWT authentication; access level is enforced per endpoint.
type InventoryHandler struct {
	inventory repository.InventoryRepository
	groups    repository.GroupRepository
	lookups   repository.LookupRepository
	stockTake repository.StockTakeRepository
	grv       repository.GrvRepository
	wastage   repository.WastageRepository
}

func NewInventoryHandler(
	inv repository.InventoryRepository,
	gr repository.GroupRepository,
	lu repository.LookupRepository,
	st repository.StockTakeRepository,
	grv repository.GrvRepository,
	w repository.WastageRepository,
) *InventoryHandler {
	return &InventoryHandler{
		inventory: inv,
		groups:    gr,
		lookups:   lu,
		stockTake: st,
		grv:       grv,
		wastage:   w,
	}
}

// ─── Helper: parse int64 path value ──────────────────────────────────────────

func parseID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	s := r.PathValue(key)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid "+key+": must be an integer")
		return 0, false
	}
	return n, true
}

// ─── Lookups ──────────────────────────────────────────────────────────────────

// GET /api/v1/inventory/lookups — access level ≥ 1
func (h *InventoryHandler) GetInventoryLookups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sheets, err := h.lookups.GetStockSheets(ctx)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	bins, err := h.lookups.GetBins(ctx)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	cats, err := h.lookups.GetCategories(ctx)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	costCats, err := h.lookups.GetCostCategories(ctx)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	suppliers, err := h.lookups.GetSuppliers(ctx)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}

	// Nil-safe empty slices — never return null JSON arrays
	if sheets == nil {
		sheets = []models.LookupItem{}
	}
	if bins == nil {
		bins = []models.LookupItem{}
	}
	if cats == nil {
		cats = []models.LookupItem{}
	}
	if costCats == nil {
		costCats = []models.LookupItem{}
	}
	if suppliers == nil {
		suppliers = []models.InventorySupplier{}
	}

	JSON(w, http.StatusOK, models.InventoryLookups{
		StockSheets:    sheets,
		Bins:           bins,
		Categories:     cats,
		CostCategories: costCats,
		Suppliers:      suppliers,
	})
}

// ─── Inventory Items ──────────────────────────────────────────────────────────

// GET /api/v1/inventory/items — access level ≥ 1
func (h *InventoryHandler) ListInventoryItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))

	filter := models.ItemFilter{
		Search:     q.Get("search"),
		StockSheet: q.Get("stock_sheet"),
		Category:   q.Get("category"),
		Bin:        q.Get("bin"),
		SupplierNo: q.Get("supplier_no"),
		Page:       page,
		Limit:      limit,
		SortBy:     q.Get("sort_by"),
		SortDir:    q.Get("sort_dir"),
	}

	items, total, err := h.inventory.ListItems(r.Context(), filter)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	if items == nil {
		items = []models.InventoryItem{}
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	pages := (total + filter.Limit - 1) / filter.Limit
	if pages < 1 {
		pages = 1
	}
	pageNum := filter.Page
	if pageNum < 1 {
		pageNum = 1
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(models.PaginatedResponse[models.InventoryItem]{
		Data: items,
		Meta: models.PaginationMeta{Total: total, Page: pageNum, Limit: filter.Limit, Pages: pages},
	})
}

// GET /api/v1/inventory/items/{id} — access level ≥ 1
func (h *InventoryHandler) GetInventoryItem(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	includeGroup := strings.EqualFold(r.URL.Query().Get("include_group"), "true")
	detail, err := h.inventory.GetItem(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			Err(w, http.StatusNotFound, "ERR_NOT_FOUND", err.Error())
		} else {
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}

	if includeGroup && detail.GroupID.Valid && detail.GroupID.Int64 > 0 {
		group, variants, err := h.groups.GetGroupWithVariants(r.Context(), detail.GroupID.Int64)
		if err == nil {
			baseQty, baseErr := h.groups.GetGroupBaseQty(r.Context(), detail.GroupID.Int64)
			if baseErr == nil {
				for i := range variants {
					qty, qtyErr := h.groups.GetVariantCalculatedQty(r.Context(), variants[i].ItemPartNo, baseQty)
					if qtyErr == nil {
						variants[i].CalculatedQty = qty
					}
				}

				raw, marshalErr := json.Marshal(detail)
				if marshalErr == nil {
					payload := map[string]any{}
					if unmarshalErr := json.Unmarshal(raw, &payload); unmarshalErr == nil {
						payload["group_details"] = map[string]any{
							"group":    group,
							"base_qty": baseQty,
							"variants": variants,
							"count":    len(variants),
						}
						JSON(w, http.StatusOK, payload)
						return
					}
				}
			}
		}
	}

	JSON(w, http.StatusOK, detail)
}

// POST /api/v1/inventory/items — access level ≥ 3
func (h *InventoryHandler) CreateInventoryItem(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 3 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 3 or higher required")
		return
	}

	var req models.CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}

	item, err := h.inventory.CreateItem(r.Context(), req)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "ERR_DUPLICATE_DESCRIPTION"):
			Err(w, http.StatusConflict, "ERR_DUPLICATE_DESCRIPTION", err.Error())
		case strings.Contains(err.Error(), "ERR_VALIDATION"):
			Err(w, http.StatusUnprocessableEntity, "ERR_VALIDATION", err.Error())
		default:
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}
	JSON(w, http.StatusCreated, item)
}

// PUT /api/v1/inventory/items/{id} — access level ≥ 3
func (h *InventoryHandler) UpdateInventoryItem(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 3 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 3 or higher required")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req models.UpdateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}

	item, err := h.inventory.UpdateItem(r.Context(), id, req)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "ERR_NOT_FOUND"):
			Err(w, http.StatusNotFound, "ERR_NOT_FOUND", err.Error())
		default:
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}
	JSON(w, http.StatusOK, item)
}

// DELETE /api/v1/inventory/items/{id} — access level ≥ 5
func (h *InventoryHandler) DeleteInventoryItem(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 5 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 5 required for item deletion")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if err := h.inventory.DeleteItem(r.Context(), id); err != nil {
		switch {
		case strings.Contains(err.Error(), "ERR_ITEM_IN_MENU"):
			Err(w, http.StatusConflict, "ERR_ITEM_IN_MENU", err.Error())
		case strings.Contains(err.Error(), "ERR_ITEM_IN_RECIPE"):
			Err(w, http.StatusConflict, "ERR_ITEM_IN_RECIPE", err.Error())
		case strings.Contains(err.Error(), "ERR_NOT_FOUND"):
			Err(w, http.StatusNotFound, "ERR_NOT_FOUND", err.Error())
		default:
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}

	JSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

// POST /api/v1/inventory/items/{id}/clone — access level ≥ 3
func (h *InventoryHandler) CloneInventoryItem(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 3 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 3 or higher required")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req models.CloneItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}
	if req.NewDescription == "" {
		Err(w, http.StatusBadRequest, "ERR_VALIDATION", "new_description is required")
		return
	}

	cloned, err := h.inventory.CloneItem(r.Context(), id, req.NewDescription)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "ERR_DUPLICATE_DESCRIPTION"):
			Err(w, http.StatusConflict, "ERR_DUPLICATE_DESCRIPTION", err.Error())
		default:
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}
	JSON(w, http.StatusCreated, cloned)
}

// POST /api/v1/inventory/items/{id}/barcode — access level ≥ 3
func (h *InventoryHandler) AssignBarcode(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 3 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 3 or higher required")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req models.AssignBarcodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}
	if req.Barcode == "" {
		Err(w, http.StatusBadRequest, "ERR_VALIDATION", "barcode is required")
		return
	}

	if err := h.inventory.AssignBarcode(r.Context(), id, req.Barcode); err != nil {
		switch {
		case strings.Contains(err.Error(), "ERR_DUPLICATE_BARCODE"):
			Err(w, http.StatusConflict, "ERR_DUPLICATE_BARCODE", err.Error())
		default:
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}
	JSON(w, http.StatusOK, map[string]any{"id": id, "barcode": req.Barcode, "cost_group": "DIR"})
}

// POST /api/v1/inventory/items/{id}/barcode/bulk — access level ≥ 3
func (h *InventoryHandler) AddLinkedBarcode(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 3 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 3 or higher required")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req models.AssignBarcodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}
	if req.Barcode == "" {
		Err(w, http.StatusBadRequest, "ERR_VALIDATION", "barcode is required")
		return
	}

	if err := h.inventory.AddLinkedBarcode(r.Context(), id, req.Barcode); err != nil {
		switch {
		case strings.Contains(err.Error(), "ERR_DUPLICATE_BARCODE"):
			Err(w, http.StatusConflict, "ERR_DUPLICATE_BARCODE", err.Error())
		default:
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}
	JSON(w, http.StatusOK, map[string]any{"id": id, "barcode": req.Barcode})
}

// ─── Stock-Take ───────────────────────────────────────────────────────────────

// GET /api/v1/inventory/stock-take?stock_sheet=...&day_of_week=... — access level ≥ 1
func (h *InventoryHandler) GetStockTakeSheet(w http.ResponseWriter, r *http.Request) {
	stockSheet := r.URL.Query().Get("stock_sheet")
	dayOfWeek := r.URL.Query().Get("day_of_week")

	if stockSheet == "" {
		Err(w, http.StatusBadRequest, "ERR_VALIDATION", "stock_sheet query param is required")
		return
	}
	if dayOfWeek == "" {
		Err(w, http.StatusBadRequest, "ERR_VALIDATION", "day_of_week query param is required")
		return
	}

	rows, err := h.stockTake.GetStockTakeSheet(r.Context(), stockSheet, dayOfWeek)
	if err != nil {
		if strings.Contains(err.Error(), "invalid day_of_week") {
			Err(w, http.StatusBadRequest, "ERR_VALIDATION", err.Error())
		} else {
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}
	if rows == nil {
		rows = []models.StockTakeDayRow{}
	}

	// Compute totals in Go
	var totOpening, totReceived, totClosing, totSales, totVariance float64
	for _, row := range rows {
		totOpening += row.OpeningStock
		totReceived += row.Received
		totClosing += row.ClosingStock
		totSales += row.Sales
		totVariance += row.Variance
	}

	JSON(w, http.StatusOK, map[string]any{
		"stock_sheet": stockSheet,
		"day_of_week": dayOfWeek,
		"items":       rows,
		"totals": map[string]any{
			"opening_stock": totOpening,
			"received":      totReceived,
			"closing_stock": totClosing,
			"sales":         totSales,
			"variance":      totVariance,
		},
	})
}

// PUT /api/v1/inventory/stock-take/closing — access level ≥ 2
func (h *InventoryHandler) UpdateClosingStock(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 2 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 2 or higher required")
		return
	}

	var req models.UpdateClosingStockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}

	updated, err := h.stockTake.UpdateClosingStock(r.Context(), req)
	if err != nil {
		if strings.Contains(err.Error(), "invalid day_of_week") {
			Err(w, http.StatusBadRequest, "ERR_VALIDATION", err.Error())
		} else {
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}
	JSON(w, http.StatusOK, map[string]any{"updated": updated})
}

// POST /api/v1/inventory/stock-take/finalize — access level ≥ 5
func (h *InventoryHandler) FinalizeStockPeriod(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 5 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 5 required for period finalization")
		return
	}

	var body struct {
		StockSheet string `json:"stock_sheet"`
		PeriodDate string `json:"period_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}
	if body.StockSheet == "" {
		Err(w, http.StatusBadRequest, "ERR_VALIDATION", "stock_sheet is required")
		return
	}
	periodDate, err := time.Parse("2006-01-02", body.PeriodDate)
	if err != nil {
		Err(w, http.StatusBadRequest, "ERR_VALIDATION", "period_date must be YYYY-MM-DD")
		return
	}

	userID, _ := middleware.GetUserID(r.Context())
	req := models.FinalizeStockRequest{
		StockSheet: body.StockSheet,
		PeriodDate: periodDate,
		UserID:     userID,
	}

	itemsReset, err := h.stockTake.FinalizeStockPeriod(r.Context(), req)
	if err != nil {
		if strings.Contains(err.Error(), "ERR_PERIOD_ALREADY_FINALIZED") {
			Err(w, http.StatusConflict, "ERR_PERIOD_ALREADY_FINALIZED", err.Error())
		} else {
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"stock_sheet":  body.StockSheet,
		"period_date":  body.PeriodDate,
		"items_reset":  itemsReset,
		"finalized_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// ─── GRV ─────────────────────────────────────────────────────────────────────

// POST /api/v1/inventory/grv — access level ≥ 3
func (h *InventoryHandler) CreateGrv(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 3 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 3 or higher required")
		return
	}

	var req models.CreateGrvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}
	if req.SupplierID == 0 {
		Err(w, http.StatusBadRequest, "ERR_VALIDATION", "supplier_id is required")
		return
	}
	if req.GrvDate.IsZero() {
		req.GrvDate = time.Now()
	}

	header, err := h.grv.CreateGrv(r.Context(), req)
	if err != nil {
		if strings.Contains(err.Error(), "ERR_VALIDATION") {
			Err(w, http.StatusBadRequest, "ERR_VALIDATION", err.Error())
		} else if strings.Contains(err.Error(), "not found") {
			Err(w, http.StatusUnprocessableEntity, "ERR_ITEM_NOT_FOUND", err.Error())
		} else {
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}

	// Resolve supplier name via SEPARATE single-table SELECT — Project Constitution §4 + Appendix A §7
	supplierName := resolveSupplierName(r.Context(), h.lookups, header.SupplierNo)

	JSON(w, http.StatusCreated, models.GrvResponse{
		GrvID:           header.OrderNo,
		SupplierID:      header.SupplierNo,
		SupplierName:    supplierName,
		GrvDate:         header.InvDate.Format("2006-01-02"),
		ReferenceNumber: req.ReferenceNumber,
		NettTotal:       header.NettTotal,
		VAT:             header.VAT,
		GrandTotal:      header.GrandTotal,
		LinesAccepted:   len(req.Lines),
	})
}

// resolveSupplierName gets the supplier name for a GRV response.
// Uses GetSuppliers (single-table SELECT) — never JOINs in the write transaction.
func resolveSupplierName(ctx context.Context, lookups repository.LookupRepository, supplierID int64) string {
	suppliers, err := lookups.GetSuppliers(ctx)
	if err != nil {
		return strconv.FormatInt(supplierID, 10)
	}
	for _, s := range suppliers {
		if s.SupplierNo == supplierID {
			return s.Name
		}
	}
	return strconv.FormatInt(supplierID, 10)
}

// ─── Wastage ──────────────────────────────────────────────────────────────────

// POST /api/v1/inventory/wastage — access level ≥ 2
func (h *InventoryHandler) RecordWastage(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 2 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 2 or higher required")
		return
	}

	var req models.RecordWastageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}

	line, err := h.wastage.RecordWastage(r.Context(), req)
	if err != nil {
		if strings.Contains(err.Error(), "ERR_VALIDATION") || strings.Contains(err.Error(), "not found") {
			Err(w, http.StatusBadRequest, "ERR_VALIDATION", err.Error())
		} else {
			Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		}
		return
	}

	nettCost := line.EachCost * line.Qty
	taxAmount := nettCost * (line.TaxRate / 100)

	JSON(w, http.StatusCreated, models.WastageResponse{
		WastageID:   line.OrderNo,
		ItemID:      line.MPartNo,
		Description: line.Description,
		Qty:         line.Qty,
		EachCost:    line.EachCost,
		NettCost:    round2dp(nettCost),
		VAT:         round2dp(taxAmount),
		TotalCost:   round2dp(nettCost + taxAmount),
		WastageDate: line.OrderDate.UTC().Format(time.RFC3339),
		Posted:      false,
	})
}

// POST /api/v1/inventory/wastage/post-pending — access level ≥ 5
func (h *InventoryHandler) PostPendingWastage(w http.ResponseWriter, r *http.Request) {
	if lvl := middleware.GetAccessLevel(r.Context()); lvl < 5 {
		Err(w, http.StatusForbidden, "ERR_FORBIDDEN", "Access level 5 required for posting wastage")
		return
	}

	stockSheet := r.URL.Query().Get("stock_sheet") // optional filter

	posted, err := h.wastage.PostPendingWastage(r.Context(), stockSheet)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	JSON(w, http.StatusOK, map[string]any{"posted": posted})
}

// ─── Reports ──────────────────────────────────────────────────────────────────

// GET /api/v1/inventory/reports/value?stock_sheet=... — access level ≥ 1
func (h *InventoryHandler) GetInventoryValue(w http.ResponseWriter, r *http.Request) {
	stockSheet := r.URL.Query().Get("stock_sheet")

	totalValue, err := h.inventory.GetInventoryValue(r.Context(), stockSheet)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	JSON(w, http.StatusOK, models.InventoryValueReport{
		StockSheet: stockSheet,
		TotalValue: totalValue,
	})
}

// GET /api/v1/inventory/reports/variance?stock_sheet=... — access level ≥ 1
func (h *InventoryHandler) GetStockVariance(w http.ResponseWriter, r *http.Request) {
	stockSheet := r.URL.Query().Get("stock_sheet")

	lines, err := h.inventory.GetStockVariance(r.Context(), stockSheet)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	if lines == nil {
		lines = []models.StockVarianceLine{}
	}
	JSON(w, http.StatusOK, map[string]any{
		"stock_sheet": stockSheet,
		"items":       lines,
	})
}

// round2dp is a local helper (also defined in inventory_firebird.go in repository package).
func round2dp(v float64) float64 {
	f := v * 100
	if f < 0 {
		f -= 0.5
	} else {
		f += 0.5
	}
	return float64(int(f)) / 100
}
