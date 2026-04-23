package security

import (
	"net/http"
	"sync"
	"time"
)

type LoginGuard struct {
	mu       sync.Mutex
	attempts map[string]*attemptInfo
	maxTries int
	window   time.Duration
}

type attemptInfo struct {
	count    int
	firstTry time.Time
}

// NewLoginGuard blocks an IP after maxTries failed attempts within window.
func NewLoginGuard(maxTries int, window time.Duration) *LoginGuard {
	g := &LoginGuard{
		attempts: make(map[string]*attemptInfo),
		maxTries: maxTries,
		window:   window,
	}
	g.startCleanup()
	return g
}

// Check returns an error message if the IP is temporarily blocked.
func (g *LoginGuard) Check(r *http.Request) (ok bool) {
	ip := clientIP(r)
	g.mu.Lock()
	defer g.mu.Unlock()

	info, ok := g.attempts[ip]
	if !ok {
		return true
	}
	if time.Since(info.firstTry) > g.window {
		delete(g.attempts, ip)
		return true
	}
	if info.count >= g.maxTries {
		return false
	}
	return true
}

// RecordFail increments the failure counter for this IP.
func (g *LoginGuard) RecordFail(r *http.Request) {
	ip := clientIP(r)
	g.mu.Lock()
	defer g.mu.Unlock()

	info, ok := g.attempts[ip]
	if !ok || time.Since(info.firstTry) > g.window {
		g.attempts[ip] = &attemptInfo{count: 1, firstTry: time.Now()}
		return
	}
	info.count++
}

// Reset clears the failure counter for this IP (on successful login).
func (g *LoginGuard) Reset(r *http.Request) {
	ip := clientIP(r)
	g.mu.Lock()
	delete(g.attempts, ip)
	g.mu.Unlock()
}

func (g *LoginGuard) startCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			g.mu.Lock()
			now := time.Now()
			for ip, info := range g.attempts {
				if now.Sub(info.firstTry) > g.window {
					delete(g.attempts, ip)
				}
			}
			g.mu.Unlock()
		}
	}()
}
