package retention

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

type Purger struct {
	db *sql.DB
}

func NewPurger(db *sql.DB) *Purger {
	return &Purger{db: db}
}

type RetentionPolicy struct {
	TableName    string `json:"table_name"`
	RetentionDays int   `json:"retention_days"`
	Enabled      bool   `json:"enabled"`
	LastPurgedAt string `json:"last_purged_at,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

func ListPolicies(ctx context.Context, db *sql.DB) ([]*RetentionPolicy, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT table_name, retention_days, enabled, COALESCE(last_purged_at::text, ''), updated_at::text
		 FROM retention_policies ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*RetentionPolicy
	for rows.Next() {
		p := &RetentionPolicy{}
		rows.Scan(&p.TableName, &p.RetentionDays, &p.Enabled, &p.LastPurgedAt, &p.UpdatedAt)
		result = append(result, p)
	}
	return result, nil
}

func UpdatePolicy(ctx context.Context, db *sql.DB, tableName string, retentionDays int, enabled bool) error {
	_, err := db.ExecContext(ctx,
		`UPDATE retention_policies SET retention_days = $1, enabled = $2, updated_at = NOW()
		 WHERE table_name = $3`,
		retentionDays, enabled, tableName,
	)
	return err
}

func (p *Purger) RunOnce(ctx context.Context) {
	policies, err := ListPolicies(ctx, p.db)
	if err != nil {
		slog.Error("purge: failed to load policies", "error", err)
		return
	}

	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		p.purgeTable(ctx, policy.TableName, policy.RetentionDays)
	}
}

func (p *Purger) purgeTable(ctx context.Context, tableName string, retentionDays int) {
	batchSize := 10000
	total := 0

	for {
		result, err := p.db.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE id IN (
				SELECT id FROM %s
				WHERE created_at < NOW() - ($1 || ' days')::interval
				ORDER BY created_at
				LIMIT %d
			)`, tableName, tableName, batchSize),
			retentionDays,
		)
		if err != nil {
			slog.Error("purge: delete failed", "table", tableName, "error", err)
			break
		}

		n, _ := result.RowsAffected()
		total += int(n)
		if n == 0 {
			break
		}
	}

	if total > 0 {
		slog.Info("purge: cleaned rows", "table", tableName, "count", total)
	}

	p.db.ExecContext(ctx,
		`UPDATE retention_policies SET last_purged_at = NOW() WHERE table_name = $1`,
		tableName,
	)
}

func (p *Purger) Start(interval time.Duration) {
	go func() {
		// Run once at startup after a delay
		time.Sleep(1 * time.Minute)
		p.RunOnce(context.Background())

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			p.RunOnce(context.Background())
		}
	}()
}
