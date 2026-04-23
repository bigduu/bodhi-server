package moderation

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sync"
	"time"
)

type Rule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Pattern  string `json:"pattern"`
	Action   string `json:"action"` // block, warn, redact
	Scope    string `json:"scope"`  // input, output, both
	IsActive bool   `json:"is_active"`
}

type Filter struct {
	db    *sql.DB
	mu    sync.RWMutex
	rules []*Rule
	re    map[string]*regexp.Regexp
}

func NewFilter(db *sql.DB) *Filter {
	f := &Filter{
		db: db,
		re: make(map[string]*regexp.Regexp),
	}
	f.Reload(context.Background())
	return f
}

func (f *Filter) Reload(ctx context.Context) error {
	rows, err := f.db.QueryContext(ctx,
		`SELECT id::text, name, pattern, action, scope, is_active FROM content_rules WHERE is_active = true`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var rules []*Rule
	for rows.Next() {
		r := &Rule{}
		rows.Scan(&r.ID, &r.Name, &r.Pattern, &r.Action, &r.Scope, &r.IsActive)
		rules = append(rules, r)
	}

	f.mu.Lock()
	f.rules = rules
	f.re = make(map[string]*regexp.Regexp)
	for _, r := range rules {
		if re, err := regexp.Compile(r.Pattern); err == nil {
			f.re[r.ID] = re
		}
	}
	f.mu.Unlock()
	return nil
}

type FilterResult struct {
	Matched bool
	Action  string
	Rule    string
}

func (f *Filter) CheckInput(ctx context.Context, text string) *FilterResult {
	return f.check(text, "input")
}

func (f *Filter) CheckOutput(ctx context.Context, text string) *FilterResult {
	return f.check(text, "output")
}

func (f *Filter) check(text, scope string) *FilterResult {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, r := range f.rules {
		if r.Scope != scope && r.Scope != "both" {
			continue
		}
		re, ok := f.re[r.ID]
		if !ok {
			continue
		}
		if re.MatchString(text) {
			return &FilterResult{Matched: true, Action: r.Action, Rule: r.Name}
		}
	}
	return nil
}

func (f *Filter) Redact(text string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := text
	for _, r := range f.rules {
		if r.Action != "redact" {
			continue
		}
		re, ok := f.re[r.ID]
		if !ok {
			continue
		}
		result = re.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}

func CreateRule(ctx context.Context, db *sql.DB, name, pattern, action, scope string) (*Rule, error) {
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}
	r := &Rule{}
	err := db.QueryRowContext(ctx,
		`INSERT INTO content_rules (name, pattern, action, scope) VALUES ($1, $2, $3, $4)
		 RETURNING id::text, name, pattern, action, scope, is_active`,
		name, pattern, action, scope,
	).Scan(&r.ID, &r.Name, &r.Pattern, &r.Action, &r.Scope, &r.IsActive)
	return r, err
}

func ListRules(ctx context.Context, db *sql.DB) ([]*Rule, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id::text, name, pattern, action, scope, is_active FROM content_rules ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Rule
	for rows.Next() {
		r := &Rule{}
		rows.Scan(&r.ID, &r.Name, &r.Pattern, &r.Action, &r.Scope, &r.IsActive)
		result = append(result, r)
	}
	return result, nil
}

func DeleteRule(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM content_rules WHERE id = $1::uuid`, id)
	return err
}

func ToggleRule(ctx context.Context, db *sql.DB, id string, active bool) error {
	_, err := db.ExecContext(ctx, `UPDATE content_rules SET is_active = $1 WHERE id = $2::uuid`, active, id)
	return err
}

var startReload time.Time
