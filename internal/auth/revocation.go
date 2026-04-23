package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// IsTokenRevoked checks if a refresh token's JTI has been revoked.
func IsTokenRevoked(ctx context.Context, db *sql.DB, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	hash := hashJTI(jti)
	var exists bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM token_revocations WHERE jti_hash = $1)`,
		hash,
	).Scan(&exists)
	return exists, err
}

// RevokeToken adds a single token JTI to the revocation list.
func RevokeToken(ctx context.Context, db *sql.DB, jti string, expiresAt time.Time) error {
	if jti == "" {
		return nil
	}
	hash := hashJTI(jti)
	_, err := db.ExecContext(ctx,
		`INSERT INTO token_revocations (jti_hash, expires_at) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		hash, expiresAt,
	)
	return err
}

// RevokeAllUserRefreshTokens revokes all refresh tokens for a user by recording
// a cutoff timestamp. All tokens issued before this time are considered revoked.
// Since we can't enumerate all JTIs, we use a user-level revocation marker.
func RevokeAllUserRefreshTokens(ctx context.Context, db *sql.DB, userID string) error {
	// We use a special marker entry with a well-known prefix + userID
	hash := hashJTI("user-revoke:" + userID)
	_, err := db.ExecContext(ctx,
		`INSERT INTO token_revocations (jti_hash, expires_at) VALUES ($1, $2)
		 ON CONFLICT (jti_hash) DO UPDATE SET expires_at = $2, created_at = NOW()`,
		hash, time.Now().Add(7*24*time.Hour),
	)
	return err
}

// IsUserTokensRevoked checks if all tokens for a user were revoked after the given time.
func IsUserTokensRevoked(ctx context.Context, db *sql.DB, userID string, issuedAt time.Time) (bool, error) {
	hash := hashJTI("user-revoke:" + userID)
	var revokedAt time.Time
	err := db.QueryRowContext(ctx,
		`SELECT created_at FROM token_revocations WHERE jti_hash = $1`,
		hash,
	).Scan(&revokedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return issuedAt.Before(revokedAt), nil
}

// CleanupRevokedTokens removes expired revocation entries.
func CleanupRevokedTokens(ctx context.Context, db *sql.DB) (int64, error) {
	result, err := db.ExecContext(ctx,
		`DELETE FROM token_revocations WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("cleanup revoked tokens: %w", err)
	}
	return result.RowsAffected()
}

func hashJTI(jti string) string {
	h := sha256.Sum256([]byte(jti))
	return hex.EncodeToString(h[:])
}
