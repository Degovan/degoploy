package store

import (
	"context"
	"database/sql"
	"errors"
)

// ProjectMember gives a user with SelectedProjects a role on one project.
type ProjectMember struct {
	ProjectID int64
	Role      Role
}

// ProjectMemberRole returns a user's role on a project, if they are a member.
// It matches auth.ProjectRoleLookup.
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

// UserProjects lists the projects a user is a member of, by project name.
func (s *Store) UserProjects(ctx context.Context, userID int64) ([]ProjectMember, error) {
	all, err := s.queryMembers(ctx, `WHERE m.user_id = ?`, userID)
	return all[userID], err
}

// AllProjectMembers returns every user's memberships, by user ID.
func (s *Store) AllProjectMembers(ctx context.Context) (map[int64][]ProjectMember, error) {
	return s.queryMembers(ctx, ``)
}

func (s *Store) queryMembers(ctx context.Context, where string, args ...any) (map[int64][]ProjectMember, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.user_id, m.project_id, m.role FROM project_members m
		JOIN projects p ON p.id = m.project_id `+where+` ORDER BY p.name COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]ProjectMember{}
	for rows.Next() {
		var uid int64
		var m ProjectMember
		if err := rows.Scan(&uid, &m.ProjectID, &m.Role); err != nil {
			return nil, err
		}
		out[uid] = append(out[uid], m)
	}
	return out, rows.Err()
}

// SetUserProjects sets whether a user sees only selected projects and, if so,
// which ones. Without selected, the user's memberships are removed.
func (s *Store) SetUserProjects(ctx context.Context, userID int64, selected bool, members []ProjectMember) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE users SET selected_projects = ? WHERE id = ?`, selected, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_members WHERE user_id = ?`, userID); err != nil {
			return err
		}
		if !selected {
			return nil
		}
		for _, m := range members {
			if _, err := tx.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES (?, ?, ?)`,
				m.ProjectID, userID, m.Role); err != nil {
				return err
			}
		}
		return nil
	})
}
