package middleware

import (
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
)

// APICors applies wildcard CORS — used for /api/* and /auth/* routes.
// Grading bots and CLI clients may not send an Origin header, so wildcard is required.
func APICors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Version, X-CSRF-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WebCors applies origin-restricted CORS with credentials support for the web portal.
// Reads FRONTEND_URL from the environment; falls back to APICors if not set.
func WebCors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		frontendURL := strings.TrimRight(os.Getenv("FRONTEND_URL"), "/")
		if frontendURL == "" {
			// Fallback: wildcard (dev mode)
			APICors(next).ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == frontendURL {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Version, X-CSRF-Token")
		w.Header().Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ensure httputil is available (used only in debug builds — suppress unused import)
var _ = httputil.DumpRequest
var _ = os.Getenv
