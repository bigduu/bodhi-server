package models

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrInviteCodeInvalid = errors.New("invalid or expired invite code")

type InviteCode struct {
	Code      string     `json:"code"`
	CreatedBy string     `json:"created_by"`
	MaxUses   int        `json:"max_uses"`
	UsedCount int        `json:"used_count"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func ValidateInviteCode(ctx context.Context, db *sql.DB, code string) error {
	var usedCount, maxUses int
	var expiresAt *time.Time
	err := db.QueryRowContext(ctx,
		`SELECT used_count, max_uses, expires_at FROM invite_codes WHERE code = $1`,
		code,
	).Scan(&usedCount, &maxUses, &expiresAt)
	if err == sql.ErrNoRows {
		return ErrInviteCodeInvalid
	}
	if err != nil {
		return err
	}
	if maxUses > 0 && usedCount >= maxUses {
		return ErrInviteCodeInvalid
	}
	if expiresAt != nil && time.Now().After(*expiresAt) {
		return ErrInviteCodeInvalid
	}
	return nil
}

func UseInviteCode(ctx context.Context, db *sql.DB, code string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE invite_codes SET used_count = used_count + 1 WHERE code = $1`,
		code,
	)
	return err
}

func CreateInviteCode(ctx context.Context, db *sql.DB, createdBy string, maxUses int, expiresAt *time.Time) (string, error) {
	var code string
	err := db.QueryRowContext(ctx,
		`INSERT INTO invite_codes (created_by, max_uses, expires_at) VALUES ($1::uuid, $2, $3) RETURNING code`,
		createdBy, maxUses, expiresAt,
	).Scan(&code)
	return code, err
}

func ListInviteCodes(ctx context.Context, db *sql.DB) ([]*InviteCode, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT code, COALESCE(created_by::text, ''), max_uses, used_count, expires_at, created_at
		 FROM invite_codes ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*InviteCode
	for rows.Next() {
		ic := &InviteCode{}
		rows.Scan(&ic.Code, &ic.CreatedBy, &ic.MaxUses, &ic.UsedCount, &ic.ExpiresAt, &ic.CreatedAt)
		result = append(result, ic)
	}
	return result, nil
}

func DeleteInviteCode(ctx context.Context, db *sql.DB, code string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM invite_codes WHERE code = $1`, code)
	return err
}
