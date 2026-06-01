// Package middleware holds cross-cutting HTTP middleware (CORS, request IDs,
// security headers) used to wrap the application router.
package middleware

import (
	"crypto/rand"
	"fmt"
	"net/http"
)

// CORS applies cross-origin resource sharing headers. When origins is empty the
// middleware allows any origin ("*"); otherwise only listed origins are echoed
// back and requests from other origins pass through without CORS headers.
type CORS struct {
	origins []string
}

// NewCORS builds a CORS middleware for the given allowed origins.
func NewCORS(origins []string) *CORS {
	return &CORS{origins: origins}
}

// Handler wraps next with CORS handling.
func (c *CORS) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := "*"
		if len(c.origins) > 0 {
			allowed = ""
			for _, o := range c.origins {
				if o == origin {
					allowed = origin
					break
				}
			}
			if allowed == "" {
				next.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", allowed)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequestID sets baseline security headers and ensures every request carries an
// X-Request-ID header (generating one when absent).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 8)
			rand.Read(b)
			id = fmt.Sprintf("%x", b)
		}
		r.Header.Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}
