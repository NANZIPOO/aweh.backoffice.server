package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/aweh-pos/gateway/internal/models"
	"github.com/aweh-pos/gateway/internal/repository"
)

type SalesMenuHandler struct {
	repo repository.SalesMenuRepository
}

func NewSalesMenuHandler(repo repository.SalesMenuRepository) *SalesMenuHandler {
	return &SalesMenuHandler{repo: repo}
}

// ── Read Handlers ────────────────────────────────────────────────────────────

func (h *SalesMenuHandler) GetGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.repo.GetGroups(r.Context())
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	if groups == nil {
		groups = []models.SalesMenuGroup{}
	}
	JSON(w, http.StatusOK, groups)
}

func (h *SalesMenuHandler) GetItems(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.URL.Query().Get("group"))
	items, err := h.repo.GetItems(r.Context(), groupID)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	if items == nil {
		items = []models.SalesMenuItem{}
	}
	JSON(w, http.StatusOK, items)
}

func (h *SalesMenuHandler) GetItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid id: must be an integer")
		return
	}

	item, err := h.repo.GetItem(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "ERR_NOT_FOUND") {
			Err(w, http.StatusNotFound, "ERR_NOT_FOUND", err.Error())
			return
		}
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}
	JSON(w, http.StatusOK, item)
}

// ── Write Handlers: Groups ───────────────────────────────────────────────────

func (h *SalesMenuHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var group models.SalesMenuGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if strings.TrimSpace(group.ID) == "" {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "id (category) is required")
		return
	}

	if err := h.repo.CreateGroup(r.Context(), &group); err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}

	JSON(w, http.StatusCreated, group)
}

func (h *SalesMenuHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "id is required")
		return
	}

	var group models.SalesMenuGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if err := h.repo.UpdateGroup(r.Context(), id, &group); err != nil {
		if strings.Contains(err.Error(), "ERR_NOT_FOUND") {
			Err(w, http.StatusNotFound, "ERR_NOT_FOUND", err.Error())
			return
		}
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}

	JSON(w, http.StatusOK, group)
}

func (h *SalesMenuHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "id is required")
		return
	}

	if err := h.repo.DeleteGroup(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "ERR_NOT_FOUND") {
			Err(w, http.StatusNotFound, "ERR_NOT_FOUND", err.Error())
			return
		}
		if strings.Contains(err.Error(), "ERR_CONFLICT") {
			Err(w, http.StatusConflict, "ERR_CONFLICT", err.Error())
			return
		}
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── Write Handlers: Items ────────────────────────────────────────────────────

func (h *SalesMenuHandler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var item models.SalesMenuItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if strings.TrimSpace(item.Label) == "" {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "label (description) is required")
		return
	}

	recordNo, err := h.repo.CreateItem(r.Context(), &item)
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}

	item.ID = recordNo
	JSON(w, http.StatusCreated, item)
}

func (h *SalesMenuHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid id: must be an integer")
		return
	}

	var item models.SalesMenuItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if err := h.repo.UpdateItem(r.Context(), id, &item); err != nil {
		if strings.Contains(err.Error(), "ERR_NOT_FOUND") {
			Err(w, http.StatusNotFound, "ERR_NOT_FOUND", err.Error())
			return
		}
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}

	item.ID = id
	JSON(w, http.StatusOK, item)
}

func (h *SalesMenuHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid id: must be an integer")
		return
	}

	if err := h.repo.DeleteItem(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "ERR_NOT_FOUND") {
			Err(w, http.StatusNotFound, "ERR_NOT_FOUND", err.Error())
			return
		}
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
