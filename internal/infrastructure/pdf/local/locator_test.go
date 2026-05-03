package local

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

func TestLocalLocator(t *testing.T) {
	t.Parallel()

	t.Run("path returns the configured path", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "doc.pdf")
		if err := os.WriteFile(path, []byte("ignored"), 0o644); err != nil {
			t.Fatalf("seed write: %v", err)
		}

		loc := newLocator(path)

		if got := loc.Path(); got != path {
			t.Fatalf("Path() = %q, want %q", got, path)
		}
	})

	t.Run("open returns the file bytes", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "doc.pdf")
		want := []byte("hello world")
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatalf("seed write: %v", err)
		}

		loc := newLocator(path)
		r, err := loc.Open(context.Background())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer r.Close()
		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		if string(got) != string(want) {
			t.Fatalf("bytes = %q, want %q", got, want)
		}
	})

	t.Run("path and open agree byte for byte", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "doc.pdf")
		payload := make([]byte, 1024)
		if _, err := rand.Read(payload); err != nil {
			t.Fatalf("rand: %v", err)
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("seed write: %v", err)
		}

		loc := newLocator(path)
		viaPath, err := os.ReadFile(loc.Path())
		if err != nil {
			t.Fatalf("ReadFile via Path: %v", err)
		}
		r, err := loc.Open(context.Background())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer r.Close()
		viaOpen, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		if string(viaPath) != string(viaOpen) {
			t.Fatalf("Path() bytes != Open() bytes (lens %d vs %d)", len(viaPath), len(viaOpen))
		}
	})

	t.Run("open wraps ErrStore when file is missing", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		missing := filepath.Join(dir, "does-not-exist.pdf")

		loc := newLocator(missing)
		r, err := loc.Open(context.Background())

		if err == nil {
			r.Close()
			t.Fatalf("Open on missing file: err = nil, want error")
		}
		if r != nil {
			t.Fatalf("Open on missing file: reader = %v, want nil", r)
		}
		if !errors.Is(err, pdf.ErrStore) {
			t.Fatalf("Open error = %v, want errors.Is(err, pdf.ErrStore)", err)
		}
	})
}
