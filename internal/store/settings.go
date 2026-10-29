package store

import (
	"context"
	"database/sql"
	"errors"
)

// SettingRuntimeDefault is the key of a runtime's default version spec.
func SettingRuntimeDefault(runtime string) string { return "runtime." + runtime + ".default" }

// SettingRuntimeEnabled is the key of a runtime's on/off switch ("false" turns it off).
func SettingRuntimeEnabled(runtime string) string { return "runtime." + runtime + ".enabled" }

// Setting returns a server-wide setting, or "" when it was never set.
func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetSetting stores a setting; an empty value removes it.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	if value == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now())
	return err
}
