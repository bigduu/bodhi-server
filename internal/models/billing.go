package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type BillingReport struct {
	UserID            string `json:"user_id"`
	PeriodStart       string `json:"period_start"`
	PeriodEnd         string `json:"period_end"`
	TotalRequests     int    `json:"total_requests"`
	TotalInputTokens  int64  `json:"total_input_tokens"`
	TotalOutputTokens int64  `json:"total_output_tokens"`
	TotalCostCents    int    `json:"total_cost_cents"`
}

type BillingReportByModel struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	Requests        int    `json:"requests"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	CostCents       int    `json:"cost_cents"`
}

func GetCurrentUsage(ctx context.Context, db *sql.DB, userID string) (*BillingReport, error) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	r := &BillingReport{
		UserID:      userID,
		PeriodStart: monthStart.Format(time.RFC3339),
		PeriodEnd:   now.Format(time.RFC3339),
	}

	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cost_cents), 0)
		 FROM usage_tracking
		 WHERE user_id = $1::uuid AND created_at >= $2`,
		userID, monthStart,
	).Scan(&r.TotalRequests, &r.TotalInputTokens, &r.TotalOutputTokens, &r.TotalCostCents)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func GetCurrentUsageByModel(ctx context.Context, db *sql.DB, userID string) ([]*BillingReportByModel, error) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	rows, err := db.QueryContext(ctx,
		`SELECT provider, COALESCE(model, 'unknown'),
		        COUNT(*), SUM(input_tokens), SUM(output_tokens), COALESCE(SUM(cost_cents), 0)
		 FROM usage_tracking
		 WHERE user_id = $1::uuid AND created_at >= $2
		 GROUP BY provider, model
		 ORDER BY SUM(input_tokens + output_tokens) DESC`,
		userID, monthStart,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*BillingReportByModel
	for rows.Next() {
		m := &BillingReportByModel{}
		rows.Scan(&m.Provider, &m.Model, &m.Requests, &m.InputTokens, &m.OutputTokens, &m.CostCents)
		result = append(result, m)
	}
	return result, nil
}

func GenerateMonthlyReport(ctx context.Context, db *sql.DB, userID string, year int, month time.Month) (*BillingReport, error) {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	r := &BillingReport{
		UserID:      userID,
		PeriodStart: start.Format(time.RFC3339),
		PeriodEnd:   end.Format(time.RFC3339),
	}

	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cost_cents), 0)
		 FROM usage_tracking
		 WHERE user_id = $1::uuid AND created_at >= $2 AND created_at < $3`,
		userID, start, end,
	).Scan(&r.TotalRequests, &r.TotalInputTokens, &r.TotalOutputTokens, &r.TotalCostCents)
	if err != nil {
		return nil, err
	}

	// Upsert billing period
	db.ExecContext(ctx,
		`INSERT INTO billing_periods (user_id, period_start, period_end, total_requests, total_input_tokens, total_output_tokens, total_cost_cents)
		 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (user_id, period_start) DO UPDATE SET
		    total_requests = EXCLUDED.total_requests,
		    total_input_tokens = EXCLUDED.total_input_tokens,
		    total_output_tokens = EXCLUDED.total_output_tokens,
		    total_cost_cents = EXCLUDED.total_cost_cents,
		    generated_at = NOW()`,
		userID, start, end, r.TotalRequests, r.TotalInputTokens, r.TotalOutputTokens, r.TotalCostCents,
	)

	return r, nil
}

func ListBillingReports(ctx context.Context, db *sql.DB, userID string) ([]*BillingReport, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT user_id::text, period_start::text, period_end::text, total_requests, total_input_tokens, total_output_tokens, total_cost_cents
		 FROM billing_periods WHERE user_id = $1::uuid ORDER BY period_start DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*BillingReport
	for rows.Next() {
		r := &BillingReport{}
		rows.Scan(&r.UserID, &r.PeriodStart, &r.PeriodEnd, &r.TotalRequests, &r.TotalInputTokens, &r.TotalOutputTokens, &r.TotalCostCents)
		result = append(result, r)
	}
	return result, nil
}

func ExportCSVRaw(ctx context.Context, db *sql.DB, userID string, year int, month time.Month) ([]byte, error) {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	rows, err := db.QueryContext(ctx,
		`SELECT created_at, provider, COALESCE(model, ''), input_tokens, output_tokens, cost_cents, status_code, duration_ms
		 FROM usage_tracking
		 WHERE user_id = $1::uuid AND created_at >= $2 AND created_at < $3
		 ORDER BY created_at`,
		userID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	csv := "date,provider,model,input_tokens,output_tokens,cost_cents,status_code,duration_ms\n"
	for rows.Next() {
		var createdAt time.Time
		var provider, model string
		var inputTokens, outputTokens, costCents, statusCode, durationMs int
		rows.Scan(&createdAt, &provider, &model, &inputTokens, &outputTokens, &costCents, &statusCode, &durationMs)
		csv += fmt.Sprintf("%s,%s,%s,%d,%d,%d,%d,%d\n",
			createdAt.Format("2006-01-02 15:04:05"), provider, model,
			inputTokens, outputTokens, costCents, statusCode, durationMs)
	}
	return []byte(csv), nil
}

func AddBalance(ctx context.Context, db *sql.DB, userID string, cents int) error {
	_, err := db.ExecContext(ctx,
		`UPDATE user_quotas SET balance_cents = COALESCE(balance_cents, 0) + $2, updated_at = NOW() WHERE user_id = $1::uuid`,
		userID, cents,
	)
	return err
}

func GetBalance(ctx context.Context, db *sql.DB, userID string) (int, error) {
	var balance int
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(balance_cents, 0) FROM user_quotas WHERE user_id = $1::uuid`,
		userID,
	).Scan(&balance)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return balance, err
}

func DeductBalance(ctx context.Context, db *sql.DB, userID string, cents int) {
	if cents <= 0 {
		return
	}
	db.ExecContext(ctx,
		`UPDATE user_quotas SET balance_cents = GREATEST(COALESCE(balance_cents, 0) - $2, 0), updated_at = NOW() WHERE user_id = $1::uuid`,
		userID, cents,
	)
}

type BalanceAlertData struct {
	UserID      string `json:"user_id"`
	BalanceCents int   `json:"balance_cents"`
	Threshold   int    `json:"threshold_cents"`
}

func CheckBalanceAlert(ctx context.Context, db *sql.DB, userID string, dispatcher interface {
	Dispatch(event string, data interface{})
}) {
	var balance, threshold int
	var lastAlerted sql.NullTime
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(balance_cents, 0), COALESCE(balance_alert_threshold, 0), balance_last_alerted_at
		 FROM user_quotas WHERE user_id = $1::uuid`,
		userID,
	).Scan(&balance, &threshold, &lastAlerted)
	if err != nil || threshold <= 0 {
		return
	}

	if balance < threshold {
		if lastAlerted.Valid && time.Since(lastAlerted.Time) < 24*time.Hour {
			return
		}
		db.ExecContext(ctx,
			`UPDATE user_quotas SET balance_last_alerted_at = NOW() WHERE user_id = $1::uuid`,
			userID,
		)
		dispatcher.Dispatch("balance_low", &BalanceAlertData{
			UserID:       userID,
			BalanceCents: balance,
			Threshold:    threshold,
		})
	}
}
