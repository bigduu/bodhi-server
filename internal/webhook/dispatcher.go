package webhook

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type Webhook struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Secret    string   `json:"secret,omitempty"`
	IsActive  bool     `json:"is_active"`
	CreatedAt string   `json:"created_at"`
}

type Payload struct {
	Event     string      `json:"event"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

type Dispatcher struct {
	db      *sql.DB
	client  *http.Client
	queue   chan *dispatchJob
	mu      sync.RWMutex
	hooks   []*Webhook
}

type dispatchJob struct {
	webhook *Webhook
	payload *Payload
}

func NewDispatcher(db *sql.DB, bufferSize int) *Dispatcher {
	if bufferSize <= 0 {
		bufferSize = 128
	}
	d := &Dispatcher{
		db:     db,
		client: &http.Client{Timeout: 10 * time.Second},
		queue:  make(chan *dispatchJob, bufferSize),
	}
	d.Reload(context.Background())
	go d.drain()
	return d
}

func (d *Dispatcher) Reload(ctx context.Context) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id::text, url, events, COALESCE(secret, ''), is_active, created_at::text FROM webhooks WHERE is_active = true`)
	if err != nil {
		log.Printf("webhook: reload error: %v", err)
		return
	}
	defer rows.Close()

	var hooks []*Webhook
	for rows.Next() {
		w := &Webhook{}
		rows.Scan(&w.ID, &w.URL, &w.Events, &w.Secret, &w.IsActive, &w.CreatedAt)
		hooks = append(hooks, w)
	}
	d.mu.Lock()
	d.hooks = hooks
	d.mu.Unlock()
}

func (d *Dispatcher) Dispatch(event string, data interface{}) {
	d.mu.RLock()
	hooks := d.hooks
	d.mu.RUnlock()

	payload := &Payload{
		Event:     event,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}

	for _, w := range hooks {
		matched := false
		for _, e := range w.Events {
			if e == event || e == "*" {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		job := &dispatchJob{webhook: w, payload: payload}
		select {
		case d.queue <- job:
		default:
			log.Printf("webhook: queue full, dropping %s for %s", event, w.URL)
		}
	}
}

func (d *Dispatcher) drain() {
	for job := range d.queue {
		d.send(job.webhook, job.payload)
	}
}

func (d *Dispatcher) send(w *Webhook, p *Payload) {
	body, err := json.Marshal(p)
	if err != nil {
		log.Printf("webhook: marshal error: %v", err)
		return
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if w.Secret != "" {
		req.Header.Set("X-Webhook-Secret", w.Secret)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		log.Printf("webhook: delivery failed to %s: %v", w.URL, err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("webhook: %s returned %d", w.URL, resp.StatusCode)
	}
}

func CreateWebhook(ctx context.Context, db *sql.DB, url string, events []string, secret string) (*Webhook, error) {
	w := &Webhook{}
	err := db.QueryRowContext(ctx,
		`INSERT INTO webhooks (url, events, secret) VALUES ($1, $2, $3)
		 RETURNING id::text, url, events, COALESCE(secret, ''), is_active, created_at::text`,
		url, events, secret,
	).Scan(&w.ID, &w.URL, &w.Events, &w.Secret, &w.IsActive, &w.CreatedAt)
	return w, err
}

func ListWebhooks(ctx context.Context, db *sql.DB) ([]*Webhook, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id::text, url, events, COALESCE(secret, ''), is_active, created_at::text FROM webhooks ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Webhook
	for rows.Next() {
		w := &Webhook{}
		rows.Scan(&w.ID, &w.URL, &w.Events, &w.Secret, &w.IsActive, &w.CreatedAt)
		result = append(result, w)
	}
	return result, nil
}

func DeleteWebhook(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = $1::uuid`, id)
	return err
}

func ToggleWebhook(ctx context.Context, db *sql.DB, id string, active bool) error {
	_, err := db.ExecContext(ctx, `UPDATE webhooks SET is_active = $1 WHERE id = $2::uuid`, active, id)
	return err
}
