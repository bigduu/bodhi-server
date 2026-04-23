package security

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type IPLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipBucket
	rate    float64
	burst   int
}

type ipBucket struct {
	tokens  float64
	lastHit time.Time
}

// NewIPLimiter creates a token bucket limiter keyed by client IP.
// rpm = requests per minute, burst = max burst size.
func NewIPLimiter(rpm, burst int) *IPLimiter {
	return &IPLimiter{
		buckets: make(map[string]*ipBucket),
		rate:    float64(rpm) / 60.0,
		burst:   burst,
	}
}

func (l *IPLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !l.allow(ip) {
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func (l *IPLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &ipBucket{tokens: float64(l.burst) - 1, lastHit: now}
		return true
	}

	elapsed := now.Sub(b.lastHit).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.lastHit = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *IPLimiter) StartCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			l.mu.Lock()
			threshold := time.Now().Add(-10 * time.Minute)
			for k, b := range l.buckets {
				if b.lastHit.Before(threshold) {
					delete(l.buckets, k)
				}
			}
			l.mu.Unlock()
		}
	}()
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if idx := strings.IndexByte(ip, ','); idx > 0 {
			return ip[:idx]
		}
		return ip
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}
