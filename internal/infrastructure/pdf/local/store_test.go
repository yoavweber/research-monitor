package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// stubFetcher is a no-op shared.Fetcher used only to satisfy NewStore's
// signature in tests that do not exercise fetch behaviour. Task 3.3 will
// add tests that actually drive the fetcher.
type stubFetcher struct{}

func (stubFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	return nil, errors.New("stub fetcher should not be called in store constructor tests")
}

// stubLogger is a no-op shared.Logger used only to satisfy NewStore's
// signature; the constructor under test does not log.
type stubLogger struct{}

func (stubLogger) InfoContext(ctx context.Context, msg string, args ...any)  {}
func (stubLogger) WarnContext(ctx context.Context, msg string, args ...any)  {}
func (stubLogger) ErrorContext(ctx context.Context, msg string, args ...any) {}
func (stubLogger) DebugContext(ctx context.Context, msg string, args ...any) {}
func (stubLogger) With(args ...any) shared.Logger                            { return stubLogger{} }

// compile-time conformance — surfaces port drift at build time.
var (
	_ shared.Fetcher = stubFetcher{}
	_ shared.Logger  = stubLogger{}
)

func TestNewStore(t *testing.T) {
	t.Parallel()

	t.Run("creates root directory if missing", func(t *testing.T) {
		t.Parallel()

		parent := t.TempDir()
		root := filepath.Join(parent, "pdfs")

		store, err := NewStore(root, stubFetcher{}, stubLogger{})

		if err != nil {
			t.Fatalf("NewStore: unexpected error: %v", err)
		}
		if store == nil {
			t.Fatalf("NewStore returned nil store")
		}
		info, statErr := os.Stat(root)
		if statErr != nil {
			t.Fatalf("expected root to exist, stat err: %v", statErr)
		}
		if !info.IsDir() {
			t.Fatalf("expected root to be a directory")
		}
	})

	t.Run("accepts existing writable directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()

		store, err := NewStore(root, stubFetcher{}, stubLogger{})

		if err != nil {
			t.Fatalf("NewStore: unexpected error for existing dir: %v", err)
		}
		if store == nil {
			t.Fatalf("NewStore returned nil store")
		}
	})

	t.Run("rejects path that is a regular file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		filePath := filepath.Join(dir, "not-a-dir")
		if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}

		_, err := NewStore(filePath, stubFetcher{}, stubLogger{})

		if err == nil {
			t.Fatalf("NewStore: expected error for regular-file root, got nil")
		}
		if !errors.Is(err, pdf.ErrStore) {
			t.Fatalf("error must wrap pdf.ErrStore, got %v", err)
		}
		if !strings.Contains(err.Error(), filePath) {
			t.Fatalf("error must mention offending root %q, got %v", filePath, err)
		}
	})

	t.Run("rejects path with non-writable parent when MkdirAll cannot succeed", func(t *testing.T) {
		t.Parallel()

		if os.Geteuid() == 0 {
			t.Skip("running as root, perms ignored")
		}

		parent := t.TempDir()
		if err := os.Chmod(parent, 0o555); err != nil {
			t.Fatalf("chmod parent: %v", err)
		}
		t.Cleanup(func() {
			// Restore perms so t.TempDir cleanup can remove the directory.
			_ = os.Chmod(parent, 0o755)
		})
		root := filepath.Join(parent, "pdfs")

		_, err := NewStore(root, stubFetcher{}, stubLogger{})

		if err == nil {
			t.Fatalf("NewStore: expected error for non-writable parent, got nil")
		}
		if !errors.Is(err, pdf.ErrStore) {
			t.Fatalf("error must wrap pdf.ErrStore, got %v", err)
		}
		if !strings.Contains(err.Error(), root) {
			t.Fatalf("error must mention offending root %q, got %v", root, err)
		}
	})
}

func TestStoreCanonicalPath(t *testing.T) {
	t.Parallel()

	t.Run("paper key resolves under <root>/paper/<id>.pdf", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		s := newTestStore(t, root)
		key := pdf.Key{SourceType: "paper", SourceID: "2404.12345v1", URL: "https://example.invalid/p.pdf"}

		got := s.canonicalPath(key)

		want := filepath.Join(root, "paper", "2404.12345v1.pdf")
		if got != want {
			t.Fatalf("canonicalPath = %q, want %q", got, want)
		}
	})

	t.Run("post key resolves under <root>/post/<id>.pdf", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		s := newTestStore(t, root)
		key := pdf.Key{SourceType: "post", SourceID: "abc-123", URL: "https://example.invalid/x.pdf"}

		got := s.canonicalPath(key)

		want := filepath.Join(root, "post", "abc-123.pdf")
		if got != want {
			t.Fatalf("canonicalPath = %q, want %q", got, want)
		}
	})

	t.Run("canonical path stays under root", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		s := newTestStore(t, root)
		key := pdf.Key{SourceType: "paper", SourceID: "id1", URL: "https://example.invalid/x.pdf"}

		got := s.canonicalPath(key)

		rel, err := filepath.Rel(root, got)
		if err != nil {
			t.Fatalf("filepath.Rel: %v", err)
		}
		if strings.HasPrefix(rel, "..") {
			t.Fatalf("canonical path %q escapes root %q (rel=%q)", got, root, rel)
		}
	})
}

// newTestStore constructs a *localStore under root, failing the test on
// constructor error. Returns the concrete type so the canonical-path
// helper (unexported) is reachable from tests.
func newTestStore(t *testing.T, root string) *localStore {
	t.Helper()
	store, err := NewStore(root, stubFetcher{}, stubLogger{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	s, ok := store.(*localStore)
	if !ok {
		t.Fatalf("NewStore returned %T, want *localStore", store)
	}
	return s
}
