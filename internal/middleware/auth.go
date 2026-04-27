package middleware

import (
	"context"
	"net/http"
	"os"
	"strings"

	"log/slog"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/repository"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/service"
	"github.com/Collinsthegreat/hng14_stage1_backend/pkg/response"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const (
	ContextKeyUserID   contextKey = "user_id"
	ContextKeyUsername contextKey = "username"
	ContextKeyRole     contextKey = "role"
)

// UserIDFromContext extracts the user_id injected by JWTAuth.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyUserID).(string)
	return v
}

// RoleFromContext extracts the role injected by JWTAuth.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyRole).(string)
	return v
}

// JWTAuth validates the Bearer token (from Authorization header or access_token cookie),
// checks is_active via the database, and injects user claims into the request context.
func JWTAuth(userRepo repository.UserRepository) func(http.Handler) http.Handler {
	secret := []byte(os.Getenv("JWT_SECRET"))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearerToken(r)
			if raw == "" {
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			claims, err := service.ParseAndValidateJWT(raw, secret)
			if err != nil {
				slog.Error("JWT validation failed", "error", err)
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			// Verify the user is still active in the database
			u, err := userRepo.GetUserByID(r.Context(), claims.UserID)
			if err != nil {
				slog.Error("DB user lookup failed", "error", err, "user_id", claims.UserID)
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if u == nil {
				slog.Error("User not found in DB", "user_id", claims.UserID)
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if !u.IsActive {
				response.Error(w, http.StatusForbidden, "account disabled")
				return
			}

			// Inject claims into context
			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyUsername, claims.Username)
			ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearerToken gets the token from the Authorization header or access_token cookie.
func extractBearerToken(r *http.Request) string {
	// Authorization: Bearer <token>
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// Fallback: HTTP-only cookie set by web portal flow
	if cookie, err := r.Cookie("access_token"); err == nil {
		return cookie.Value
	}
	return ""
}
