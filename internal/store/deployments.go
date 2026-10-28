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
	// ScriptStartedAt ends the repository update stage and starts the script stage.
	ScriptStartedAt *time.Time
	FinishedAt      *time.Time
	// Error summarizes why a deployment did not succeed (last output line or cause).
	Error string
	// AutoPull records whether the run updated the repository; nil before it starts.
	AutoPull *bool
	// ScriptRevisionID is the managed script revision the run executed; nil for
	// repository scripts and runs that never reached the script.
	ScriptRevisionID *int64
}

const deploymentCols = `id, project_id, rule_id, trigger, triggered_by, ref, commit_sha, commit_message,
	author, status, exit_code, log_path, queued_at, started_at, script_started_at, finished_at, error, auto_pull,
	script_revision_id`

func scanDeployment(row interface{ Scan(...any) error }) (*Deployment, error) {
	var d Deployment
	var rule, by, exit, rev sql.NullInt64
	var started, scriptStarted, finished sql.NullTime
	var autoPull sql.NullBool
	err := row.Scan(&d.ID, &d.ProjectID, &rule, &d.Trigger, &by, &d.Ref, &d.CommitSHA, &d.CommitMessage,
		&d.Author, &d.Status, &exit, &d.LogPath, &d.QueuedAt, &started, &scriptStarted, &finished, &d.Error, &autoPull, &rev)
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
	if scriptStarted.Valid {
		d.ScriptStartedAt = &scriptStarted.Time
	}
	if finished.Valid {
		d.FinishedAt = &finished.Time
	}
	if autoPull.Valid {
		d.AutoPull = &autoPull.Bool
	}
	if rev.Valid {
		d.ScriptRevisionID = &rev.Int64
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
	d.commit_message, d.author, d.status, d.exit_code, d.log_path, d.queued_at, d.started_at, d.script_started_at,
	d.finished_at, d.error, d.auto_pull, d.script_revision_id, COALESCE(r.name, ''), COALESCE(u.username, '')`

const deploymentViewFrom = ` FROM deployments d
	LEFT JOIN trigger_rules r ON r.id = d.rule_id
	LEFT JOIN users u ON u.id = d.triggered_by`

func scanDeploymentView(row interface{ Scan(...any) error }) (*DeploymentView, error) {
	var v DeploymentView
	var rule, by, exit, rev sql.NullInt64
	var started, scriptStarted, finished sql.NullTime
	var autoPull sql.NullBool
	err := row.Scan(&v.ID, &v.ProjectID, &rule, &v.Trigger, &by, &v.Ref, &v.CommitSHA, &v.CommitMessage,
		&v.Author, &v.Status, &exit, &v.LogPath, &v.QueuedAt, &started, &scriptStarted, &finished, &v.Error,
		&autoPull, &rev, &v.RuleName, &v.TriggeredByName)
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
	if scriptStarted.Valid {
		v.ScriptStartedAt = &scriptStarted.Time
	}
	if finished.Valid {
		v.FinishedAt = &finished.Time
	}
	if autoPull.Valid {
		v.AutoPull = &autoPull.Bool
	}
	if rev.Valid {
		v.ScriptRevisionID = &rev.Int64
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

// DeploymentNeighbors returns the ids of the project's deployments just before
// and after id, or nil at either end.
func (s *Store) DeploymentNeighbors(ctx context.Context, projectID, id int64) (prev, next *int64, err error) {
	var p, n sql.NullInt64
	err = s.db.QueryRowContext(ctx, `SELECT
		(SELECT MAX(id) FROM deployments WHERE project_id = ? AND id < ?),
		(SELECT MIN(id) FROM deployments WHERE project_id = ? AND id > ?)`,
		projectID, id, projectID, id).Scan(&p, &n)
	if err != nil {
		return nil, nil, err
	}
	if p.Valid {
		prev = &p.Int64
	}
	if n.Valid {
		next = &n.Int64
	}
	return prev, next, nil
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

// RecentDeployments returns the newest deployments across all projects; with
// activeOnly, only queued and running ones.
func (s *Store) RecentDeployments(ctx context.Context, limit int, activeOnly bool) ([]*DeploymentView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+deploymentViewCols+deploymentViewFrom+`
		WHERE (? = 0 OR d.status IN (?, ?)) ORDER BY d.id DESC LIMIT ?`, activeOnly, DeployQueued, DeployRunning, limit)
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

// Health counts how many of a project's last window finished deployments
// succeeded. Skipped and canceled runs say nothing about health and are ignored.
type Health struct {
	Passed int
	Total  int
}

// DeploymentHealth returns the health of every project with finished
// deployments, or of one project when projectID is not 0.
func (s *Store) DeploymentHealth(ctx context.Context, projectID int64, window int) (map[int64]Health, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT project_id, SUM(status = ?), COUNT(*) FROM (
			SELECT project_id, status, ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY id DESC) AS n
			FROM deployments WHERE status IN (?, ?, ?) AND (? = 0 OR project_id = ?)
		) WHERE n <= ? GROUP BY project_id`,
		DeploySuccess, DeploySuccess, DeployFailed, DeployTimeout, projectID, projectID, window)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Health{}
	for rows.Next() {
		var id int64
		var h Health
		if err := rows.Scan(&id, &h.Passed, &h.Total); err != nil {
			return nil, err
		}
		out[id] = h
	}
	return out, rows.Err()
}

// StartDeployment moves a queued deployment to running, recording whether it will
// update the repository first. It reports false if the deployment was no longer
// queued (e.g. canceled meanwhile).
func (s *Store) StartDeployment(ctx context.Context, id int64, logPath string, autoPull bool) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE deployments SET status = ?, log_path = ?, started_at = ?, auto_pull = ?
		WHERE id = ? AND status = ?`, DeployRunning, logPath, now(), autoPull, id, DeployQueued)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// MarkScriptStarted records that the repository is ready and the deploy script is
// starting. For a managed script it also records the current (newest) revision,
// which is what the script file on disk holds.
func (s *Store) MarkScriptStarted(ctx context.Context, id int64, managed bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET script_started_at = ?,
		script_revision_id = CASE WHEN ? THEN
			(SELECT MAX(r.id) FROM script_revisions r WHERE r.project_id = deployments.project_id) END
		WHERE id = ? AND status = ?`, now(), managed, id, DeployRunning)
	return err
}

// FinishDeployment records the outcome of a running deployment; errLine is
// empty on success.
func (s *Store) FinishDeployment(ctx context.Context, id int64, status string, exitCode *int, errLine string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET status = ?, exit_code = ?, finished_at = ?, error = ?
		WHERE id = ? AND status = ?`, status, exitCode, now(), errLine, id, DeployRunning)
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
	res, err := s.db.ExecContext(ctx, `UPDATE deployments SET status = ?, finished_at = ?, error = ? WHERE status = ?`,
		DeployFailed, now(), "interrupted: degoploy stopped while this deployment was running", DeployRunning)
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
