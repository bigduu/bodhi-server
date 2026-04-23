package models

import (
	"context"
	"database/sql"
	"time"
)

type Model struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DisplayName   string    `json:"display_name"`
	Provider      string    `json:"provider"`
	IsActive      bool      `json:"is_active"`
	IsFeatured    bool      `json:"is_featured"`
	ContextWindow int       `json:"context_window"`
	MaxOutput     int       `json:"max_output"`
	Capabilities  []string  `json:"capabilities"`
	SortOrder     int       `json:"sort_order"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func ListModels(ctx context.Context, db *sql.DB) ([]*Model, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id::text, name, COALESCE(display_name, ''), provider, is_active, is_featured,
		        COALESCE(context_window, 0), COALESCE(max_output, 0), capabilities, sort_order, created_at, updated_at
		 FROM models ORDER BY sort_order, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Model
	for rows.Next() {
		m := &Model{}
		rows.Scan(&m.ID, &m.Name, &m.DisplayName, &m.Provider, &m.IsActive, &m.IsFeatured,
			&m.ContextWindow, &m.MaxOutput, &m.Capabilities, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt)
		result = append(result, m)
	}
	return result, nil
}

func ListActiveModels(ctx context.Context, db *sql.DB) ([]*Model, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id::text, name, COALESCE(display_name, ''), provider, is_active, is_featured,
		        COALESCE(context_window, 0), COALESCE(max_output, 0), capabilities, sort_order, created_at, updated_at
		 FROM models WHERE is_active = true ORDER BY sort_order, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Model
	for rows.Next() {
		m := &Model{}
		rows.Scan(&m.ID, &m.Name, &m.DisplayName, &m.Provider, &m.IsActive, &m.IsFeatured,
			&m.ContextWindow, &m.MaxOutput, &m.Capabilities, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt)
		result = append(result, m)
	}
	return result, nil
}

func GetModelByName(ctx context.Context, db *sql.DB, name string) (*Model, error) {
	m := &Model{}
	err := db.QueryRowContext(ctx,
		`SELECT id::text, name, COALESCE(display_name, ''), provider, is_active, is_featured,
		        COALESCE(context_window, 0), COALESCE(max_output, 0), capabilities, sort_order, created_at, updated_at
		 FROM models WHERE name = $1`, name,
	).Scan(&m.ID, &m.Name, &m.DisplayName, &m.Provider, &m.IsActive, &m.IsFeatured,
		&m.ContextWindow, &m.MaxOutput, &m.Capabilities, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func CreateModel(ctx context.Context, db *sql.DB, m *Model) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO models (name, display_name, provider, is_active, is_featured, context_window, max_output, capabilities, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		m.Name, m.DisplayName, m.Provider, m.IsActive, m.IsFeatured,
		m.ContextWindow, m.MaxOutput, m.Capabilities, m.SortOrder,
	)
	return err
}

func UpdateModel(ctx context.Context, db *sql.DB, id string, m *Model) error {
	_, err := db.ExecContext(ctx,
		`UPDATE models SET name=$1, display_name=$2, provider=$3, is_active=$4, is_featured=$5,
		        context_window=$6, max_output=$7, capabilities=$8, sort_order=$9, updated_at=NOW()
		 WHERE id = $10::uuid`,
		m.Name, m.DisplayName, m.Provider, m.IsActive, m.IsFeatured,
		m.ContextWindow, m.MaxOutput, m.Capabilities, m.SortOrder, id,
	)
	return err
}

func DeleteModel(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM models WHERE id = $1::uuid`, id)
	return err
}
