package models

import (
	"context"
	"database/sql"
	"time"
)

type UsageRecord struct {
	ID             string
	UserID         string
	APIKeyID       sql.NullString
	Provider       string
	Model          sql.NullString
	Endpoint       sql.NullString
	InputTokens    int
	OutputTokens   int
	DurationMs     int
	StatusCode     int
	CostCents      sql.NullInt64
	IsError        bool
	IsCached       bool
	ErrorMessage   sql.NullString
	CreatedAt      time.Time
}

func RecordUsage(ctx context.Context, db *sql.DB, record *UsageRecord) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO usage_tracking (user_id, api_key_id, provider, model, endpoint,
		  input_tokens, output_tokens, duration_ms, status_code, cost_cents, is_error, is_cached, error_message)
		 VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		record.UserID, record.APIKeyID, record.Provider, record.Model, record.Endpoint,
		record.InputTokens, record.OutputTokens, record.DurationMs, record.StatusCode,
		record.CostCents, record.IsError, record.IsCached, record.ErrorMessage,
	)
	return err
}

// ... (keeping existing query types, adding new metrics queries)

type LatencyStats struct {
	P50 int `json:"p50"`
	P95 int `json:"p95"`
	P99 int `json:"p99"`
}

type ModelTokenUsage struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CostCents    int64  `json:"cost_cents"`
	Requests     int    `json:"requests"`
}

type ErrorEntry struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	StatusCode  int    `json:"status_code"`
	ErrorMessage string `json:"error_message"`
	CreatedAt   string `json:"created_at"`
}

