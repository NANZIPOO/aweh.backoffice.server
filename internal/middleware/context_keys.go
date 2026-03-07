package middleware

// Inventory-module context key constants.
// These extend the existing TenantIDKey and UserNoKey defined in auth.go.
// The contextKey type is defined in auth.go (same package).
//
// All middleware context keys use this typed constant pattern to avoid
// string-key collisions between packages (Project Constitution Stage 1.2).
const (
	UserIDKey      contextKey = "user_id"
	UsernameKey    contextKey = "username"
	AccessLevelKey contextKey = "access_level"
)
