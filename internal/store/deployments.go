package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	DeployQueued   = "queued"
	DeployRunning  = "running"
	DeploySuccess  = "success"
	DeployFailed   = "failed"
	DeployCanceled = "canceled"
	DeployTimeout  = "timeout"
	DeploySkipped  = "skipped"

	TriggerWebhook  = "webhook"
	TriggerManual   = "manual"
	TriggerRedeploy = "redeploy"
)

type Deployment struct {
	ID            int64
	ProjectID     int64
	RuleID        *int64
	Trigger       string
	TriggeredBy   *int64
	Ref           string
	CommitSHA     string
	CommitMessage string
	Author        string
	Status        string
	ExitCode      *int
	LogPath       string
	QueuedAt      time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

const deploymentCols = `id, project_id, rule_id, trigger, triggered_by, ref, commit_sha, commit_message,
	author, status, exit_code, log_path, queued_at, started_at, finished_at`

func scanDeployment(row interface{ Scan(...any) error }) (*Deployment, error) {
	var d Deployment
	var rule, by, exit sql.NullInt64
	var started, finished sql.NullTime
	err := row.Scan(&d.ID, &d.ProjectID, &rule, &d.Trigger, &by, &d.Ref, &d.CommitSHA, &d.CommitMessage,
		&d.Author, &d.Status, &exit, &d.LogPath, &d.QueuedAt, &started, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if rule.Valid {
		d.RuleID = &rule.Int64
	}
	if by.Valid {
		d.TriggeredBy = &by.Int64
	}
	if exit.Valid {
		c := int(exit.Int64)
		d.ExitCode = &c
	}
	if started.Valid {
		d.StartedAt = &started.Time
	}
	if finished.Valid {
		d.FinishedAt = &finished.Time
	}
	return &d, nil
}

func (s *Store) CreateDeployment(ctx context.Context, d *Deployment) error {
	d.QueuedAt = now()
	if d.Status == "" {
		d.Status = DeployQueued
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO deployments
		(project_id, rule_id, trigger, triggered_by, ref, commit_sha, commit_message, author, status, log_path, queued_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ProjectID, d.RuleID, d.Trigger, d.TriggeredBy, d.Ref, d.CommitSHA, d.CommitMessage, d.Author,
		d.Status, d.LogPath, d.QueuedAt)
	if err != nil {
		return err
	}
	d.ID, err = res.LastInsertId()
	return err
}

func (s *Store) DeploymentByID(ctx context.Context, id int64) (*Deployment, error) {
	return scanDeployment(s.db.QueryRowContext(ctx, `SELECT `+deploymentCols+` FROM deployments WHERE id = ?`, id))
}

// DeploymentView is a deployment joined with display names for the API.
type DeploymentView struct {
	Deployment
	RuleName        string
	TriggeredByName string
}

const deploymentViewCols = `d.id, d.project_id, d.rule_id, d.trigger, d.triggered_by, d.ref, d.commit_sha,
	d.commit_message, d.author, d.status, d.exit_code, d.log_path, d.queued_at, d.started_at, d.finished_at,
	COALESCE(r.name, ''), COALESCE(u.username, '')`

const deploymentViewFrom = ` FROM deployments d
	LEFT JOIN trigger_rules r ON r.id = d.rule_id
	LEFT JOIN users u ON u.id = d.triggered_by`

func scanDeploymentView(row interface{ Scan(...any) error }) (*DeploymentView, error) {
	var v DeploymentView
	var rule, by, exit sql.NullInt64
	var started, finished sql.NullTime
	err := row.Scan(&v.ID, &v.ProjectID, &rule, &v.Trigger, &by, &v.Ref, &v.CommitSHA, &v.CommitMessage,
		&v.Author, &v.Status, &exit, &v.LogPath, &v.QueuedAt, &started, &finished, &v.RuleName, &v.TriggeredByName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if rule.Valid {
		v.RuleID = &rule.Int64
	}
	if by.Valid {
		v.TriggeredBy = &by.Int64
	}
	if exit.Valid {
		c := int(exit.Int64)
		v.ExitCode = &c
	}
	if started.Valid {
		v.StartedAt = &started.Time
	}
	if finished.Valid {
		v.FinishedAt = &finished.Time
	}
	return &v, nil
}

func (s *Store) DeploymentViewByID(ctx context.Context, id int64) (*DeploymentView, error) {
	return scanDeploymentView(s.db.QueryRowContext(ctx, `SELECT `+deploymentViewCols+deploymentViewFrom+` WHERE d.id = ?`, id))
}

// ListDeployments returns a project's deployments newest first; beforeID 0 starts at the latest.
func (s *Store) ListDeployments(ctx context.Context, projectID, beforeID int64, limit int) ([]*DeploymentView, error) {
	if beforeID <= 0 {
		beforeID = 1<<63 - 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+deploymentViewCols+deploymentViewFrom+`
		WHERE d.project_id = ? AND d.id < ? ORDER BY d.id DESC LIMIT ?`, projectID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*DeploymentView{}
	for rows.Next() {
		v, err := scanDeploymentView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// LatestDeployments returns the newest deployment of every project that has one.
func (s *Store) LatestDeployments(ctx context.Context) (map[int64]*DeploymentView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+deploymentViewCols+deploymentViewFrom+`
		WHERE d.id IN (SELECT MAX(id) FROM deployments GROUP BY project_id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*DeploymentView{}
	for rows.Next() {
		v, err := scanDeploymentView(rows)
		if err != nil {
			return nil, err
		}
		out[v.ProjectID] = v
	}
	return out, rows.Err()
}

// StartDeployment moves a queued deployment to running. It reports false if the
// deployment was no longer queued (e.g. canceled meanwhile).
func (s *Store) StartDeployment(ctx context.Context, id int64, logPath string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE deployments SET status = ?, log_path = ?, started_at = ?
		WHERE id = ? AND status = ?`, DeployRunning, logPath, now(), id, DeployQueued)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// FinishDeployment records the outcome of a running deployment.
func (s *Store) FinishDeployment(ctx context.Context, id int64, status string, exitCode *int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET status = ?, exit_code = ?, finished_at = ?
		WHERE id = ? AND status = ?`, status, exitCode, now(), id, DeployRunning)
	return err
}

// CloseQueuedDeployment marks a still-queued deployment as skipped or canceled.
func (s *Store) CloseQueuedDeployment(ctx context.Context, id int64, status string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE deployments SET status = ?, finished_at = ?
		WHERE id = ? AND status = ?`, status, now(), id, DeployQueued)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *Store) SetDeploymentCommit(ctx context.Context, id int64, sha, message, author string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET commit_sha = ?,
		commit_message = CASE WHEN commit_message = '' THEN ? ELSE commit_message END,
		author = CASE WHEN author = '' THEN ? ELSE author END
		WHERE id = ?`, sha, message, author, id)
	return err
}

// RecoverDeployments runs at startup: deployments that were running when the
// process died are failed; still-queued ones are returned (oldest first) to resubmit.
func (s *Store) RecoverDeployments(ctx context.Context) (failed int64, queued []*Deployment, err error) {
	res, err := s.db.ExecContext(ctx, `UPDATE deployments SET status = ?, finished_at = ? WHERE status = ?`,
		DeployFailed, now(), DeployRunning)
	if err != nil {
		return 0, nil, err
	}
	failed, _ = res.RowsAffected()
	rows, err := s.db.QueryContext(ctx, `SELECT `+deploymentCols+` FROM deployments WHERE status = ? ORDER BY id`, DeployQueued)
	if err != nil {
		return failed, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return failed, nil, err
		}
		queued = append(queued, d)
	}
	return failed, queued, rows.Err()
}

// PruneDeployments deletes all but the newest keep finished deployments of a
// project and returns the log paths of the deleted rows.
func (s *Store) PruneDeployments(ctx context.Context, projectID int64, keep int) ([]string, error) {
	var paths []string
	err := s.tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id, log_path FROM deployments
			WHERE project_id = ? AND status NOT IN (?, ?) ORDER BY id DESC LIMIT -1 OFFSET ?`,
			projectID, DeployQueued, DeployRunning, keep)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			var p string
			if err := rows.Scan(&id, &p); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
			if p != "" {
				paths = append(paths, p)
			}
		}
		rows.Close()
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `DELETE FROM deployments WHERE id = ?`, id); err != nil {
				return err
			}
		}
		return nil
	})
	return paths, err
}
