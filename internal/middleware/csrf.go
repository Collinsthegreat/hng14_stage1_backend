package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/Collinsthegreat/hng14_stage1_backend/pkg/response"
)

// CSRF middleware protects mutating endpoints (POST, DELETE, PUT, PATCH)
// against Cross-Site Request Forgery.
// It requires an X-CSRF-Token header that matches the csrf_token cookie.
// If the client does not send a csrf_token cookie (e.g., a CLI client using Bearer tokens),
// the check is bypassed.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only check mutating methods
		if r.Method != http.MethodPost && r.Method != http.MethodDelete && r.Method != http.MethodPut && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie("csrf_token")
		if err != nil || cookie.Value == "" {
			// No CSRF cookie present, assume CLI/API client (not a browser)
			next.ServeHTTP(w, r)
			return
		}

		headerToken := r.Header.Get("X-CSRF-Token")
		if headerToken == "" || headerToken != cookie.Value {
			response.Error(w, http.StatusForbidden, "invalid csrf token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GenerateCSRFToken generates a cryptographically secure random token.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
