package store

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		if _, err := s.AddText(ctx, laptop, strings.Repeat("x", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := s.List(ctx, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 3 || latest[0].ID != 3 || latest[2].ID != 5 {
		t.Fatalf("latest page = %v", ids(latest))
	}
	older, err := s.List(ctx, latest[0].ID, 3)
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
	m, err := s.AddFile(ctx, laptop, "notes.txt", "text/plain", strings.NewReader("hello"), 100)
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
	_, err := s.AddFile(context.Background(), laptop, "big.bin", "", strings.NewReader("0123456789"), 5)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "files"))
	if len(entries) != 0 {
		t.Fatalf("partial upload left behind: %v", entries)
	}
	if msgs, _ := s.List(context.Background(), 0, 10); len(msgs) != 0 {
		t.Fatalf("message created for rejected upload")
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddText(context.Background(), laptop, "still here")
	s.Close()

	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	msgs, _ := s.List(context.Background(), 0, 10)
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
