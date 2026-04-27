package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

// Logger logs every HTTP request with method, path, status, latency, and user_id.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		t1 := time.Now()
		defer func() {
			userID := UserIDFromContext(r.Context())
			slog.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"response_time_ms", time.Since(t1).Milliseconds(),
				"user_id", userID,
				"remote_addr", r.RemoteAddr,
			)
		}()
		next.ServeHTTP(ww, r)
	})
}
