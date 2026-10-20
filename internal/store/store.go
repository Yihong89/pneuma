// Package store persists messages and uploaded files.
//
// Message metadata lives in SQLite; file contents live on disk under
// <dir>/files/<id>, written in a streaming fashion so uploads never have to
// fit in memory.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a message or file does not exist.
var ErrNotFound = errors.New("not found")

// ErrTooLarge is returned when an upload exceeds the configured limit.
var ErrTooLarge = errors.New("file too large")

// Kind is the type of a message.
type Kind string

const (
	KindText Kind = "text"
	KindFile Kind = "file"
	// KindExpired is a file message whose file was removed by retention;
	// Body keeps the file's name.
	KindExpired Kind = "expired"
)

// Sender identifies the device that created a message.
type Sender struct {
	// ID is the device's stable tailnet node ID. Unlike Device (its
	// MagicDNS name, which a different machine could take over later) it
	// never changes, so it is what chat membership is based on. It is not
	// stored with messages.
	ID     string `json:"id,omitempty"`
	Device string `json:"device"`
	OS     string `json:"os"`
	User   string `json:"user"`
}

// File is the metadata of an uploaded file.
type File struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Mime   string `json:"mime"`
	SHA256 string `json:"sha256"`
}

// Chat is a conversation thread. Every message belongs to one.
type Chat struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	// LastAt is the time of the newest message, or CreatedAt if empty.
	LastAt time.Time `json:"last_at"`
	Count  int64     `json:"count"`
	// Members restricts the chat to these devices, by stable node ID.
	// Empty means every device on the tailnet can see it.
	Members []string `json:"members,omitempty"`
}

// Visible reports whether the device with this stable ID may see the chat.
func (c *Chat) Visible(deviceID string) bool {
	return len(c.Members) == 0 || (deviceID != "" && slices.Contains(c.Members, deviceID))
}

func encodeMembers(m []string) string { return strings.Join(m, "\n") }

func decodeMembers(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// Message is a single chat entry: either a piece of text or a file.
type Message struct {
	ID        int64     `json:"id"`
	ChatID    int64     `json:"chat_id"`
	CreatedAt time.Time `json:"created_at"`
	Sender
	Kind Kind   `json:"kind"`
	Body string `json:"body,omitempty"`
	File *File  `json:"file,omitempty"`
}

// Store is safe for concurrent use.
type Store struct {
	db       *sql.DB
	filesDir string
}

const schema = `
CREATE TABLE IF NOT EXISTS chats (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	members    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS files (
	id     TEXT PRIMARY KEY,
	name   TEXT NOT NULL,
	size   INTEGER NOT NULL,
	mime   TEXT NOT NULL,
	sha256 TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at INTEGER NOT NULL,
	device     TEXT NOT NULL,
	device_os  TEXT NOT NULL,
	user       TEXT NOT NULL,
	kind       TEXT NOT NULL,
	body       TEXT NOT NULL DEFAULT '',
	file_id    TEXT REFERENCES files(id),
	chat_id    INTEGER NOT NULL DEFAULT 1 REFERENCES chats(id)
);
`

// migrate brings databases created by older versions up to date.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	// v0.1 had no chats: everything lands in chat 1.
	var hasChat int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name = 'chat_id'`).Scan(&hasChat); err != nil {
		return err
	}
	if hasChat == 0 {
		// SQLite can't add a REFERENCES column with a non-NULL default.
		if _, err := db.Exec(`ALTER TABLE messages ADD COLUMN chat_id INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	var hasMembers int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('chats') WHERE name = 'members'`).Scan(&hasMembers); err != nil {
		return err
	}
	if hasMembers == 0 {
		if _, err := db.Exec(`ALTER TABLE chats ADD COLUMN members TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS messages_chat ON messages(chat_id, id)`); err != nil {
		return err
	}
	if _, err := db.Exec(settingsSchema); err != nil {
		return err
	}
	return ensureChat(db)
}

// ensureChat makes sure there is always at least one chat to write to.
func ensureChat(db *sql.DB) error {
	_, err := db.Exec(`INSERT INTO chats (id, name, created_at)
		SELECT COALESCE((SELECT MIN(chat_id) FROM messages), 1), 'general', ?
		WHERE NOT EXISTS (SELECT 1 FROM chats)`, time.Now().UnixMilli())
	return err
}

// Open opens (creating if needed) the store rooted at dir.
func Open(dir string) (*Store, error) {
	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.Join(dir, "pneuma.db") +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows a single writer; serialising through one connection
	// avoids SQLITE_BUSY under concurrent uploads.
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db, filesDir: filesDir}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// AddText stores a text message.
func (s *Store) AddText(ctx context.Context, chat int64, from Sender, body string) (*Message, error) {
	if err := s.chatExists(ctx, chat); err != nil {
		return nil, err
	}
	return s.insert(ctx, chat, from, KindText, body, nil)
}

// AddFile streams r to disk and stores a file message. If more than max bytes
// are read the partial file is removed and ErrTooLarge is returned.
func (s *Store) AddFile(ctx context.Context, chat int64, from Sender, name, mime string, r io.Reader, max int64) (*Message, error) {
	if err := s.chatExists(ctx, chat); err != nil {
		return nil, err
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(s.filesDir, id)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, max+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > max {
		err = ErrTooLarge
	}
	if err != nil {
		os.Remove(path)
		return nil, err
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	file := &File{ID: id, Name: name, Size: n, Mime: mime, SHA256: hex.EncodeToString(h.Sum(nil))}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO files (id, name, size, mime, sha256) VALUES (?, ?, ?, ?, ?)`,
		file.ID, file.Name, file.Size, file.Mime, file.SHA256); err != nil {
		os.Remove(path)
		return nil, err
	}
	m, err := s.insert(ctx, chat, from, KindFile, "", file)
	if err != nil {
		s.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id)
		os.Remove(path)
		return nil, err
	}
	return m, nil
}

