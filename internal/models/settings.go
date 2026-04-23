package models

import (
	"context"
	"database/sql"
	"encoding/json"
)

func GetSetting(ctx context.Context, db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx,
		`SELECT value::text FROM system_settings WHERE key = $1`, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func SetSetting(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO system_settings (key, value) VALUES ($1, $2::jsonb)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		key, value,
	)
	return err
}

func IsRegistrationEnabled(ctx context.Context, db *sql.DB) bool {
	val, err := GetSetting(ctx, db, "registration_enabled")
	if err != nil || val == "" {
		return true
	}
	var enabled bool
	if json.Unmarshal([]byte(val), &enabled) == nil {
		return enabled
	}
	return true
}

func IsInviteRequired(ctx context.Context, db *sql.DB) bool {
	val, err := GetSetting(ctx, db, "invite_required")
	if err != nil || val == "" {
		return false
	}
	var required bool
	if json.Unmarshal([]byte(val), &required) == nil {
		return required
	}
	return false
}

func ListSettings(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value::text FROM system_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		result[k] = v
	}
	return result, nil
}
