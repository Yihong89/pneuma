package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

const settingsSchema = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS aliases (
	device TEXT PRIMARY KEY,
	alias  TEXT NOT NULL
);
`

// Setting returns a stored setting, or "" if unset.
func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SettingInt returns a numeric setting, or fallback if unset or invalid.
func (s *Store) SettingInt(ctx context.Context, key string, fallback int64) (int64, error) {
	v, err := s.Setting(ctx, key)
	if err != nil || v == "" {
		return fallback, err
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback, nil
	}
	return n, nil
}

// SetSetting stores a setting.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// Aliases returns the custom display names, keyed by tailnet device name.
func (s *Store) Aliases(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device, alias FROM aliases`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var d, a string
		if err := rows.Scan(&d, &a); err != nil {
			return nil, err
		}
		out[d] = a
	}
	return out, rows.Err()
}

// SetAlias sets a device's display name; an empty alias removes it.
func (s *Store) SetAlias(ctx context.Context, device, alias string) error {
	if alias == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM aliases WHERE device = ?`, device)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO aliases (device, alias) VALUES (?, ?) ON CONFLICT(device) DO UPDATE SET alias = excluded.alias`, device, alias)
	return err
}

// Usage is how much the store holds.
type Usage struct {
	Messages int64 `json:"messages"`
	Files    int64 `json:"files"`
	Bytes    int64 `json:"bytes"`
}

// Usage reports message and file totals.
func (s *Store) Usage(ctx context.Context) (Usage, error) {
	var u Usage
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&u.Messages); err != nil {
		return u, err
	}
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(size), 0) FROM files`).Scan(&u.Files, &u.Bytes)
	return u, err
}

// DeleteOlderThan removes every message (and its file) created before t.
// It returns how many messages were removed.
func (s *Store) DeleteOlderThan(ctx context.Context, t time.Time) (int64, error) {
	cutoff := t.UnixMilli()
	rows, err := s.db.QueryContext(ctx, `SELECT file_id FROM messages WHERE created_at < ? AND file_id IS NOT NULL`, cutoff)
	if err != nil {
		return 0, err
	}
	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return 0, err
		}
		files = append(files, f)
	}
	rows.Close()
	res, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	if err := s.removeFiles(ctx, files); err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
