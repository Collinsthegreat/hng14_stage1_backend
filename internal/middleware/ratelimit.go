package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/Collinsthegreat/hng14_stage1_backend/pkg/response"
	"golang.org/x/time/rate"
)

// ─── Per-key limiter store ─────────────────────────────────────────────────────

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type limiterStore struct {
	mu       sync.Mutex
	limiters map[string]*limiterEntry
	r        rate.Limit
	b        int
}

func newLimiterStore(r rate.Limit, b int) *limiterStore {
	s := &limiterStore{
		limiters: make(map[string]*limiterEntry),
		r:        r,
		b:        b,
	}
	// Cleanup goroutine: evict entries idle for > 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.mu.Lock()
			for k, e := range s.limiters {
				if time.Since(e.lastSeen) > 5*time.Minute {
					delete(s.limiters, k)
				}
			}
			s.mu.Unlock()
		}
	}()
	return s
}

func (s *limiterStore) getLimiter(key string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.limiters[key]
	if !ok {
		e = &limiterEntry{limiter: rate.NewLimiter(s.r, s.b)}
		s.limiters[key] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// ─── Middleware factories ──────────────────────────────────────────────────────

// authLimiterStore: 10 req/min per IP for /auth/* routes.
// Token bucket: burst=10, refill=10/min.
var authLimiterStore = newLimiterStore(rate.Every(6*time.Second), 10)

// apiLimiterStore: 60 req/min per user_id for /api/* routes.
// Token bucket: burst=60, refill=60/min.
var apiLimiterStore = newLimiterStore(rate.Every(time.Second), 60)

// AuthRateLimit limits /auth/* to 10 requests/min per IP.
func AuthRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := realIP(r)
		if !authLimiterStore.getLimiter(ip).Allow() {
			response.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// APIRateLimit limits /api/* to 60 requests/min per user_id (falls back to IP).
func APIRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := UserIDFromContext(r.Context())
		if key == "" {
			key = realIP(r)
		}
		if !apiLimiterStore.getLimiter(key).Allow() {
			response.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// realIP extracts the client IP, respecting common proxy headers.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// First IP in a comma-separated list
		for i := 0; i < len(ip); i++ {
			if ip[i] == ',' {
				return ip[:i]
			}
		}
		return ip
	}
	// Strip port from RemoteAddr
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
