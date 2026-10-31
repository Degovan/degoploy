package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleDeployer Role = "deployer"
	RoleViewer   Role = "viewer"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleDeployer, RoleViewer:
		return true
	}
	return false
}

const (
	ProviderLocal = "local"
	ProviderOIDC  = "oidc"
)

type User struct {
	ID           int64
	Username     string
	Email        string
	DisplayName  string
	AuthProvider string
	OIDCIssuer   string
	OIDCSubject  string
	PasswordHash string
	Role         Role
	Disabled     bool
	// MustChangePassword blocks everything but a password change until cleared.
	MustChangePassword bool
	// SelectedProjects limits a deployer or viewer to the projects in
	// project_members, with the role given there. Admins ignore it.
	SelectedProjects bool
	LastLoginAt        *time.Time
	CreatedAt          time.Time
}

// ErrDuplicate is returned when a unique constraint is violated.
var ErrDuplicate = errors.New("already exists")

const userCols = `id, username, email, display_name, auth_provider,
	COALESCE(oidc_issuer, ''), COALESCE(oidc_subject, ''), COALESCE(password_hash, ''),
	role, disabled, must_change_password, selected_projects, last_login_at, created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var last sql.NullTime
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.AuthProvider,
		&u.OIDCIssuer, &u.OIDCSubject, &u.PasswordHash,
		&u.Role, &u.Disabled, &u.MustChangePassword, &u.SelectedProjects, &last, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if last.Valid {
		u.LastLoginAt = &last.Time
	}
	return &u, nil
}

func (s *Store) CreateUser(ctx context.Context, u *User) error {
	u.CreatedAt = now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO users
		(username, email, display_name, auth_provider, oidc_issuer, oidc_subject, password_hash, role, disabled, created_at)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)`,
		u.Username, u.Email, u.DisplayName, u.AuthProvider, u.OIDCIssuer, u.OIDCSubject,
		u.PasswordHash, u.Role, u.Disabled, u.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		return err
	}
	u.ID, err = res.LastInsertId()
	return err
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (s *Store) UserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username = ?`, username))
}

func (s *Store) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// SetUserPassword stores a new hash; mustChange asks for another change at next sign-in.
func (s *Store) SetUserPassword(ctx context.Context, id int64, hash string, mustChange bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, must_change_password = ? WHERE id = ?`,
		hash, mustChange, id)
	return err
}

func (s *Store) TouchUserLogin(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, now(), id)
	return err
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// UpdateUser saves the editable profile fields: email, display name, role, disabled.
func (s *Store) UpdateUser(ctx context.Context, u *User) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET email = ?, display_name = ?, role = ?, disabled = ?
		WHERE id = ?`, u.Email, u.DisplayName, u.Role, u.Disabled, u.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountActiveAdmins counts enabled admin accounts.
func (s *Store) CountActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ? AND disabled = 0`, RoleAdmin).Scan(&n)
	return n, err
}

// UserByOIDC finds the account linked to an identity provider subject.
func (s *Store) UserByOIDC(ctx context.Context, issuer, subject string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users
		WHERE oidc_issuer = ? AND oidc_subject = ?`, issuer, subject))
}
