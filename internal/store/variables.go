package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Variable is a project-level environment variable. For secrets Value holds the
// sealed ciphertext, never the plaintext.
type Variable struct {
	ID        int64
	ProjectID int64
	Name      string
	Value     string
	Secret    bool
	UpdatedBy *int64
	CreatedAt time.Time
	UpdatedAt time.Time

	// UpdatedByName is the username of UpdatedBy (read-only, joined).
	UpdatedByName string
}

const variableCols = `v.id, v.project_id, v.name, v.value, v.secret, v.updated_by, v.created_at, v.updated_at,
	COALESCE(u.username, '')`

const variableFrom = ` FROM project_variables v LEFT JOIN users u ON u.id = v.updated_by`

func scanVariable(row interface{ Scan(...any) error }) (*Variable, error) {
	var v Variable
	err := row.Scan(&v.ID, &v.ProjectID, &v.Name, &v.Value, &v.Secret, &v.UpdatedBy, &v.CreatedAt, &v.UpdatedAt,
		&v.UpdatedByName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListVariables returns a project's variables ordered by name.
func (s *Store) ListVariables(ctx context.Context, projectID int64) ([]*Variable, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+variableCols+variableFrom+`
		WHERE v.project_id = ? ORDER BY v.name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Variable{}
	for rows.Next() {
		v, err := scanVariable(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) VariableByID(ctx context.Context, projectID, id int64) (*Variable, error) {
	return scanVariable(s.db.QueryRowContext(ctx, `SELECT `+variableCols+variableFrom+`
		WHERE v.project_id = ? AND v.id = ?`, projectID, id))
}

// CreateVariable inserts v; ErrDuplicate means the project already has the name.
func (s *Store) CreateVariable(ctx context.Context, v *Variable) error {
	v.CreatedAt = now()
	v.UpdatedAt = v.CreatedAt
	res, err := s.db.ExecContext(ctx, `INSERT INTO project_variables
		(project_id, name, value, secret, updated_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.ProjectID, v.Name, v.Value, v.Secret, v.UpdatedBy, v.CreatedAt, v.UpdatedAt)
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	v.ID, err = res.LastInsertId()
	return err
}

// UpdateVariable saves name, value and secret of v.
func (s *Store) UpdateVariable(ctx context.Context, v *Variable) error {
	v.UpdatedAt = now()
	res, err := s.db.ExecContext(ctx, `UPDATE project_variables
		SET name = ?, value = ?, secret = ?, updated_by = ?, updated_at = ? WHERE project_id = ? AND id = ?`,
		v.Name, v.Value, v.Secret, v.UpdatedBy, v.UpdatedAt, v.ProjectID, v.ID)
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteVariable(ctx context.Context, projectID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM project_variables WHERE project_id = ? AND id = ?`, projectID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
