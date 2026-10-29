package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	ScriptManaged = "managed"
	ScriptRepo    = "repo"
)

type Project struct {
	ID            int64
	PublicID      string
	Slug          string
	Name          string
	Provider      string
	RepoURL       string
	WorkDir       string
	ScriptSource  string
	ScriptPath    string
	WebhookSecret string
	AutoPull      bool
	TimeoutSec    int
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const projectCols = `id, public_id, slug, name, provider, repo_url, work_dir, script_source, script_path,
	webhook_secret, auto_pull, timeout_sec, enabled, created_at, updated_at`

func scanProject(row interface{ Scan(...any) error }) (*Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Provider, &p.RepoURL, &p.WorkDir,
		&p.ScriptSource, &p.ScriptPath, &p.WebhookSecret, &p.AutoPull, &p.TimeoutSec, &p.Enabled,
		&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreateProject(ctx context.Context, p *Project) error {
	p.CreatedAt = now()
	p.UpdatedAt = p.CreatedAt
	res, err := s.db.ExecContext(ctx, `INSERT INTO projects
		(public_id, slug, name, provider, repo_url, work_dir, script_source, script_path,
		 webhook_secret, auto_pull, timeout_sec, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.PublicID, p.Slug, p.Name, p.Provider, p.RepoURL, p.WorkDir, p.ScriptSource, p.ScriptPath,
		p.WebhookSecret, p.AutoPull, p.TimeoutSec, p.Enabled, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

// UpdateProject saves every mutable column. Slug and public_id never change.
func (s *Store) UpdateProject(ctx context.Context, p *Project) error {
	p.UpdatedAt = now()
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET
		name = ?, provider = ?, repo_url = ?, work_dir = ?, script_source = ?, script_path = ?,
		webhook_secret = ?, auto_pull = ?, timeout_sec = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, p.Provider, p.RepoURL, p.WorkDir, p.ScriptSource, p.ScriptPath,
		p.WebhookSecret, p.AutoPull, p.TimeoutSec, p.Enabled, p.UpdatedAt, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ProjectByID(ctx context.Context, id int64) (*Project, error) {
	return scanProject(s.db.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE id = ?`, id))
}

func (s *Store) ProjectByPublicID(ctx context.Context, publicID string) (*Project, error) {
	return scanProject(s.db.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE public_id = ?`, publicID))
}

func (s *Store) ListProjects(ctx context.Context) ([]*Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+projectCols+` FROM projects ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type ScriptRevision struct {
	ID        int64
	ProjectID int64
	Content   string
	CreatedBy *int64
	Username  string // joined, read-only
	CreatedAt time.Time
}

func (s *Store) AddScriptRevision(ctx context.Context, r *ScriptRevision) error {
	r.CreatedAt = now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO script_revisions (project_id, content, created_by, created_at)
		VALUES (?, ?, ?, ?)`, r.ProjectID, r.Content, r.CreatedBy, r.CreatedAt)
	if err != nil {
		return err
	}
	r.ID, err = res.LastInsertId()
	return err
}

// ListScriptRevisions returns the newest revisions first.
func (s *Store) ListScriptRevisions(ctx context.Context, projectID int64, limit int) ([]*ScriptRevision, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.project_id, r.content, r.created_by,
		COALESCE(u.username, ''), r.created_at
		FROM script_revisions r LEFT JOIN users u ON u.id = r.created_by
		WHERE r.project_id = ? ORDER BY r.id DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ScriptRevision
	for rows.Next() {
		var r ScriptRevision
		var by sql.NullInt64
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Content, &by, &r.Username, &r.CreatedAt); err != nil {
			return nil, err
		}
		if by.Valid {
			r.CreatedBy = &by.Int64
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}
