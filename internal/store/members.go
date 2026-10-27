package store

import (
	"context"
	"database/sql"
	"errors"
)

// ProjectMemberRole returns the per-project role override for a user.
// It matches auth.ProjectRoleLookup; it is not wired in yet (global roles only).
func (s *Store) ProjectMemberRole(ctx context.Context, projectID, userID int64) (Role, bool, error) {
	var r Role
	err := s.db.QueryRowContext(ctx, `SELECT role FROM project_members WHERE project_id = ? AND user_id = ?`,
		projectID, userID).Scan(&r)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return r, true, nil
}
