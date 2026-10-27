package store

import (
	"context"
	"database/sql"
	"time"
)

// WebhookEvent records one webhook delivery and what degoploy did with it.
type WebhookEvent struct {
	ID           int64
	ProjectID    int64
	Provider     string
	Event        string
	DeliveryID   string
	Ref          string
	CommitSHA    string
	Matched      bool
	Reason       string
	DeploymentID *int64
	ReceivedAt   time.Time
}

// keepWebhookEvents is how many deliveries are kept per project.
const keepWebhookEvents = 200

func (s *Store) InsertWebhookEvent(ctx context.Context, e *WebhookEvent) error {
	e.ReceivedAt = now()
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO webhook_events
			(project_id, provider, event, delivery_id, ref, commit_sha, matched, reason, deployment_id, received_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ProjectID, e.Provider, e.Event, e.DeliveryID, e.Ref, e.CommitSHA, e.Matched, e.Reason, e.DeploymentID, e.ReceivedAt)
		if err != nil {
			return err
		}
		if e.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM webhook_events WHERE project_id = ? AND id <= (
			SELECT id FROM webhook_events WHERE project_id = ? ORDER BY id DESC LIMIT 1 OFFSET ?)`,
			e.ProjectID, e.ProjectID, keepWebhookEvents)
		return err
	})
}

func (s *Store) ListWebhookEvents(ctx context.Context, projectID int64, limit int) ([]*WebhookEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, provider, event, delivery_id, ref, commit_sha,
		matched, reason, deployment_id, received_at
		FROM webhook_events WHERE project_id = ? ORDER BY id DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WebhookEvent{}
	for rows.Next() {
		var e WebhookEvent
		var dep sql.NullInt64
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Provider, &e.Event, &e.DeliveryID, &e.Ref, &e.CommitSHA,
			&e.Matched, &e.Reason, &dep, &e.ReceivedAt); err != nil {
			return nil, err
		}
		if dep.Valid {
			e.DeploymentID = &dep.Int64
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
