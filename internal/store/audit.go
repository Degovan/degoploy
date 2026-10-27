package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type AuditEntry struct {
	ID         int64
	UserID     *int64
	Username   string // joined, read-only
	Action     string
	TargetType string
	TargetID   string
	Detail     string // JSON object
	IP         string
	CreatedAt  time.Time
}

func (s *Store) InsertAudit(ctx context.Context, e *AuditEntry) error {
	if e.Detail == "" {
		e.Detail = "{}"
	}
	e.CreatedAt = now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO audit_logs
		(user_id, action, target_type, target_id, detail, ip, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.Action, e.TargetType, e.TargetID, e.Detail, e.IP, e.CreatedAt)
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

// AuditFilter narrows ListAudit. Zero values mean "no filter".
type AuditFilter struct {
	BeforeID     int64 // only entries with a smaller id (pagination cursor)
	Limit        int
	ActionPrefix string // e.g. "project." or "auth.login"
	UserID       int64
}

// ListAudit returns entries newest first.
func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]*AuditEntry, error) {
	if f.BeforeID <= 0 {
		f.BeforeID = 1<<63 - 1
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	q := `SELECT a.id, a.user_id, COALESCE(u.username, ''), a.action,
		a.target_type, a.target_id, a.detail, a.ip, a.created_at
		FROM audit_logs a LEFT JOIN users u ON u.id = a.user_id
		WHERE a.id < ?`
	args := []any{f.BeforeID}
	if f.ActionPrefix != "" {
		q += ` AND a.action LIKE ? ESCAPE '\'`
		args = append(args, escapeLike(f.ActionPrefix)+"%")
	}
	if f.UserID != 0 {
		q += ` AND a.user_id = ?`
		args = append(args, f.UserID)
	}
	q += ` ORDER BY a.id DESC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var uid sql.NullInt64
		if err := rows.Scan(&e.ID, &uid, &e.Username, &e.Action, &e.TargetType, &e.TargetID,
			&e.Detail, &e.IP, &e.CreatedAt); err != nil {
			return nil, err
		}
		if uid.Valid {
			e.UserID = &uid.Int64
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
