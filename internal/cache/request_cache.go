package cache

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type CacheEntry struct {
	ID           string
	CacheKey     string
	Model        string
	ResponseBody []byte
	InputTokens  int
	OutputTokens int
	CostCents    int
	HitCount     int
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

func ComputeKey(model string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(model))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func Get(ctx context.Context, db *sql.DB, key string) (*CacheEntry, error) {
	e := &CacheEntry{}
	err := db.QueryRowContext(ctx,
		`SELECT id::text, cache_key, model, response_body, input_tokens, output_tokens, cost_cents, hit_count, expires_at, created_at
		 FROM request_cache
		 WHERE cache_key = $1 AND expires_at > NOW()`, key,
	).Scan(&e.ID, &e.CacheKey, &e.Model, &e.ResponseBody, &e.InputTokens, &e.OutputTokens, &e.CostCents, &e.HitCount, &e.ExpiresAt, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	db.ExecContext(ctx, `UPDATE request_cache SET hit_count = hit_count + 1 WHERE cache_key = $1`, key)
	return e, nil
}

func Set(ctx context.Context, db *sql.DB, key, model string, body []byte, inputTokens, outputTokens, costCents int, ttl time.Duration) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO request_cache (cache_key, model, response_body, input_tokens, output_tokens, cost_cents, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW() + $7::interval)
		 ON CONFLICT (cache_key) DO UPDATE SET
		     response_body = EXCLUDED.response_body,
		     input_tokens = EXCLUDED.input_tokens,
		     output_tokens = EXCLUDED.output_tokens,
		     cost_cents = EXCLUDED.cost_cents,
		     hit_count = 1,
		     expires_at = EXCLUDED.expires_at`,
		key, model, body, inputTokens, outputTokens, costCents, fmt.Sprintf("%d seconds", int(ttl.Seconds())))
	return err
}

func IsCacheable(body []byte) bool {
	var req struct {
		Stream    bool          `json:"stream"`
		Temp      *float64      `json:"temperature"`
		ToolCalls []interface{} `json:"tools"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	if req.Stream {
		return false
	}
	if req.Temp != nil && *req.Temp > 0 {
		return false
	}
	if len(req.ToolCalls) > 0 {
		return false
	}
	return true
}

func Cleanup(ctx context.Context, db *sql.DB) {
	result, err := db.ExecContext(ctx, `DELETE FROM request_cache WHERE expires_at <= NOW()`)
	if err != nil {
		log.Printf("cache cleanup error: %v", err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		log.Printf("cache cleanup: removed %d expired entries", n)
	}
}

func StartCleanupLoop(db *sql.DB) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			Cleanup(context.Background(), db)
		}
	}()
}
