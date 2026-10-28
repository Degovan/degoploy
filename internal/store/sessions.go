package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Session struct {
	IDHash    string
	UserID    int64
	CSRFToken string
	ExpiresAt time.Time
	CreatedAt time.Time
	UserAgent string
	IP        string
}

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	sess.CreatedAt = now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions
		(id_hash, user_id, csrf_token, expires_at, created_at, user_agent, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sess.IDHash, sess.UserID, sess.CSRFToken, sess.ExpiresAt.UTC(), sess.CreatedAt, sess.UserAgent, sess.IP)
	return err
}

// SessionWithUser returns a non-expired session and its owner.
func (s *Store) SessionWithUser(ctx context.Context, idHash string) (*Session, *User, error) {
	var sess Session
	err := s.db.QueryRowContext(ctx, `SELECT id_hash, user_id, csrf_token, expires_at, created_at, user_agent, ip
		FROM sessions WHERE id_hash = ? AND expires_at > ?`, idHash, now()).
		Scan(&sess.IDHash, &sess.UserID, &sess.CSRFToken, &sess.ExpiresAt, &sess.CreatedAt, &sess.UserAgent, &sess.IP)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	u, err := s.UserByID(ctx, sess.UserID)
	if err != nil {
		return nil, nil, err
	}
	return &sess, u, nil
}

func (s *Store) ExtendSession(ctx context.Context, idHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE id_hash = ?`, expiresAt.UTC(), idHash)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Store) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteUserSessionsExcept revokes every session of a user but the one given.
func (s *Store) DeleteUserSessionsExcept(ctx context.Context, userID int64, keepHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id_hash <> ?`, userID, keepHash)
	return err
}

// CountUserSessions counts a user's unexpired sessions, leaving out exceptHash.
func (s *Store) CountUserSessions(ctx context.Context, userID int64, exceptHash string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions
		WHERE user_id = ? AND expires_at > ? AND id_hash <> ?`, userID, now(), exceptHash).Scan(&n)
	return n, err
}
