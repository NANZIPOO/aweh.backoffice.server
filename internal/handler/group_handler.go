package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/aweh-pos/gateway/internal/models"
	"github.com/aweh-pos/gateway/internal/repository"
)

// ProductGroupHandlers contains all handlers for inventory groups.
type ProductGroupHandlers struct {
	groupRepo repository.GroupRepository
	invRepo   repository.InventoryRepository
}

func NewProductGroupHandlers(groupRepo repository.GroupRepository, invRepo repository.InventoryRepository) *ProductGroupHandlers {
	return &ProductGroupHandlers{
		groupRepo: groupRepo,
		invRepo:   invRepo,
	}
}

// ─── POST /api/v1/inventory/groups ────────────────────────────────────────

type CreateGroupRequest struct {
	GroupName string               `json:"group_name" validate:"required"`
	BaseUOM   string               `json:"base_uom" validate:"required"`
	Variants  []CreateGroupVariant `json:"variants" validate:"required,min=1"`
}

type CreateGroupVariant struct {
	Description     string  `json:"description" validate:"required"`
	SupplierNo      string  `json:"supplier_no" validate:"required"`
	UOM             string  `json:"uom" validate:"required"`
	Pack            float64 `json:"pack_size"`
	PackUnit        string  `json:"pack_unit"`
	Units           float64 `json:"units" validate:"required,gt=0"`
	EachUnit        string  `json:"each_unit" validate:"required"`
	SellingPrice    float64 `json:"selling_price" validate:"required,gte=0"`
	PackCost        float64 `json:"pack_cost"`
	EachCost        float64 `json:"each_cost"`
	Markup          float64 `json:"markup" validate:"gte=0"`
	TaxRate         float64 `json:"tax_rate" validate:"gte=0"`
	IsSellable      bool    `json:"is_sellable"`
	OrderingAllowed *bool   `json:"ordering_allowed,omitempty"`
}

func (h *ProductGroupHandlers) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Convert request variants to models
	variants := make([]models.InventoryItem, len(req.Variants))
	for i, v := range req.Variants {
		orderingAllowed := true
		if v.OrderingAllowed != nil {
			orderingAllowed = *v.OrderingAllowed
		}
		variants[i] = models.InventoryItem{
			Description:       v.Description,
			SupplierNo:        v.SupplierNo,
			UOM:               models.NullStringFrom(v.UOM),
			Pack:              v.Pack,
			PackUnit:          v.PackUnit,
			EachCost:          v.EachCost,
			Units:             v.Units,
			EachUnit:          v.EachUnit,
			SellingPrice:      v.SellingPrice,
			PackCost:          v.PackCost,
			Markup:            v.Markup,
			TaxRate:           v.TaxRate,
			IsSellable:        v.IsSellable,
			IsOrderingAllowed: orderingAllowed,
		}
	}

	groupID, err := h.groupRepo.CreateGroup(r.Context(), req.GroupName, req.BaseUOM, variants)
	if err != nil {
		Err(w, http.StatusInternalServerError, "CREATE_GROUP_FAILED", err.Error())
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"group_id":   groupID,
		"group_name": req.GroupName,
		"base_uom":   req.BaseUOM,
		"variants":   len(variants),
	})
}

// ─── GET /api/v1/inventory/groups/:group_id ──────────────────────────────

func (h *ProductGroupHandlers) GetGroup(w http.ResponseWriter, r *http.Request) {
	groupIDStr := r.PathValue("group_id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "INVALID_GROUP_ID", "group_id must be numeric")
		return
	}

	group, variants, err := h.groupRepo.GetGroupWithVariants(r.Context(), groupID)
	if err != nil {
		Err(w, http.StatusNotFound, "GROUP_NOT_FOUND", err.Error())
		return
	}

	// Get base quantity
	baseQty, err := h.groupRepo.GetGroupBaseQty(r.Context(), groupID)
	if err != nil {
		Err(w, http.StatusInternalServerError, "FETCH_BASE_QTY_FAILED", err.Error())
		return
	}

	// Calculate quantities for each variant
	for i := range variants {
		qty, _ := h.groupRepo.GetVariantCalculatedQty(r.Context(), variants[i].ItemPartNo, baseQty)
		variants[i].CalculatedQty = qty
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"group":    group,
		"base_qty": baseQty,
		"variants": variants,
		"count":    len(variants),
	})
}

// ─── POST /api/v1/inventory/groups/:group_id/link-item ──────────────────

type LinkItemRequest struct {
	ItemID int64 `json:"item_id" validate:"required,gt=0"`
}

func (h *ProductGroupHandlers) LinkItem(w http.ResponseWriter, r *http.Request) {
	groupIDStr := r.PathValue("group_id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "INVALID_GROUP_ID", "group_id must be numeric")
		return
	}

	var req LinkItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	err = h.groupRepo.LinkItemToGroup(r.Context(), req.ItemID, groupID)
	if err != nil {
		Err(w, http.StatusInternalServerError, "LINK_FAILED", err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"status":   "linked",
		"item_id":  req.ItemID,
		"group_id": groupID,
	})
}

