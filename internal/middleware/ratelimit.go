package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Collinsthegreat/hng14_stage1_backend/pkg/response"
	"golang.org/x/time/rate"
)

// ─── Per-key limiter store ─────────────────────────────────────────────────────

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64
}

type limiterStore struct {
	limiters sync.Map
	r        rate.Limit
	b        int
}

func newLimiterStore(r rate.Limit, b int) *limiterStore {
	s := &limiterStore{
		r: r,
		b: b,
	}
	// Cleanup goroutine: evict entries idle for > 5 minutes
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			s.limiters.Range(func(k, v any) bool {
				entry := v.(*limiterEntry)
				lastSeen := time.Unix(0, entry.lastSeen.Load())
				if now.Sub(lastSeen) > 5*time.Minute {
					s.limiters.Delete(k)
				}
				return true
			})
		}
	}()
	return s
}

func (s *limiterStore) getLimiter(key string) *rate.Limiter {
	now := time.Now().UnixNano()
	if v, ok := s.limiters.Load(key); ok {
		entry := v.(*limiterEntry)
		entry.lastSeen.Store(now)
		return entry.limiter
	}

	entry := &limiterEntry{limiter: rate.NewLimiter(s.r, s.b)}
	entry.lastSeen.Store(now)
	actual, _ := s.limiters.LoadOrStore(key, entry)
	stored := actual.(*limiterEntry)
	stored.lastSeen.Store(now)
	return stored.limiter
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
		ip := remoteIP(r)
		if isWhitelistedIP(ip) {
			next.ServeHTTP(w, r)
			return
		}
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
			key = remoteIP(r)
		}
		if !apiLimiterStore.getLimiter(key).Allow() {
			response.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// remoteIP extracts the client IP from RemoteAddr and strips the port.
func remoteIP(r *http.Request) string {
	addr := strings.TrimSpace(r.RemoteAddr)
	if addr == "" {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return addr
}

func isWhitelistedIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed != nil && parsed.IsLoopback() {
		return true
	}
	for _, allowed := range strings.Split(os.Getenv("RATE_LIMIT_WHITELIST"), ",") {
		if strings.TrimSpace(allowed) == ip {
			return true
		}
	}
	return false
}
