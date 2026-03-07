package models

// ErrorResponse is the standard JSON error envelope for all 4xx/5xx responses.
// Format: {"error": "ERR_CODE", "code": "ERR_CODE", "message": "human readable"}
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PaginatedResponse is a generic paginated response container.
type PaginatedResponse[T any] struct {
	Data []T            `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

// PaginationMeta carries pagination metadata returned with list endpoints.
type PaginationMeta struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Pages int `json:"pages"`
}
