package models

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type ProviderCredential struct {
	ID              string
	UserID          string
	Provider        string
	EncryptedAPIKey string
	BaseURL         sql.NullString
	IsActive        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

var ErrCredentialNotFound = errors.New("credential not found")

func CreateCredential(ctx context.Context, db *sql.DB, userID, provider, encryptedKey, baseURL string) (*ProviderCredential, error) {
	c := &ProviderCredential{
		UserID:          userID,
		Provider:        provider,
		EncryptedAPIKey: encryptedKey,
		IsActive:        true,
	}

	var baseURLVal interface{}
	if baseURL != "" {
		c.BaseURL = sql.NullString{String: baseURL, Valid: true}
		baseURLVal = baseURL
	}

	err := db.QueryRowContext(ctx,
		`INSERT INTO provider_credentials (user_id, provider, encrypted_api_key, base_url)
		 VALUES ($1::uuid, $2, $3, $4)
		 ON CONFLICT (user_id, provider) DO UPDATE
		    SET encrypted_api_key = EXCLUDED.encrypted_api_key,
		        base_url = EXCLUDED.base_url,
		        updated_at = NOW(),
		        is_active = true
		 RETURNING id::text, created_at, updated_at`,
		userID, provider, encryptedKey, baseURLVal,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return c, nil
}

func GetCredential(ctx context.Context, db *sql.DB, userID, provider string) (*ProviderCredential, error) {
	c := &ProviderCredential{}
	err := db.QueryRowContext(ctx,
		`SELECT id::text, user_id::text, provider, encrypted_api_key, base_url, is_active, created_at, updated_at
		 FROM provider_credentials
		 WHERE user_id = $1::uuid AND provider = $2 AND is_active = true`,
		userID, provider,
	).Scan(&c.ID, &c.UserID, &c.Provider, &c.EncryptedAPIKey, &c.BaseURL, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, err
	}
	return c, nil
}

func ListCredentials(ctx context.Context, db *sql.DB, userID string) ([]*ProviderCredential, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT provider, base_url, is_active, created_at, updated_at
		 FROM provider_credentials
		 WHERE user_id = $1::uuid
		 ORDER BY provider`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []*ProviderCredential
	for rows.Next() {
		c := &ProviderCredential{}
		err := rows.Scan(&c.Provider, &c.BaseURL, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, nil
}

func DeleteCredential(ctx context.Context, db *sql.DB, userID, provider string) error {
	res, err := db.ExecContext(ctx,
		`DELETE FROM provider_credentials WHERE user_id = $1::uuid AND provider = $2`,
		userID, provider,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCredentialNotFound
	}
	return nil
}
