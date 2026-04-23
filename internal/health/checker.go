package health

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"sync"
	"time"
)

type Checker struct {
	db       *sql.DB
	client   *http.Client
	mu       sync.Mutex
	statuses map[string]string
}

type instanceInfo struct {
	ID           string
	ProviderType string
	BaseURL      string
}

func NewChecker(db *sql.DB) *Checker {
	return &Checker{
		db:       db,
		client:   &http.Client{Timeout: 10 * time.Second},
		statuses: make(map[string]string),
	}
}

func (c *Checker) Start(interval time.Duration) {
	go c.run(interval)
}

func (c *Checker) run(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	c.checkAll()
	for range ticker.C {
		c.checkAll()
	}
}

func (c *Checker) checkAll() {
	rows, err := c.db.QueryContext(context.Background(),
		`SELECT id::text, provider_type, COALESCE(base_url, '') FROM provider_instances WHERE is_active = true`)
	if err != nil {
		log.Printf("health check: failed to list instances: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var inst instanceInfo
		rows.Scan(&inst.ID, &inst.ProviderType, &inst.BaseURL)
		status := c.checkInstance(inst)
		c.setStatus(inst.ID, status)

		c.db.ExecContext(context.Background(),
			`UPDATE provider_instances SET health_status = $1, last_check_at = NOW() WHERE id = $2::uuid`,
			status, inst.ID)
	}
}

func (c *Checker) checkInstance(inst instanceInfo) string {
	url := buildCheckURL(inst.ProviderType, inst.BaseURL)
	if url == "" {
		return "unknown"
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "unhealthy"
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "unhealthy"
	}
	resp.Body.Close()

	if resp.StatusCode < 500 {
		return "healthy"
	}
	return "unhealthy"
}

func buildCheckURL(providerType, baseURL string) string {
	switch providerType {
	case "openai":
		base := "https://api.openai.com"
		if baseURL != "" {
			base = baseURL
		}
		return base + "/v1/models"
	case "anthropic":
		return ""
	case "gemini":
		base := "https://generativelanguage.googleapis.com"
		if baseURL != "" {
			base = baseURL
		}
		return base + "/v1beta/models"
	}
	return ""
}

func (c *Checker) setStatus(instanceID, status string) {
	c.mu.Lock()
	c.statuses[instanceID] = status
	c.mu.Unlock()
}

func (c *Checker) GetStatus(instanceID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statuses[instanceID]
}

type CircuitBreaker struct {
	mu         sync.Mutex
	failures   map[string]int
	threshold  int
	resetAfter time.Duration
	lastFail   map[string]time.Time
}

func NewCircuitBreaker(threshold int, resetAfter time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failures:   make(map[string]int),
		threshold:  threshold,
		resetAfter: resetAfter,
		lastFail:   make(map[string]time.Time),
	}
}

func (cb *CircuitBreaker) RecordFailure(instanceID string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures[instanceID]++
	cb.lastFail[instanceID] = time.Now()
}

func (cb *CircuitBreaker) RecordSuccess(instanceID string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.failures, instanceID)
	delete(cb.lastFail, instanceID)
}

func (cb *CircuitBreaker) IsOpen(instanceID string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.failures[instanceID] >= cb.threshold {
		if last, ok := cb.lastFail[instanceID]; ok {
			if time.Since(last) > cb.resetAfter {
				cb.failures[instanceID] = cb.threshold - 1
				return false
			}
		}
		return true
	}
	return false
}
