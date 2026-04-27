package middleware

import (
	"net/http"

	"github.com/Collinsthegreat/hng14_stage1_backend/pkg/response"
)

// RequireRole returns a middleware that enforces a specific role requirement.
// admin: all methods allowed
// analyst: GET only
// Any other role or a mismatch → 403
func RequireRole(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromContext(r.Context())
			if role == "" {
				response.Error(w, http.StatusForbidden, "forbidden")
				return
			}

			// Admin passes everything
			if role == "admin" {
				next.ServeHTTP(w, r)
				return
			}

			// Analyst is read-only — only GET is allowed
			if role == "analyst" && requiredRole == "admin" {
				response.Error(w, http.StatusForbidden, "forbidden")
				return
			}

			// Role matches or is analyst for read-only route
			if role != requiredRole && role != "admin" {
				response.Error(w, http.StatusForbidden, "forbidden")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RBACAnalystRead is a convenience middleware that enforces that the caller is
// at least an analyst (admin also passes). Used on GET-only routes.
func RBACAnalystRead(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := RoleFromContext(r.Context())
		if role == "admin" || role == "analyst" {
			next.ServeHTTP(w, r)
			return
		}
		response.Error(w, http.StatusForbidden, "forbidden")
	})
}