func (s *Store) insert(ctx context.Context, chat int64, from Sender, kind Kind, body string, file *File) (*Message, error) {
	m := &Message{ChatID: chat, CreatedAt: time.Now().UTC().Truncate(time.Millisecond), Sender: from, Kind: kind, Body: body, File: file}
	var fileID any
	if file != nil {
		fileID = file.ID
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (chat_id, created_at, device, device_os, user, kind, body, file_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		chat, m.CreatedAt.UnixMilli(), from.Device, from.OS, from.User, string(kind), body, fileID)
	if err != nil {
		return nil, err
	}
	if m.ID, err = res.LastInsertId(); err != nil {
		return nil, err
	}
	return m, nil
}

const selectMessages = `
SELECT m.id, m.chat_id, m.created_at, m.device, m.device_os, m.user, m.kind, m.body,
       f.id, f.name, f.size, f.mime, f.sha256
FROM messages m LEFT JOIN files f ON f.id = m.file_id`

// List returns up to limit messages of a chat with id < before (or the
// newest when before <= 0), in ascending id order.
func (s *Store) List(ctx context.Context, chat, before int64, limit int) ([]*Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := selectMessages + ` WHERE m.chat_id = ? AND (? <= 0 OR m.id < ?) ORDER BY m.id DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, chat, before, before, limit)
	if err != nil {
		return nil, err
	}
	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// Files returns the file messages of a chat, newest first.
func (s *Store) Files(ctx context.Context, chat int64) ([]*Message, error) {
	rows, err := s.db.QueryContext(ctx,
		selectMessages+` WHERE m.chat_id = ? AND m.kind = 'file' ORDER BY m.id DESC LIMIT 5000`, chat)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func scanMessages(rows *sql.Rows) ([]*Message, error) {
	defer rows.Close()
	var msgs []*Message
	for rows.Next() {
		var (
			m       Message
			created int64
			kind    string
			fid     sql.NullString
			fname   sql.NullString
			fsize   sql.NullInt64
			fmime   sql.NullString
			fsum    sql.NullString
		)
		if err := rows.Scan(&m.ID, &m.ChatID, &created, &m.Device, &m.OS, &m.User, &kind, &m.Body,
			&fid, &fname, &fsize, &fmime, &fsum); err != nil {
			return nil, err
		}
		m.CreatedAt = time.UnixMilli(created).UTC()
		m.Kind = Kind(kind)
		if fid.Valid {
			m.File = &File{ID: fid.String, Name: fname.String, Size: fsize.Int64, Mime: fmime.String, SHA256: fsum.String}
		}
		msgs = append(msgs, &m)
	}
	return msgs, rows.Err()
}

// Get returns a single message.
func (s *Store) Get(ctx context.Context, id int64) (*Message, error) {
	rows, err := s.db.QueryContext(ctx, selectMessages+` WHERE m.id = ?`, id)
	if err != nil {
		return nil, err
	}
	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, ErrNotFound
	}
	return msgs[0], nil
}

// Delete removes a message and, if it carried a file, the file itself.
func (s *Store) Delete(ctx context.Context, id int64) error {
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, id); err != nil {
		return err
	}
	if m.File != nil {
		return s.removeFiles(ctx, []string{m.File.ID})
	}
	return nil
}

func (s *Store) removeFiles(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(s.filesDir, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// ---- chats ----

const selectChats = `
SELECT c.id, c.name, c.created_at, COALESCE(MAX(m.created_at), c.created_at), COUNT(m.id), c.members
FROM chats c LEFT JOIN messages m ON m.chat_id = c.id`

// Chats returns all chats, most recently active first.
func (s *Store) Chats(ctx context.Context) ([]*Chat, error) {
	rows, err := s.db.QueryContext(ctx, selectChats+` GROUP BY c.id ORDER BY 4 DESC, c.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chats []*Chat
	for rows.Next() {
		var c Chat
		var created, last int64
		var members string
		if err := rows.Scan(&c.ID, &c.Name, &created, &last, &c.Count, &members); err != nil {
			return nil, err
		}
		c.CreatedAt, c.LastAt = time.UnixMilli(created).UTC(), time.UnixMilli(last).UTC()
		c.Members = decodeMembers(members)
		chats = append(chats, &c)
	}
	return chats, rows.Err()
}

// Chat returns a single chat.
func (s *Store) Chat(ctx context.Context, id int64) (*Chat, error) {
	var c Chat
	var created, last int64
	var members string
	err := s.db.QueryRowContext(ctx, selectChats+` WHERE c.id = ? GROUP BY c.id`, id).
		Scan(&c.ID, &c.Name, &created, &last, &c.Count, &members)
	c.Members = decodeMembers(members)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt, c.LastAt = time.UnixMilli(created).UTC(), time.UnixMilli(last).UTC()
	return &c, nil
}

// CreateChat adds a new, empty chat. With members, only those devices can
// see it.
func (s *Store) CreateChat(ctx context.Context, name string, members ...string) (*Chat, error) {
	members = normalizeMembers(members)
	res, err := s.db.ExecContext(ctx, `INSERT INTO chats (name, created_at, members) VALUES (?, ?, ?)`,
		name, time.Now().UnixMilli(), encodeMembers(members))
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.Chat(ctx, id)
}

// AddMember gives a device (by stable ID) access to a private chat.
func (s *Store) AddMember(ctx context.Context, id int64, device string) (*Chat, error) {
	c, err := s.Chat(ctx, id)
	if err != nil {
		return nil, err
	}
	members := normalizeMembers(append(c.Members, device))
	if _, err := s.db.ExecContext(ctx, `UPDATE chats SET members = ? WHERE id = ?`, encodeMembers(members), id); err != nil {
		return nil, err
	}
	return s.Chat(ctx, id)
}

func normalizeMembers(m []string) []string {
	var out []string
	for _, d := range m {
		if d = strings.TrimSpace(d); d != "" && !strings.Contains(d, "\n") && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	slices.Sort(out)
	return out
}

// FileChat returns the chat a stored file belongs to.
func (s *Store) FileChat(ctx context.Context, fileID string) (int64, error) {
	var chat int64
	err := s.db.QueryRowContext(ctx, `SELECT chat_id FROM messages WHERE file_id = ?`, fileID).Scan(&chat)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return chat, err
}

// RenameChat changes a chat's name.
func (s *Store) RenameChat(ctx context.Context, id int64, name string) (*Chat, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE chats SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.Chat(ctx, id)
}

// DeleteChat removes a chat with all its messages and files. If no chat
// open to every device is left, a fresh "general" is created so there is
// always somewhere to write; it is returned (nil otherwise).
func (s *Store) DeleteChat(ctx context.Context, id int64) (*Chat, error) {
	if err := s.chatExists(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT file_id FROM messages WHERE chat_id = ? AND file_id IS NOT NULL`, id)
	if err != nil {
		return nil, err
	}
	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return nil, err
		}
		files = append(files, f)
	}
	rows.Close()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE chat_id = ?`, id); err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM chats WHERE id = ?`, id); err != nil {
		return nil, err
	}
	if err := s.removeFiles(ctx, files); err != nil {
		return nil, err
	}
	// Keep at least one chat every device can see.
	var left int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chats WHERE members = ''`).Scan(&left); err != nil {
		return nil, err
	}
	if left == 0 {
		return s.CreateChat(ctx, "general")
	}
	return nil, nil
}

func (s *Store) chatExists(ctx context.Context, id int64) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chats WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// OpenFile returns the metadata and an open handle for a stored file.
func (s *Store) OpenFile(ctx context.Context, id string) (*File, *os.File, error) {
	var f File
	err := s.db.QueryRowContext(ctx, `SELECT id, name, size, mime, sha256 FROM files WHERE id = ?`, id).
		Scan(&f.ID, &f.Name, &f.Size, &f.Mime, &f.SHA256)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	fh, err := os.Open(filepath.Join(s.filesDir, f.ID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	return &f, fh, nil
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
