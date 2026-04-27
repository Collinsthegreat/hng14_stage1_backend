package middleware

import (
	"net/http"

	"github.com/Collinsthegreat/hng14_stage1_backend/pkg/response"
)

// APIVersion checks that the request carries the header X-API-Version: 1.
// Missing or incorrect value results in 400 "API version header required".
// This middleware is applied to all /api/* routes.
func APIVersion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Version") != "1" {
			response.Error(w, http.StatusBadRequest, "API version header required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
