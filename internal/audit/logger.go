package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"
)

type Entry struct {
	UserID    string         `json:"user_id,omitempty"`
	APIKeyID  string         `json:"api_key_id,omitempty"`
	Action    string         `json:"action"`
	Resource  string         `json:"resource,omitempty"`
	Detail    interface{}    `json:"detail,omitempty"`
	IPAddress string         `json:"ip_address,omitempty"`
	UserAgent string         `json:"user_agent,omitempty"`
}

type Logger struct {
	db    *sql.DB
	queue chan *Entry
}

func NewLogger(db *sql.DB, bufferSize int) *Logger {
	if bufferSize <= 0 {
		bufferSize = 256
	}
	l := &Logger{
		db:    db,
		queue: make(chan *Entry, bufferSize),
	}
	go l.drain()
	return l
}

func (l *Logger) Log(entry *Entry) {
	select {
	case l.queue <- entry:
	default:
		// Queue full, drop to avoid blocking
		log.Println("audit: queue full, dropping entry")
	}
}

func (l *Logger) drain() {
	for entry := range l.queue {
		l.write(entry)
	}
}

func (l *Logger) write(e *Entry) {
	var detailJSON []byte
	if e.Detail != nil {
		detailJSON, _ = json.Marshal(e.Detail)
	}

	var userID, apiKeyID sql.NullString
	if e.UserID != "" {
		userID = sql.NullString{String: e.UserID, Valid: true}
	}
	if e.APIKeyID != "" {
		apiKeyID = sql.NullString{String: e.APIKeyID, Valid: true}
	}

	_, err := l.db.ExecContext(context.Background(),
		`INSERT INTO audit_log (user_id, api_key_id, action, resource, detail, ip_address, user_agent)
		 VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)`,
		userID, apiKeyID, e.Action, e.Resource, detailJSON, e.IPAddress, e.UserAgent)
	if err != nil {
		log.Printf("audit: failed to write entry: %v", err)
	}
}

type AuditEntry struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Detail    string    `json:"detail"`
	IPAddress string    `json:"ip_address"`
	CreatedAt time.Time `json:"created_at"`
}

func QueryLogs(ctx context.Context, db *sql.DB, action string, page, pageSize int) ([]*AuditEntry, int, error) {
	var total int
	countQuery := `SELECT COUNT(*) FROM audit_log`
	query := `SELECT a.id::text, COALESCE(a.user_id::text, ''), COALESCE(u.username, ''),
	                 a.action, COALESCE(a.resource, ''), COALESCE(a.detail::text, ''),
	                 COALESCE(a.ip_address, ''), a.created_at
	          FROM audit_log a LEFT JOIN users u ON a.user_id = u.id`
	args := []interface{}{}

	if action != "" {
		countQuery += ` WHERE action = $1`
		query += ` WHERE a.action = $1`
		args = append(args, action)
	}

	db.QueryRowContext(ctx, countQuery, args...).Scan(&total)

	query += ` ORDER BY a.created_at DESC LIMIT $` + string(rune('0'+len(args)+1)) + ` OFFSET $` + string(rune('0'+len(args)+2))
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		rows.Scan(&e.ID, &e.UserID, &e.Username, &e.Action, &e.Resource, &e.Detail, &e.IPAddress, &e.CreatedAt)
		result = append(result, e)
	}
	return result, total, nil
}
