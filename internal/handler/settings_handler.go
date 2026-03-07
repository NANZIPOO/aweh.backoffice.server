package handler

import (
	"encoding/json"
	"net/http"

	"github.com/aweh-pos/gateway/internal/models"
	"github.com/aweh-pos/gateway/internal/repository"
)

type SettingsHandler struct {
	repo *repository.SettingsRepository
}

func NewSettingsHandler(repo *repository.SettingsRepository) *SettingsHandler {
	return &SettingsHandler{repo: repo}
}

// GetCoreSetup handles GET /api/v1/settings/core-setup
func (h *SettingsHandler) GetCoreSetup(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.GetCoreSetup(r.Context())
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to fetch core setup settings: "+err.Error())
		return
	}
	JSON(w, http.StatusOK, settings)
}

// SaveCoreSetup handles PUT /api/v1/settings/core-setup
func (h *SettingsHandler) SaveCoreSetup(w http.ResponseWriter, r *http.Request) {
	var settings models.CoreSetupSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "Invalid request body: "+err.Error())
		return
	}
	
	if err := h.repo.SaveCoreSetup(r.Context(), &settings); err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to save core setup settings: "+err.Error())
		return
	}
	
	JSON(w, http.StatusOK, map[string]any{
		"message": "Core setup settings saved successfully",
		"data":    settings,
	})
}

// GetBusinessProfile handles GET /api/v1/settings/business-profile
func (h *SettingsHandler) GetBusinessProfile(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.GetBusinessProfile(r.Context())
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to fetch business profile settings: "+err.Error())
		return
	}
	JSON(w, http.StatusOK, settings)
}

// SaveBusinessProfile handles PUT /api/v1/settings/business-profile
func (h *SettingsHandler) SaveBusinessProfile(w http.ResponseWriter, r *http.Request) {
	var settings models.BusinessProfileSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "Invalid request body: "+err.Error())
		return
	}
	
	if err := h.repo.SaveBusinessProfile(r.Context(), &settings); err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to save business profile settings: "+err.Error())
		return
	}
	
	JSON(w, http.StatusOK, map[string]any{
		"message": "Business profile settings saved successfully",
		"data":    settings,
	})
}

// GetFinancialControl handles GET /api/v1/settings/financial-control
func (h *SettingsHandler) GetFinancialControl(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.GetFinancialControl(r.Context())
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to fetch financial control settings: "+err.Error())
		return
	}
	JSON(w, http.StatusOK, settings)
}

// SaveFinancialControl handles PUT /api/v1/settings/financial-control
func (h *SettingsHandler) SaveFinancialControl(w http.ResponseWriter, r *http.Request) {
	var settings models.FinancialControlSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "Invalid request body: "+err.Error())
		return
	}
	
	if err := h.repo.SaveFinancialControl(r.Context(), &settings); err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to save financial control settings: "+err.Error())
		return
	}
	
	JSON(w, http.StatusOK, map[string]any{
		"message": "Financial control settings saved successfully",
		"data":    settings,
	})
}

// GetSecurityAccess handles GET /api/v1/settings/security-access
func (h *SettingsHandler) GetSecurityAccess(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.GetSecurityAccess(r.Context())
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to fetch security settings: "+err.Error())
		return
	}
	JSON(w, http.StatusOK, settings)
}

// SaveSecurityAccess handles PUT /api/v1/settings/security-access
func (h *SettingsHandler) SaveSecurityAccess(w http.ResponseWriter, r *http.Request) {
	var settings models.SecurityAccessSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "Invalid request body: "+err.Error())
		return
	}
	
	if err := h.repo.SaveSecurityAccess(r.Context(), &settings); err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to save security settings: "+err.Error())
		return
	}
	
	JSON(w, http.StatusOK, map[string]any{
		"message": "Security settings saved successfully",
		"data":    settings,
	})
}

// ChangeAppKey handles PUT /api/v1/settings/security-access/appkey
func (h *SettingsHandler) ChangeAppKey(w http.ResponseWriter, r *http.Request) {
	var req models.AppKeyChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "Invalid request body: "+err.Error())
		return
	}
	
	if err := h.repo.ChangeAppKey(r.Context(), req.CurrentKey, req.NewKey); err != nil {
		Err(w, http.StatusBadRequest, "ERR_VALIDATION", err.Error())
		return
	}
	
	JSON(w, http.StatusOK, map[string]string{
		"message": "AppKey changed successfully",
	})
}

// GetDeviceTerminal handles GET /api/v1/settings/device-terminal
func (h *SettingsHandler) GetDeviceTerminal(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.GetDeviceTerminal(r.Context())
	if err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to fetch device terminal settings: "+err.Error())
		return
	}
	JSON(w, http.StatusOK, settings)
}

// SaveDeviceTerminal handles PUT /api/v1/settings/device-terminal
func (h *SettingsHandler) SaveDeviceTerminal(w http.ResponseWriter, r *http.Request) {
	var settings models.DeviceTerminalSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		Err(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "Invalid request body: "+err.Error())
		return
	}
	
	if err := h.repo.SaveDeviceTerminal(r.Context(), &settings); err != nil {
		Err(w, http.StatusInternalServerError, "ERR_INTERNAL", "Failed to save device terminal settings: "+err.Error())
		return
	}
	
	JSON(w, http.StatusOK, map[string]any{
		"message": "Device terminal settings saved successfully",
		"data":    settings,
	})
}
