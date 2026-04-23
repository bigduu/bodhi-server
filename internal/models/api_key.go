package models

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/bigduu/bodhi-server/internal/auth"
)

// stringArray wraps a *[]string for database/sql scanning of PostgreSQL text[] columns.
type stringArray struct {
	s *[]string
}

func (a stringArray) Scan(src interface{}) error {
	if src == nil {
		*a.s = nil
		return nil
	}
	switch v := src.(type) {
	case string:
		*a.s = parsePostgresArray(v)
		return nil
	case []byte:
		*a.s = parsePostgresArray(string(v))
		return nil
	}
	return fmt.Errorf("cannot scan %T into stringArray", src)
}

func parsePostgresArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "{}" || s == "" {
		return nil
	}
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, strings.Trim(p, `"`))
		}
	}
	return result
}

func toPostgresArray(vals []string) string {
	if len(vals) == 0 {
		return "{}"
	}
	escaped := make([]string, len(vals))
	for i, v := range vals {
		if strings.ContainsAny(v, `,"\ {}`) {
			escaped[i] = `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
		} else {
			escaped[i] = v
		}
	}
	return "{" + strings.Join(escaped, ",") + "}"
}

type APIKey struct {
	ID               string
	UserID           string
	Name             string
	KeyPrefix        string
	KeySuffix        string
	IsActive         bool
	ExpiresAt        sql.NullTime
	LastUsedAt       sql.NullTime
	CreatedAt        time.Time
	RotatedFrom      sql.NullString
	AllowedModels    []string
	AllowedProviders []string
	IPWhitelist      []string
}

func CreateAPIKey(ctx context.Context, db *sql.DB, userID, name string, expiresAt *time.Time, allowedModels, allowedProviders, ipWhitelist []string) (key *APIKey, fullKey string, err error) {
	rawKey, keyHash, keyPrefix, keySuffix, genErr := auth.GenerateAPIKey()
	if genErr != nil {
		return nil, "", genErr
	}
	fullKey = rawKey

	k := &APIKey{
		UserID:           userID,
		Name:             name,
		KeyPrefix:        keyPrefix,
		KeySuffix:        keySuffix,
		IsActive:         true,
		AllowedModels:    allowedModels,
		AllowedProviders: allowedProviders,
		IPWhitelist:      ipWhitelist,
	}

	var expiresAtVal interface{}
	if expiresAt != nil {
		k.ExpiresAt = sql.NullTime{Time: *expiresAt, Valid: true}
		expiresAtVal = *expiresAt
	}

	err = db.QueryRowContext(ctx,
		`INSERT INTO api_keys (user_id, name, key_prefix, key_hash, key_suffix, expires_at, allowed_models, allowed_providers, ip_whitelist)
		 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id::text, created_at`,
		userID, name, keyPrefix, keyHash, keySuffix, expiresAtVal,
		toPostgresArray(allowedModels), toPostgresArray(allowedProviders), toPostgresArray(ipWhitelist),
	).Scan(&k.ID, &k.CreatedAt)

	if err != nil {
		return nil, "", err
	}

	return k, fullKey, nil
}

func ListAPIKeys(ctx context.Context, db *sql.DB, userID string) ([]*APIKey, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id::text, user_id::text, name, key_prefix, key_suffix, is_active, expires_at, last_used_at, created_at, rotated_from::text,
		        COALESCE(allowed_models, '{}'), COALESCE(allowed_providers, '{}'), COALESCE(ip_whitelist, '{}')
		 FROM api_keys WHERE user_id = $1::uuid
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		k := &APIKey{}
		err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.KeySuffix, &k.IsActive, &k.ExpiresAt, &k.LastUsedAt, &k.CreatedAt, &k.RotatedFrom,
			stringArray{&k.AllowedModels}, stringArray{&k.AllowedProviders}, stringArray{&k.IPWhitelist})
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func DeleteAPIKey(ctx context.Context, db *sql.DB, userID, keyID string) error {
	res, err := db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE id = $1::uuid AND user_id = $2::uuid`,
		keyID, userID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func RotateAPIKey(ctx context.Context, db *sql.DB, userID, oldKeyID, name string, expiresAt *time.Time) (key *APIKey, fullKey string, err error) {
	rawKey, keyHash, keyPrefix, keySuffix, genErr := auth.GenerateAPIKey()
	if genErr != nil {
		return nil, "", genErr
	}
	fullKey = rawKey

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE api_keys SET is_active = false WHERE id = $1::uuid AND user_id = $2::uuid`,
		oldKeyID, userID,
	)
	if err != nil {
		return nil, "", err
	}

	k := &APIKey{
		UserID:     userID,
		Name:       name,
		KeyPrefix:  keyPrefix,
		KeySuffix:  keySuffix,
		IsActive:   true,
		RotatedFrom: sql.NullString{String: oldKeyID, Valid: true},
	}

	var expiresAtVal interface{}
	if expiresAt != nil {
		k.ExpiresAt = sql.NullTime{Time: *expiresAt, Valid: true}
		expiresAtVal = *expiresAt
	}

	err = tx.QueryRowContext(ctx,
		`INSERT INTO api_keys (user_id, name, key_prefix, key_hash, key_suffix, expires_at, rotated_from)
		 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::uuid)
		 RETURNING id::text, created_at`,
		userID, name, keyPrefix, keyHash, keySuffix, expiresAtVal, oldKeyID,
	).Scan(&k.ID, &k.CreatedAt)

	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	return k, fullKey, nil
}
