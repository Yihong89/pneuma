package store

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var laptop = Sender{Device: "laptop", OS: "linux", User: "me@example.com"}

func open(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func TestTextAndPagination(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	for i := range 5 {
		if _, err := s.AddText(ctx, 1, laptop, strings.Repeat("x", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := s.List(ctx, 1, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 3 || latest[0].ID != 3 || latest[2].ID != 5 {
		t.Fatalf("latest page = %v", ids(latest))
	}
	older, err := s.List(ctx, 1, latest[0].ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(older); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("older page = %v", got)
	}
	if older[0].Device != "laptop" || older[0].Kind != KindText || older[0].Body != "x" {
		t.Fatalf("unexpected message %+v", older[0])
	}
}

func TestFileLifecycle(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()
	m, err := s.AddFile(ctx, 1, laptop, "notes.txt", "text/plain", strings.NewReader("hello"), 100)
	if err != nil {
		t.Fatal(err)
	}
	if m.File == nil || m.File.Size != 5 || m.File.SHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("file = %+v", m.File)
	}

	got, err := s.Get(ctx, m.ID)
	if err != nil || got.File == nil || got.File.Name != "notes.txt" {
		t.Fatalf("get = %+v, %v", got, err)
	}

	meta, fh, err := s.OpenFile(ctx, m.File.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(fh)
	fh.Close()
	if string(body) != "hello" || meta.Mime != "text/plain" {
		t.Fatalf("content = %q, meta = %+v", body, meta)
	}

	if err := s.Delete(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "files", m.File.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still on disk: %v", err)
	}
	if _, _, err := s.OpenFile(ctx, m.File.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenFile after delete = %v", err)
	}
	if err := s.Delete(ctx, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}

func TestFileTooLarge(t *testing.T) {
	s, dir := open(t)
	_, err := s.AddFile(context.Background(), 1, laptop, "big.bin", "", strings.NewReader("0123456789"), 5)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "files"))
	if len(entries) != 0 {
		t.Fatalf("partial upload left behind: %v", entries)
	}
	if msgs, _ := s.List(context.Background(), 1, 0, 10); len(msgs) != 0 {
		t.Fatalf("message created for rejected upload")
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddText(context.Background(), 1, laptop, "still here")
	s.Close()

	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	msgs, _ := s.List(context.Background(), 1, 0, 10)
	if len(msgs) != 1 || msgs[0].Body != "still here" {
		t.Fatalf("after reopen: %+v", msgs)
	}
}

func ids(msgs []*Message) []int64 {
	var out []int64
	for _, m := range msgs {
		out = append(out, m.ID)
	}
	return out
}

func TestChats(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()

	chats, err := s.Chats(ctx)
	if err != nil || len(chats) != 1 || chats[0].Name != "general" {
		t.Fatalf("default chats = %+v, %v", chats, err)
	}
	general := chats[0].ID

	work, err := s.CreateChat(ctx, "work")
	if err != nil {
		t.Fatal(err)
	}
	s.AddText(ctx, work.ID, laptop, "in work")
	m, _ := s.AddFile(ctx, work.ID, laptop, "a.txt", "text/plain", strings.NewReader("a"), 10)
	time.Sleep(5 * time.Millisecond) // make "general" strictly the most recent
	s.AddText(ctx, general, laptop, "in general")

	if msgs, _ := s.List(ctx, work.ID, 0, 10); len(msgs) != 2 {
		t.Fatalf("work has %d messages", len(msgs))
	}
	if _, err := s.AddText(ctx, 999, laptop, "nowhere"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("write to missing chat = %v", err)
	}

	chats, _ = s.Chats(ctx)
	if chats[0].ID != general || chats[1].Count != 2 {
		t.Fatalf("order/counts = %+v %+v", chats[0], chats[1])
	}

	if c, err := s.RenameChat(ctx, work.ID, "trabajo"); err != nil || c.Name != "trabajo" {
		t.Fatalf("rename = %+v, %v", c, err)
	}

	if fresh, err := s.DeleteChat(ctx, work.ID); err != nil || fresh != nil {
		t.Fatalf("delete = %+v, %v", fresh, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "files", m.File.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("chat file left on disk")
	}

	fresh, err := s.DeleteChat(ctx, general)
	if err != nil || fresh == nil || fresh.Name != "general" {
		t.Fatalf("deleting last chat = %+v, %v", fresh, err)
	}
}

func TestMigrateFromV01(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "pneuma.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE files (id TEXT PRIMARY KEY, name TEXT NOT NULL, size INTEGER NOT NULL, mime TEXT NOT NULL, sha256 TEXT NOT NULL)`,
		`CREATE TABLE messages (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at INTEGER NOT NULL, device TEXT NOT NULL, device_os TEXT NOT NULL, user TEXT NOT NULL, kind TEXT NOT NULL, body TEXT NOT NULL DEFAULT '', file_id TEXT REFERENCES files(id))`,
		`INSERT INTO messages (created_at, device, device_os, user, kind, body) VALUES (1, 'old', 'linux', 'me', 'text', 'from v0.1')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	chats, _ := s.Chats(context.Background())
	if len(chats) != 1 {
		t.Fatalf("chats = %+v", chats)
	}
	msgs, _ := s.List(context.Background(), chats[0].ID, 0, 10)
	if len(msgs) != 1 || msgs[0].Body != "from v0.1" {
		t.Fatalf("migrated messages = %+v", msgs)
	}
}

func TestSettingsAliasesRetention(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()

	if n, _ := s.SettingInt(ctx, "retention_days", 7); n != 7 {
		t.Fatalf("fallback = %d", n)
	}
	s.SetSetting(ctx, "retention_days", "30")
	s.SetSetting(ctx, "retention_days", "90")
	if n, _ := s.SettingInt(ctx, "retention_days", 7); n != 90 {
		t.Fatalf("setting = %d", n)
	}

	s.SetAlias(ctx, "nothing-phone-3a", "móvil")
	if a, _ := s.Aliases(ctx); a["nothing-phone-3a"] != "móvil" {
		t.Fatalf("aliases = %v", a)
	}
	s.SetAlias(ctx, "nothing-phone-3a", "")
	if a, _ := s.Aliases(ctx); len(a) != 0 {
		t.Fatalf("alias not removed: %v", a)
	}

	old, _ := s.AddFile(ctx, 1, laptop, "old.txt", "text/plain", strings.NewReader("old"), 10)
	time.Sleep(5 * time.Millisecond)
	cut := time.Now()
	time.Sleep(5 * time.Millisecond)
	s.AddText(ctx, 1, laptop, "new")
	n, err := s.DeleteOlderThan(ctx, cut)
	if err != nil || n != 1 {
		t.Fatalf("deleted %d, %v", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "files", old.File.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old file still on disk")
	}
	u, _ := s.Usage(ctx)
	if u.Messages != 1 || u.Files != 0 {
		t.Fatalf("usage = %+v", u)
	}
}
