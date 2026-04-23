package models

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           string
	Username     string
	Email        sql.NullString
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  sql.NullTime
	IsActive     bool
	IsAdmin      bool
}

var ErrUserExists = errors.New("username already exists")
var ErrUserNotFound = errors.New("user not found")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrForbidden = errors.New("forbidden")

func CreateUser(ctx context.Context, db *sql.DB, username, password, email string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &User{
		Username:     username,
		PasswordHash: string(hash),
		IsActive:     true,
	}

	if email != "" {
		u.Email = sql.NullString{String: email, Valid: true}
	}

	// First user becomes admin
	var userCount int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount)
	u.IsAdmin = userCount == 0

	err = db.QueryRowContext(ctx,
		`INSERT INTO users (username, email, password_hash, is_admin)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id::text, created_at, updated_at`,
		u.Username, u.Email, u.PasswordHash, u.IsAdmin,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if isDuplicateKey(err) {
			return nil, ErrUserExists
		}
		return nil, err
	}

	return u, nil
}

func AuthenticateUser(ctx context.Context, db *sql.DB, username, password string) (*User, error) {
	u := &User{}
	err := db.QueryRowContext(ctx,
		`SELECT id::text, username, email, password_hash, created_at, updated_at, last_login_at, is_active, is_admin
		 FROM users WHERE username = $1`,
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt, &u.IsActive, &u.IsAdmin)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if !u.IsActive {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	db.ExecContext(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1`, u.ID)
	return u, nil
}

func GetUserByID(ctx context.Context, db *sql.DB, id string) (*User, error) {
	u := &User{}
	err := db.QueryRowContext(ctx,
		`SELECT id::text, username, email, created_at, updated_at, last_login_at, is_active, is_admin
		 FROM users WHERE id = $1::uuid`,
		id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt, &u.IsActive, &u.IsAdmin)

	if err != nil {
		return nil, err
	}
	return u, nil
}

type UserListItem struct {
	ID          string
	Username    string
	Email       string
	IsActive    bool
	IsAdmin     bool
	APIKeyCount int
	CredCount   int
	LastLoginAt *time.Time
	CreatedAt   time.Time
}

func ListUsers(ctx context.Context, db *sql.DB, page, pageSize int) ([]*UserListItem, int, error) {
	var total int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)

	offset := (page - 1) * pageSize
	rows, err := db.QueryContext(ctx,
		`SELECT u.id::text, u.username, COALESCE(u.email, ''), u.is_active, u.is_admin,
		        u.last_login_at, u.created_at,
		        (SELECT COUNT(*) FROM api_keys ak WHERE ak.user_id = u.id AND ak.is_active = true) as api_key_count,
		        (SELECT COUNT(*) FROM provider_credentials pc WHERE pc.user_id = u.id AND pc.is_active = true) as cred_count
		 FROM users u
		 ORDER BY u.created_at DESC
		 LIMIT $1 OFFSET $2`,
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*UserListItem
	for rows.Next() {
		item := &UserListItem{}
		var lastLogin sql.NullTime
		err := rows.Scan(&item.ID, &item.Username, &item.Email, &item.IsActive, &item.IsAdmin,
			&lastLogin, &item.CreatedAt, &item.APIKeyCount, &item.CredCount)
		if err != nil {
			return nil, 0, err
		}
		if lastLogin.Valid {
			item.LastLoginAt = &lastLogin.Time
		}
		items = append(items, item)
	}
	return items, total, nil
}

func UpdateUserStatus(ctx context.Context, db *sql.DB, userID string, isActive, isAdmin *bool) error {
	if isActive != nil && isAdmin != nil {
		_, err := db.ExecContext(ctx,
			`UPDATE users SET is_active = $1, is_admin = $2, updated_at = NOW() WHERE id = $3::uuid`,
			*isActive, *isAdmin, userID,
		)
		return err
	}
	if isActive != nil {
		_, err := db.ExecContext(ctx,
			`UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2::uuid`,
			*isActive, userID,
		)
		return err
	}
	if isAdmin != nil {
		_, err := db.ExecContext(ctx,
			`UPDATE users SET is_admin = $1, updated_at = NOW() WHERE id = $2::uuid`,
			*isAdmin, userID,
		)
		return err
	}
	return nil
}

func DeleteUser(ctx context.Context, db *sql.DB, userID string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = $1::uuid`, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func isDuplicateKey(err error) bool {
	return err != nil && (contains(err.Error(), "duplicate key") || contains(err.Error(), "violates unique constraint"))
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
