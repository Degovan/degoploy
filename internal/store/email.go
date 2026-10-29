package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// EmailTest is one test email sent from the Email page.
type EmailTest struct {
	ID          int64
	Recipient   string
	OK          bool
	Detail      string // the server's reply, or the error
	SettingsMAC string
	SentBy      *int64
	CreatedAt   time.Time
}

func (s *Store) InsertEmailTest(ctx context.Context, t *EmailTest) error {
	t.CreatedAt = now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO email_tests
		(recipient, ok, detail, settings_mac, sent_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		t.Recipient, t.OK, t.Detail, t.SettingsMAC, t.SentBy, t.CreatedAt)
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	return err
}

// LastEmailTest returns the newest test, or ErrNotFound when there is none.
// okOnly restricts it to tests that were delivered.
func (s *Store) LastEmailTest(ctx context.Context, okOnly bool) (*EmailTest, error) {
	q := `SELECT id, recipient, ok, detail, settings_mac, sent_by, created_at FROM email_tests`
	if okOnly {
		q += ` WHERE ok = 1`
	}
	var t EmailTest
	err := s.db.QueryRowContext(ctx, q+` ORDER BY id DESC LIMIT 1`).
		Scan(&t.ID, &t.Recipient, &t.OK, &t.Detail, &t.SettingsMAC, &t.SentBy, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}