// ─── POST /api/v1/inventory/groups/:group_id/add-variant ─────────────────

func (h *ProductGroupHandlers) AddVariant(w http.ResponseWriter, r *http.Request) {
	groupIDStr := r.PathValue("group_id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "INVALID_GROUP_ID", "group_id must be numeric")
		return
	}

	var req models.AddVariantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Validate required fields
	if req.Description == "" {
		Err(w, http.StatusBadRequest, "VALIDATION_FAILED", "description is required")
		return
	}
	if req.UOM == "" {
		Err(w, http.StatusBadRequest, "VALIDATION_FAILED", "uom is required")
		return
	}

	newItemID, err := h.groupRepo.AddVariantToGroup(r.Context(), groupID, req)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ADD_VARIANT_FAILED", err.Error())
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"status":      "variant_created",
		"item_id":     newItemID,
		"group_id":    groupID,
		"description": req.Description,
		"uom":         req.UOM,
	})
}

// ─── POST /api/v1/inventory/items/:item_id/add-variant ────────────────────

func (h *ProductGroupHandlers) AddVariantFromItem(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("item_id")
	itemID, err := strconv.ParseInt(itemIDStr, 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "INVALID_ITEM_ID", "item_id must be numeric")
		return
	}

	var req models.AddVariantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Description == "" {
		Err(w, http.StatusBadRequest, "VALIDATION_FAILED", "description is required")
		return
	}
	if req.UOM == "" {
		Err(w, http.StatusBadRequest, "VALIDATION_FAILED", "uom is required")
		return
	}

	groupID, err := h.groupRepo.EnsureGroupForItem(r.Context(), itemID)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ENSURE_GROUP_FAILED", err.Error())
		return
	}

	newItemID, err := h.groupRepo.AddVariantToGroup(r.Context(), groupID, req)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ADD_VARIANT_FAILED", err.Error())
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"status":      "variant_created",
		"item_id":     newItemID,
		"group_id":    groupID,
		"description": req.Description,
		"uom":         req.UOM,
	})
}

// ─── DELETE /api/v1/inventory/groups/:group_id/unlink/:item_id ────────────

func (h *ProductGroupHandlers) UnlinkItem(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("item_id")
	itemID, err := strconv.ParseInt(itemIDStr, 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "INVALID_ITEM_ID", "item_id must be numeric")
		return
	}

	err = h.groupRepo.UnlinkItemFromGroup(r.Context(), itemID)
	if err != nil {
		Err(w, http.StatusInternalServerError, "UNLINK_FAILED", err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"status":  "unlinked",
		"item_id": itemID,
	})
}

// ─── POST /api/v1/inventory/groups/:group_id/change-base-unit ────────────

type ChangeBaseUnitRequest struct {
	NewBaseItemID int64 `json:"new_base_item_id" validate:"required,gt=0"`
}

func (h *ProductGroupHandlers) ChangeBaseUnit(w http.ResponseWriter, r *http.Request) {
	groupIDStr := r.PathValue("group_id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "INVALID_GROUP_ID", "group_id must be numeric")
		return
	}

	var req ChangeBaseUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	err = h.groupRepo.ChangeBaseUnit(r.Context(), groupID, req.NewBaseItemID)
	if err != nil {
		Err(w, http.StatusInternalServerError, "CHANGE_BASE_UNIT_FAILED", err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"status":        "base_unit_changed",
		"group_id":      groupID,
		"new_base_item": req.NewBaseItemID,
	})
}

// ─── GET /api/v1/inventory/groups/:group_id/movements ────────────────────

func (h *ProductGroupHandlers) GetMovements(w http.ResponseWriter, r *http.Request) {
	groupIDStr := r.PathValue("group_id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "INVALID_GROUP_ID", "group_id must be numeric")
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		offset, _ = strconv.Atoi(o)
	}

	movements, err := h.groupRepo.GetMovements(r.Context(), groupID, limit, offset)
	if err != nil {
		Err(w, http.StatusInternalServerError, "FETCH_MOVEMENTS_FAILED", err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"movements": movements,
		"count":     len(movements),
		"limit":     limit,
		"offset":    offset,
	})
}

// ─── DELETE /api/v1/inventory/groups/:group_id ────────────────────────────

func (h *ProductGroupHandlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupIDStr := r.PathValue("group_id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "INVALID_GROUP_ID", "group_id must be numeric")
		return
	}

	err = h.groupRepo.DeleteGroup(r.Context(), groupID)
	if err != nil {
		Err(w, http.StatusInternalServerError, "DELETE_GROUP_FAILED", err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"status":   "deleted",
		"group_id": groupID,
	})
}
