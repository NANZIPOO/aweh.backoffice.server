package handler

import (
	"encoding/json"
	"net/http"

	"github.com/aweh-pos/gateway/internal/models"
)

// JSON writes a standard {"data": ...} JSON envelope with the given HTTP status.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"data": data})
}

// Err writes a standard {"error":...,"code":...,"message":...} JSON envelope.
func Err(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(models.ErrorResponse{
		Error:   code,
		Code:    code,
		Message: message,
	})
}
