package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	TenantIDKey contextKey = "tenant_id"
	UserNoKey   contextKey = "user_no"
	// UserIDKey, UsernameKey, AccessLevelKey are in context_keys.go (same package).
)

// Claims carries all JWT payload fields.
// Legacy fields (TenantID, UserNo) are kept for backward compatibility with existing
// bill/employee/PO endpoints. Inventory endpoints use UserID, Username, and AccessLevel.
type Claims struct {
	TenantID    string `json:"tenant_id"`
	UserNo      int16  `json:"user_no"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	AccessLevel int    `json:"access_level"`
	jwt.RegisteredClaims
}

// AuthMiddleware extracts JWT and injects tenant_id into context
func AuthMiddleware(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Missing Authorization Header", http.StatusUnauthorized)
				return
			}

			bearerToken := strings.TrimPrefix(authHeader, "Bearer ")
			claims := &Claims{}

			token, err := jwt.ParseWithClaims(bearerToken, claims, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return jwtSecret, nil
			})

			if err != nil || !token.Valid {
				http.Error(w, "Invalid Token", http.StatusUnauthorized)
				return
			}

			// Store legacy fields (tenant_id, user_no)
			ctx := context.WithValue(r.Context(), TenantIDKey, claims.TenantID)
			ctx = context.WithValue(ctx, UserNoKey, claims.UserNo)
			// Store inventory fields (user_id, username, access_level)
			ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, UsernameKey, claims.Username)
			ctx = context.WithValue(ctx, AccessLevelKey, claims.AccessLevel)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Helpers to get values from context
func GetTenantID(ctx context.Context) (string, error) {
	val, ok := ctx.Value(TenantIDKey).(string)
	if !ok {
		return "", errors.New("tenant_id not found in context")
	}
	return val, nil
}

func GetUserNo(ctx context.Context) (int16, error) {
	val, ok := ctx.Value(UserNoKey).(int16)
	if !ok {
		return 0, errors.New("user_no not found in context")
	}
	return val, nil
}

// GenerateToken creates a new JWT for a tenant/user pair
func GenerateToken(tenantID string, userNo int16, jwtSecret []byte) (string, error) {
	claims := &Claims{
		TenantID: tenantID,
		UserNo:   userNo,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}
// GenerateFullToken creates a JWT with all claim fields including access_level.
// Used by inventory and future modules that require role-based access checking.
func GenerateFullToken(tenantID string, userNo int16, userID int64, username string, accessLevel int, jwtSecret []byte) (string, error) {
	claims := &Claims{
		TenantID:    tenantID,
		UserNo:      userNo,
		UserID:      userID,
		Username:    username,
		AccessLevel: accessLevel,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// GetUserID extracts the user_id from context (set by AuthMiddleware).
func GetUserID(ctx context.Context) (int64, error) {
	val, ok := ctx.Value(UserIDKey).(int64)
	if !ok {
		return 0, errors.New("user_id not found in context")
	}
	return val, nil
}

// GetUsername extracts the username from context (set by AuthMiddleware).
func GetUsername(ctx context.Context) (string, error) {
	val, ok := ctx.Value(UsernameKey).(string)
	if !ok {
		return "", errors.New("username not found in context")
	}
	return val, nil
}

// GetAccessLevel extracts the access_level integer from context (set by AuthMiddleware).
// Returns 0 (no access) if not present — safe default.
func GetAccessLevel(ctx context.Context) int {
	val, _ := ctx.Value(AccessLevelKey).(int)
	return val
}

// RequireLevel returns a middleware that rejects requests with access_level < level.
// Use by wrapping a handler: middleware.RequireLevel(3)(myHandler)
func RequireLevel(level int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if GetAccessLevel(r.Context()) < level {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, `{"error":"ERR_FORBIDDEN","code":"ERR_FORBIDDEN","message":"Access level %d or higher required"}`, level)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}