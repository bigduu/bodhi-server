package ratelimit

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type DistributedLimiter struct {
	db *sql.DB
}

func NewDistributed(db *sql.DB) *DistributedLimiter {
	return &DistributedLimiter{db: db}
}

// Allow atomically increments and checks the counter for the given key/window.
// Returns true if the request is allowed, false if limit exceeded.
func (dl *DistributedLimiter) Allow(ctx context.Context, key string, window time.Duration, limit int) (bool, error) {
	windowStart := time.Now().Truncate(window)

	var count int
	err := dl.db.QueryRowContext(ctx,
		`INSERT INTO distributed_rate_limits (key_prefix, window_start, count)
		 VALUES ($1, $2, 1)
		 ON CONFLICT (key_prefix, window_start) DO UPDATE SET count = distributed_rate_limits.count + 1
		 RETURNING count`,
		key, windowStart,
	).Scan(&count)
	if err != nil {
		return true, err
	}

	return count <= limit, nil
}

func (dl *DistributedLimiter) Cleanup(ctx context.Context) {
	threshold := time.Now().Add(-1 * time.Hour)
	dl.db.ExecContext(ctx,
		`DELETE FROM distributed_rate_limits WHERE window_start < $1`, threshold)
}

func (dl *DistributedLimiter) StartCleanup() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			dl.Cleanup(context.Background())
		}
	}()
}

// CheckRPM checks requests-per-minute limit for a user.
func (dl *DistributedLimiter) CheckRPM(ctx context.Context, userID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	key := fmt.Sprintf("rpm:%s", userID)
	allowed, err := dl.Allow(ctx, key, time.Minute, limit)
	if err != nil {
		return nil
	}
	if !allowed {
		return fmt.Errorf("RPM limit exceeded (%d)", limit)
	}
	return nil
}

// CheckRPD checks requests-per-day limit for a user.
func (dl *DistributedLimiter) CheckRPD(ctx context.Context, userID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	key := fmt.Sprintf("rpd:%s", userID)
	allowed, err := dl.Allow(ctx, key, 24*time.Hour, limit)
	if err != nil {
		return nil
	}
	if !allowed {
		return fmt.Errorf("daily request limit exceeded (%d)", limit)
	}
	return nil
}
