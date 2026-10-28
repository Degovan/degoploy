package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Invite is a pending invitation. TokenHash is the hash of the secret link
// token; the token itself is only shown once, when the invite is created.
type Invite struct {
	ID        int64
	Email     string
	Role      Role
	TokenHash string
	InvitedBy *int64
	CreatedAt time.Time
	ExpiresAt time.Time

	// InvitedByName is the username of InvitedBy (read-only, joined).
	InvitedByName string
}

const inviteCols = `i.id, i.email, i.role, i.token_hash, i.invited_by, i.created_at, i.expires_at,
	COALESCE(u.username, '')`

const inviteFrom = ` FROM invites i LEFT JOIN users u ON u.id = i.invited_by`

func scanInvite(row interface{ Scan(...any) error }) (*Invite, error) {
	var inv Invite
	err := row.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.InvitedBy, &inv.CreatedAt,
		&inv.ExpiresAt, &inv.InvitedByName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// ListInvites returns all pending invites, newest first, expired ones included.
func (s *Store) ListInvites(ctx context.Context) ([]*Invite, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+inviteCols+inviteFrom+` ORDER BY i.created_at DESC, i.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Invite{}
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (s *Store) InviteByID(ctx context.Context, id int64) (*Invite, error) {
	return scanInvite(s.db.QueryRowContext(ctx, `SELECT `+inviteCols+inviteFrom+` WHERE i.id = ?`, id))
}

func (s *Store) InviteByTokenHash(ctx context.Context, hash string) (*Invite, error) {
	return scanInvite(s.db.QueryRowContext(ctx, `SELECT `+inviteCols+inviteFrom+` WHERE i.token_hash = ?`, hash))
}

// CreateInvite inserts inv; ErrDuplicate means the email already has an invite.
func (s *Store) CreateInvite(ctx context.Context, inv *Invite) error {
	inv.CreatedAt = now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO invites
		(email, role, token_hash, invited_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		inv.Email, inv.Role, inv.TokenHash, inv.InvitedBy, inv.CreatedAt, inv.ExpiresAt.UTC())
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	inv.ID, err = res.LastInsertId()
	return err
}

// RenewInvite replaces the token and expiry, which invalidates the old link.
func (s *Store) RenewInvite(ctx context.Context, inv *Invite) error {
	res, err := s.db.ExecContext(ctx, `UPDATE invites SET token_hash = ?, expires_at = ? WHERE id = ?`,
		inv.TokenHash, inv.ExpiresAt.UTC(), inv.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteInvite(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM invites WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AcceptInvite creates u and deletes the invite in one transaction, so a link
// can be used only once. ErrNotFound means the invite is already gone;
// ErrDuplicate means the username is taken.
func (s *Store) AcceptInvite(ctx context.Context, inviteID int64, u *User) error {
	u.CreatedAt = now()
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM invites WHERE id = ?`, inviteID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		res, err = tx.ExecContext(ctx, `INSERT INTO users
			(username, email, display_name, auth_provider, password_hash, role, disabled, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
			u.Username, u.Email, u.DisplayName, u.AuthProvider, u.PasswordHash, u.Role, u.CreatedAt)
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		if err != nil {
			return err
		}
		u.ID, err = res.LastInsertId()
		return err
	})
}
