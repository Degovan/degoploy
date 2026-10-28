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
	// Query matches the actor, action, target, detail or IP (case-insensitive).
	Query string
	// Actions keeps only these exact actions (e.g. the sensitive ones).
	Actions []string
	Since   time.Time
}

const auditFrom = ` FROM audit_logs a LEFT JOIN users u ON u.id = a.user_id`

// where builds the filter clause; the pagination cursor and limit are not part of it.
func (f AuditFilter) where() (string, []any) {
	q := ` WHERE 1 = 1`
	var args []any
	if f.ActionPrefix != "" {
		q += ` AND a.action LIKE ? ESCAPE '\'`
		args = append(args, escapeLike(f.ActionPrefix)+"%")
	}
	if f.UserID != 0 {
		q += ` AND a.user_id = ?`
		args = append(args, f.UserID)
	}
	if f.Query != "" {
		like := "%" + escapeLike(f.Query) + "%"
		q += ` AND (COALESCE(u.username, '') LIKE ? ESCAPE '\' OR a.action LIKE ? ESCAPE '\'
			OR a.target_id LIKE ? ESCAPE '\' OR a.detail LIKE ? ESCAPE '\' OR a.ip LIKE ? ESCAPE '\')`
		args = append(args, like, like, like, like, like)
	}
	if len(f.Actions) > 0 {
		q += ` AND a.action IN (?` + strings.Repeat(`, ?`, len(f.Actions)-1) + `)`
		for _, act := range f.Actions {
			args = append(args, act)
		}
	}
	if !f.Since.IsZero() {
		q += ` AND a.created_at >= ?`
		args = append(args, f.Since.UTC())
	}
	return q, args
}

// ListAudit returns entries newest first.
func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]*AuditEntry, error) {
	if f.BeforeID <= 0 {
		f.BeforeID = 1<<63 - 1
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	where, args := f.where()
	q := `SELECT a.id, a.user_id, COALESCE(u.username, ''), a.action,
		a.target_type, a.target_id, a.detail, a.ip, a.created_at` + auditFrom + where +
		` AND a.id < ? ORDER BY a.id DESC LIMIT ?`
	args = append(args, f.BeforeID, f.Limit)

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

// CountAudit counts the entries matching f, ignoring its cursor and limit.
func (s *Store) CountAudit(ctx context.Context, f AuditFilter) (int, error) {
	where, args := f.where()
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+auditFrom+where, args...).Scan(&n)
	return n, err
}

// AuditDayCounts counts the entries matching f per local calendar day, for the
// given days ("2006-01-02"). offset is the local zone's offset from UTC.
func (s *Store) AuditDayCounts(ctx context.Context, f AuditFilter, offset time.Duration, days []string) (map[string]int, error) {
	out := map[string]int{}
	if len(days) == 0 {
		return out, nil
	}
	// Day boundaries are computed in Go so the query compares plain timestamps.
	where, args := f.where()
	for _, day := range days {
		start, err := time.Parse(time.DateOnly, day)
		if err != nil {
			return nil, err
		}
		from := start.Add(-offset).UTC()
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+auditFrom+where+
			` AND a.created_at >= ? AND a.created_at < ?`,
			append(args, from, from.Add(24*time.Hour))...).Scan(&n); err != nil {
			return nil, err
		}
		out[day] = n
	}
	return out, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
