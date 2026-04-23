package models

import (
	"context"
	"database/sql"
	"time"
)

type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

type GroupMember struct {
	GroupID  string    `json:"group_id"`
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type GroupCredential struct {
	ID              string         `json:"id"`
	GroupID         string         `json:"group_id"`
	Provider        string         `json:"provider"`
	EncryptedAPIKey string         `json:"-"`
	BaseURL         sql.NullString `json:"base_url"`
	IsActive        bool           `json:"is_active"`
	CreatedAt       time.Time      `json:"created_at"`
}

type GroupQuota struct {
	GroupID      string   `json:"group_id"`
	RPMLimit     int      `json:"rpm_limit"`
	RPDLimit     int      `json:"rpd_limit"`
	TokenDaily   int64    `json:"token_daily"`
	TokenMonthly int64    `json:"token_monthly"`
	SpendDaily   int      `json:"spend_daily"`
	SpendMonthly int      `json:"spend_monthly"`
	AllowedModels []string `json:"allowed_models"`
}

func CreateGroup(ctx context.Context, db *sql.DB, name, description, createdBy string) (*Group, error) {
	g := &Group{}
	err := db.QueryRowContext(ctx,
		`INSERT INTO groups (name, description, created_by) VALUES ($1, $2, $3::uuid)
		 RETURNING id::text, name, COALESCE(description, ''), created_by::text, created_at`,
		name, description, createdBy,
	).Scan(&g.ID, &g.Name, &g.Description, &g.CreatedBy, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func ListGroups(ctx context.Context, db *sql.DB) ([]*Group, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id::text, name, COALESCE(description, ''), COALESCE(created_by::text, ''), created_at
		 FROM groups ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Group
	for rows.Next() {
		g := &Group{}
		rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedBy, &g.CreatedAt)
		result = append(result, g)
	}
	return result, nil
}

func GetGroup(ctx context.Context, db *sql.DB, id string) (*Group, error) {
	g := &Group{}
	err := db.QueryRowContext(ctx,
		`SELECT id::text, name, COALESCE(description, ''), COALESCE(created_by::text, ''), created_at
		 FROM groups WHERE id = $1::uuid`, id,
	).Scan(&g.ID, &g.Name, &g.Description, &g.CreatedBy, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func UpdateGroup(ctx context.Context, db *sql.DB, id, name, description string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE groups SET name = $1, description = $2 WHERE id = $3::uuid`,
		name, description, id)
	return err
}

func DeleteGroup(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM groups WHERE id = $1::uuid`, id)
	return err
}

func AddGroupMember(ctx context.Context, db *sql.DB, groupID, userID, role string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO group_members (group_id, user_id, role) VALUES ($1::uuid, $2::uuid, $3)
		 ON CONFLICT (group_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		groupID, userID, role)
	return err
}

func RemoveGroupMember(ctx context.Context, db *sql.DB, groupID, userID string) error {
	_, err := db.ExecContext(ctx,
		`DELETE FROM group_members WHERE group_id = $1::uuid AND user_id = $2::uuid`,
		groupID, userID)
	return err
}

func ListGroupMembers(ctx context.Context, db *sql.DB, groupID string) ([]*GroupMember, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT gm.group_id::text, gm.user_id::text, u.username, gm.role, gm.joined_at
		 FROM group_members gm
		 JOIN users u ON gm.user_id = u.id
		 WHERE gm.group_id = $1::uuid
		 ORDER BY gm.joined_at`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*GroupMember
	for rows.Next() {
		m := &GroupMember{}
		rows.Scan(&m.GroupID, &m.UserID, &m.Username, &m.Role, &m.JoinedAt)
		result = append(result, m)
	}
	return result, nil
}

func GetUserGroups(ctx context.Context, db *sql.DB, userID string) ([]*Group, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT g.id::text, g.name, COALESCE(g.description, ''), COALESCE(g.created_by::text, ''), g.created_at
		 FROM groups g
		 JOIN group_members gm ON g.id = gm.group_id
		 WHERE gm.user_id = $1::uuid
		 ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Group
	for rows.Next() {
		g := &Group{}
		rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedBy, &g.CreatedAt)
		result = append(result, g)
	}
	return result, nil
}

func GetGroupCredential(ctx context.Context, db *sql.DB, groupID, provider string) (*GroupCredential, error) {
	c := &GroupCredential{}
	err := db.QueryRowContext(ctx,
		`SELECT id::text, group_id::text, provider, encrypted_api_key, base_url, is_active, created_at
		 FROM group_credentials
		 WHERE group_id = $1::uuid AND provider = $2 AND is_active = true`,
		groupID, provider,
	).Scan(&c.ID, &c.GroupID, &c.Provider, &c.EncryptedAPIKey, &c.BaseURL, &c.IsActive, &c.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrCredentialNotFound
		}
		return nil, err
	}
	return c, nil
}

func SetGroupCredential(ctx context.Context, db *sql.DB, groupID, provider, encryptedKey string, baseURL sql.NullString) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO group_credentials (group_id, provider, encrypted_api_key, base_url)
		 VALUES ($1::uuid, $2, $3, $4)
		 ON CONFLICT (group_id, provider) DO UPDATE SET
		     encrypted_api_key = EXCLUDED.encrypted_api_key,
		     base_url = EXCLUDED.base_url,
		     is_active = TRUE`,
		groupID, provider, encryptedKey, baseURL)
	return err
}

func DeleteGroupCredential(ctx context.Context, db *sql.DB, groupID, provider string) error {
	_, err := db.ExecContext(ctx,
		`DELETE FROM group_credentials WHERE group_id = $1::uuid AND provider = $2`,
		groupID, provider)
	return err
}

func ListGroupCredentials(ctx context.Context, db *sql.DB, groupID string) ([]*GroupCredential, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id::text, group_id::text, provider, encrypted_api_key, base_url, is_active, created_at
		 FROM group_credentials WHERE group_id = $1::uuid ORDER BY provider`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*GroupCredential
	for rows.Next() {
		c := &GroupCredential{}
		rows.Scan(&c.ID, &c.GroupID, &c.Provider, &c.EncryptedAPIKey, &c.BaseURL, &c.IsActive, &c.CreatedAt)
		result = append(result, c)
	}
	return result, nil
}

func GetGroupQuota(ctx context.Context, db *sql.DB, groupID string) (*GroupQuota, error) {
	q := &GroupQuota{GroupID: groupID}
	err := db.QueryRowContext(ctx,
		`SELECT group_id::text, rpm_limit, rpd_limit, token_daily, token_monthly,
		        spend_daily, spend_monthly, allowed_models
		 FROM group_quotas WHERE group_id = $1::uuid`, groupID,
	).Scan(&q.GroupID, &q.RPMLimit, &q.RPDLimit, &q.TokenDaily, &q.TokenMonthly,
		&q.SpendDaily, &q.SpendMonthly, &q.AllowedModels)
	if err == sql.ErrNoRows {
		return q, nil
	}
	if err != nil {
		return nil, err
	}
	return q, nil
}

func SetGroupQuota(ctx context.Context, db *sql.DB, q *GroupQuota) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO group_quotas (group_id, rpm_limit, rpd_limit, token_daily, token_monthly, spend_daily, spend_monthly, allowed_models)
		 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (group_id) DO UPDATE SET
		     rpm_limit = EXCLUDED.rpm_limit,
		     rpd_limit = EXCLUDED.rpd_limit,
		     token_daily = EXCLUDED.token_daily,
		     token_monthly = EXCLUDED.token_monthly,
		     spend_daily = EXCLUDED.spend_daily,
		     spend_monthly = EXCLUDED.spend_monthly,
		     allowed_models = EXCLUDED.allowed_models,
		     updated_at = NOW()`,
		q.GroupID, q.RPMLimit, q.RPDLimit, q.TokenDaily, q.TokenMonthly,
		q.SpendDaily, q.SpendMonthly, q.AllowedModels)
	return err
}