func GetLatencyStats(ctx context.Context, db *sql.DB, days int, provider string) (*LatencyStats, error) {
	s := &LatencyStats{}
	query := `SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms),
	                 percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms),
	                 percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms)
	          FROM usage_tracking
	          WHERE created_at >= NOW() - ($1 || ' days')::interval AND is_error = false`
	args := []interface{}{days}
	if provider != "" {
		query += ` AND provider = $2`
		args = append(args, provider)
	}
	err := db.QueryRowContext(ctx, query, args...).Scan(&s.P50, &s.P95, &s.P99)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func GetTokenUsageByModel(ctx context.Context, db *sql.DB, days int) ([]*ModelTokenUsage, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT provider, COALESCE(model, 'unknown'),
		        SUM(input_tokens), SUM(output_tokens), SUM(COALESCE(cost_cents, 0)), COUNT(*)
		 FROM usage_tracking
		 WHERE created_at >= NOW() - ($1 || ' days')::interval
		 GROUP BY provider, model
		 ORDER BY SUM(input_tokens + output_tokens) DESC`,
		days,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*ModelTokenUsage
	for rows.Next() {
		m := &ModelTokenUsage{}
		rows.Scan(&m.Provider, &m.Model, &m.InputTokens, &m.OutputTokens, &m.CostCents, &m.Requests)
		result = append(result, m)
	}
	return result, nil
}

func GetErrorLogs(ctx context.Context, db *sql.DB, days, page, pageSize int) ([]*ErrorEntry, int, error) {
	var total int
	db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM usage_tracking WHERE is_error = true AND created_at >= NOW() - ($1 || ' days')::interval`,
		days,
	).Scan(&total)

	offset := (page - 1) * pageSize
	rows, err := db.QueryContext(ctx,
		`SELECT ut.id::text, u.username, ut.provider, COALESCE(ut.model, ''),
		        ut.status_code, COALESCE(ut.error_message, ''), ut.created_at
		 FROM usage_tracking ut
		 JOIN users u ON ut.user_id = u.id
		 WHERE ut.is_error = true AND ut.created_at >= NOW() - ($1 || ' days')::interval
		 ORDER BY ut.created_at DESC
		 LIMIT $2 OFFSET $3`,
		days, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*ErrorEntry
	for rows.Next() {
		e := &ErrorEntry{}
		rows.Scan(&e.ID, &e.Username, &e.Provider, &e.Model, &e.StatusCode, &e.ErrorMessage, &e.CreatedAt)
		e.CreatedAt = formatTime(e.CreatedAt)
		items = append(items, e)
	}
	return items, total, nil
}

func formatTime(s string) string {
	if t, err := time.Parse("2006-01-02T15:04:05Z07:00", s); err == nil {
		return t.Format(time.RFC3339)
	}
	return s
}

// Existing admin query types

type UsageSummary struct {
	TotalRequests int     `json:"total_requests"`
	TotalTokens   int     `json:"total_tokens"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	ActiveUsers   int     `json:"active_users"`
	ErrorCount    int     `json:"error_count"`
}

type DailyUsage struct {
	Date     string `json:"date"`
	Requests int    `json:"requests"`
	Tokens   int    `json:"tokens"`
}

type ProviderUsage struct {
	Provider string `json:"provider"`
	Requests int    `json:"requests"`
	Tokens   int    `json:"tokens"`
}

type UserUsageEntry struct {
	Username string `json:"username"`
	Requests int    `json:"requests"`
	Tokens   int    `json:"tokens"`
}

func GetUsageSummary(ctx context.Context, db *sql.DB, days int) (*UsageSummary, error) {
	s := &UsageSummary{}
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) as total_requests,
		        COALESCE(SUM(input_tokens + output_tokens), 0) as total_tokens,
		        COALESCE(AVG(duration_ms), 0) as avg_duration,
		        COUNT(DISTINCT user_id) as active_users,
		        COUNT(*) FILTER (WHERE is_error) as error_count
		 FROM usage_tracking
		 WHERE created_at >= NOW() - ($1 || ' days')::interval`,
		days,
	).Scan(&s.TotalRequests, &s.TotalTokens, &s.AvgDurationMs, &s.ActiveUsers, &s.ErrorCount)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func GetDailyUsage(ctx context.Context, db *sql.DB, days int) ([]*DailyUsage, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DATE(created_at)::text as day, COUNT(*) as requests, COALESCE(SUM(input_tokens + output_tokens), 0) as tokens
		 FROM usage_tracking
		 WHERE created_at >= NOW() - ($1 || ' days')::interval
		 GROUP BY DATE(created_at)
		 ORDER BY day`,
		days,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*DailyUsage
	for rows.Next() {
		d := &DailyUsage{}
		rows.Scan(&d.Date, &d.Requests, &d.Tokens)
		result = append(result, d)
	}
	return result, nil
}

func GetProviderUsage(ctx context.Context, db *sql.DB, days int) ([]*ProviderUsage, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT provider, COUNT(*) as requests, COALESCE(SUM(input_tokens + output_tokens), 0) as tokens
		 FROM usage_tracking
		 WHERE created_at >= NOW() - ($1 || ' days')::interval
		 GROUP BY provider
		 ORDER BY requests DESC`,
		days,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*ProviderUsage
	for rows.Next() {
		p := &ProviderUsage{}
		rows.Scan(&p.Provider, &p.Requests, &p.Tokens)
		result = append(result, p)
	}
	return result, nil
}

func GetUserUsage(ctx context.Context, db *sql.DB, days int) ([]*UserUsageEntry, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT u.username, COUNT(*) as requests, COALESCE(SUM(ut.input_tokens + ut.output_tokens), 0) as tokens
		 FROM usage_tracking ut
		 JOIN users u ON ut.user_id = u.id
		 WHERE ut.created_at >= NOW() - ($1 || ' days')::interval
		 GROUP BY u.username
		 ORDER BY requests DESC`,
		days,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*UserUsageEntry
	for rows.Next() {
		e := &UserUsageEntry{}
		rows.Scan(&e.Username, &e.Requests, &e.Tokens)
		result = append(result, e)
	}
	return result, nil
}

type UserUsageDetail struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Endpoint     string `json:"endpoint"`
	Tokens       int    `json:"tokens"`
	DurationMs   int    `json:"duration_ms"`
	StatusCode   int    `json:"status_code"`
	CostCents    int    `json:"cost_cents"`
	ErrorMessage string `json:"error_message,omitempty"`
	CreatedAt    string `json:"created_at"`
}

func GetUserUsageDetail(ctx context.Context, db *sql.DB, userID string, page, pageSize int) ([]*UserUsageDetail, int, error) {
	var total int
	db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM usage_tracking WHERE user_id = $1::uuid`,
		userID,
	).Scan(&total)

	offset := (page - 1) * pageSize
	rows, err := db.QueryContext(ctx,
		`SELECT id::text, provider, COALESCE(model, ''), COALESCE(endpoint, ''),
		        input_tokens + output_tokens, duration_ms,
		        COALESCE(status_code, 0), COALESCE(cost_cents, 0), COALESCE(error_message, ''), created_at
		 FROM usage_tracking
		 WHERE user_id = $1::uuid
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*UserUsageDetail
	for rows.Next() {
		d := &UserUsageDetail{}
		var createdAt time.Time
		rows.Scan(&d.ID, &d.Provider, &d.Model, &d.Endpoint, &d.Tokens, &d.DurationMs, &d.StatusCode, &d.CostCents, &d.ErrorMessage, &createdAt)
		d.CreatedAt = createdAt.Format(time.RFC3339)
		items = append(items, d)
	}
	return items, total, nil
}
