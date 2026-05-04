package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
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

func TestStoreEnsure(t *testing.T) {
	t.Parallel()

	t.Run("validates key before any I/O and never invokes the fetcher", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		fetcher := &mocks.Fetcher{Body: []byte("never returned")}
		store := newTestStoreWith(t, root, fetcher)
		// Empty SourceID — Key.Validate must reject before any fetch or write.
		bad := pdf.Key{SourceType: "paper", SourceID: "", URL: "https://example.invalid/p.pdf"}

		_, err := store.Ensure(context.Background(), bad)

		if err == nil {
			t.Fatalf("Ensure with invalid key: err = nil, want error")
		}
		if !errors.Is(err, pdf.ErrInvalidKey) {
			t.Fatalf("Ensure error = %v, want errors.Is(err, pdf.ErrInvalidKey)", err)
		}
		if fetcher.Invocations != 0 {
			t.Fatalf("fetcher invoked on invalid key: count = %d, want 0", fetcher.Invocations)
		}
		assertNoFiles(t, root)
	})

	t.Run("cache miss writes atomically and returns locator", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		body := bytesFilled(1024, 0xab)
		fetcher := &mocks.Fetcher{Body: body}
		store := newTestStoreWith(t, root, fetcher)
		key := newTestKey()

		loc, err := store.Ensure(context.Background(), key)

		if err != nil {
			t.Fatalf("Ensure: unexpected error: %v", err)
		}
		want := filepath.Join(root, key.SourceType, key.SourceID+".pdf")
		if loc.Path() != want {
			t.Fatalf("locator path = %q, want %q", loc.Path(), want)
		}
		got, readErr := os.ReadFile(loc.Path())
		if readErr != nil {
			t.Fatalf("read canonical: %v", readErr)
		}
		if string(got) != string(body) {
			t.Fatalf("canonical bytes mismatch: len(got)=%d len(want)=%d", len(got), len(body))
		}
		if fetcher.Invocations != 1 {
			t.Fatalf("fetcher invocations = %d, want 1", fetcher.Invocations)
		}
		assertNoTempSiblings(t, filepath.Dir(loc.Path()))
	})

	t.Run("cache hit returns without invoking the fetcher", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		key := newTestKey()
		canonical := filepath.Join(root, key.SourceType, key.SourceID+".pdf")
		seed := []byte("pre-existing bytes")
		seedFile(t, canonical, seed)
		fetcher := &mocks.Fetcher{Body: []byte("would replace if called")}
		store := newTestStoreWith(t, root, fetcher)

		loc, err := store.Ensure(context.Background(), key)

		if err != nil {
			t.Fatalf("Ensure: unexpected error: %v", err)
		}
		if fetcher.Invocations != 0 {
			t.Fatalf("fetcher invoked on cache hit: count = %d, want 0", fetcher.Invocations)
		}
		got, readErr := os.ReadFile(loc.Path())
		if readErr != nil {
			t.Fatalf("read canonical: %v", readErr)
		}
		if string(got) != string(seed) {
			t.Fatalf("cache hit changed bytes: got %q, want %q", got, seed)
		}
	})

	t.Run("zero-byte canonical file is replaced via fetch", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		key := newTestKey()
		canonical := filepath.Join(root, key.SourceType, key.SourceID+".pdf")
		seedFile(t, canonical, nil) // zero-byte sentinel
		body := []byte("fresh bytes from upstream")
		fetcher := &mocks.Fetcher{Body: body}
		store := newTestStoreWith(t, root, fetcher)

		loc, err := store.Ensure(context.Background(), key)

		if err != nil {
			t.Fatalf("Ensure: unexpected error: %v", err)
		}
		if fetcher.Invocations != 1 {
			t.Fatalf("fetcher invocations = %d, want 1", fetcher.Invocations)
		}
		got, readErr := os.ReadFile(loc.Path())
		if readErr != nil {
			t.Fatalf("read canonical: %v", readErr)
		}
		if string(got) != string(body) {
			t.Fatalf("canonical bytes after replace = %q, want %q", got, body)
		}
		assertNoTempSiblings(t, filepath.Dir(loc.Path()))
	})

	t.Run("fetcher status-code error surfaces ErrFetch and shared.ErrBadStatus", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		fetcher := &mocks.Fetcher{Error: fmt.Errorf("%w: status=%d", shared.ErrBadStatus, 404)}
		store := newTestStoreWith(t, root, fetcher)
		key := newTestKey()

		_, err := store.Ensure(context.Background(), key)

		if err == nil {
			t.Fatalf("Ensure with status-code error: err = nil, want error")
		}
		if !errors.Is(err, pdf.ErrFetch) {
			t.Fatalf("error must wrap pdf.ErrFetch, got %v", err)
		}
		if !errors.Is(err, shared.ErrBadStatus) {
			t.Fatalf("error must preserve shared.ErrBadStatus, got %v", err)
		}
		canonical := filepath.Join(root, key.SourceType, key.SourceID+".pdf")
		if _, statErr := os.Stat(canonical); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("canonical file should not exist after fetch failure, stat err = %v", statErr)
		}
		assertNoTempSiblings(t, filepath.Join(root, key.SourceType))
	})

	t.Run("empty fetcher body surfaces ErrFetch and writes no file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		fetcher := &mocks.Fetcher{Body: []byte{}}
		store := newTestStoreWith(t, root, fetcher)
		key := newTestKey()

		_, err := store.Ensure(context.Background(), key)

		if err == nil {
			t.Fatalf("Ensure with empty body: err = nil, want error")
		}
		if !errors.Is(err, pdf.ErrFetch) {
			t.Fatalf("error must wrap pdf.ErrFetch, got %v", err)
		}
		canonical := filepath.Join(root, key.SourceType, key.SourceID+".pdf")
		if _, statErr := os.Stat(canonical); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("canonical file should not exist after empty-body failure, stat err = %v", statErr)
		}
		assertNoTempSiblings(t, filepath.Join(root, key.SourceType))
	})

	t.Run("context cancellation mid-fetch surfaces context.Canceled chained under ErrFetch", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		// Sleep long enough that the AfterFunc cancellation deterministically
		// wins; the mock honours ctx.Done() so the wait collapses immediately.
		fetcher := &mocks.Fetcher{Body: []byte("never delivered"), Sleep: 5 * time.Second}
		store := newTestStoreWith(t, root, fetcher)
		key := newTestKey()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		time.AfterFunc(20*time.Millisecond, cancel)

		_, err := store.Ensure(ctx, key)

		if err == nil {
			t.Fatalf("Ensure with cancelled ctx: err = nil, want error")
		}
		if !errors.Is(err, pdf.ErrFetch) {
			t.Fatalf("error must wrap pdf.ErrFetch, got %v", err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error must preserve context.Canceled, got %v", err)
		}
		canonical := filepath.Join(root, key.SourceType, key.SourceID+".pdf")
		if _, statErr := os.Stat(canonical); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("canonical file must not exist after cancellation, stat err = %v", statErr)
		}
		assertNoTempSiblings(t, filepath.Join(root, key.SourceType))
	})

	t.Run("successful Ensure preserves prior unrelated files (keep-forever)", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		other := filepath.Join(root, "paper", "other.pdf")
		otherBytes := []byte("untouched neighbour")
		seedFile(t, other, otherBytes)
		fetcher := &mocks.Fetcher{Body: []byte("new bytes")}
		store := newTestStoreWith(t, root, fetcher)
		key := newTestKey() // different SourceID than "other"

		_, err := store.Ensure(context.Background(), key)

		if err != nil {
			t.Fatalf("Ensure: unexpected error: %v", err)
		}
		got, readErr := os.ReadFile(other)
		if readErr != nil {
			t.Fatalf("unrelated file vanished: %v", readErr)
		}
		if string(got) != string(otherBytes) {
			t.Fatalf("unrelated file mutated: got %q, want %q", got, otherBytes)
		}
	})

	t.Run("filesystem write failure surfaces ErrStore", func(t *testing.T) {
		t.Parallel()

		if os.Geteuid() == 0 {
			t.Skip("running as root, dir perms ignored")
		}

		root := t.TempDir()
		// Pre-create the per-source-type subdir and make it non-writable so
		// CreateTemp inside it fails. This drives the post-stat write-path
		// failure that the spec mandates surfaces ErrStore.
		subdir := filepath.Join(root, "paper")
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatalf("seed subdir: %v", err)
		}
		if err := os.Chmod(subdir, 0o555); err != nil {
			t.Fatalf("chmod subdir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(subdir, 0o755) })

		fetcher := &mocks.Fetcher{Body: []byte("upstream payload")}
		store := newTestStoreWith(t, root, fetcher)
		key := newTestKey()

		_, err := store.Ensure(context.Background(), key)

		if err == nil {
			t.Fatalf("Ensure with non-writable subdir: err = nil, want error")
		}
		if !errors.Is(err, pdf.ErrStore) {
			t.Fatalf("error must wrap pdf.ErrStore, got %v", err)
		}
	})
}

// newTestKey returns a well-formed Key suitable for Ensure tests.
func newTestKey() pdf.Key {
	return pdf.Key{SourceType: "paper", SourceID: "2404.12345v1", URL: "https://example.invalid/p.pdf"}
}

// newTestStoreWith constructs a *localStore wired to fetcher. Returns the
// concrete type so tests can reach the unexported helpers if needed.
func newTestStoreWith(t *testing.T, root string, fetcher shared.Fetcher) *localStore {
	t.Helper()
	store, err := NewStore(root, fetcher, &mocks.RecordingLogger{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	s, ok := store.(*localStore)
	if !ok {
		t.Fatalf("NewStore returned %T, want *localStore", store)
	}
	return s
}

// seedFile writes bytes to path, creating parent dirs. Used to pre-populate
// the canonical layout for cache-hit and zero-byte-replacement tests.
func seedFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("seed mkdir: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("seed write: %v", err)
	}
}

// assertNoFiles fails if root contains any regular file. Used to verify
// that error paths leave the store empty.
func assertNoFiles(t *testing.T, root string) {
	t.Helper()
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || p == root {
			return nil
		}
		if info.Mode().IsRegular() {
			t.Fatalf("expected no files under %q, found %q", root, p)
		}
		return nil
	})
}

// assertNoTempSiblings fails if dir contains any *.tmp file. Verifies that
// the atomic-write recipe never leaves a temp sibling behind.
func assertNoTempSiblings(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatalf("glob tmp siblings: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("found leftover temp siblings under %q: %v", dir, matches)
	}
}

// bytesFilled returns a slice of length n filled with v. Used to construct
// a deterministic non-trivial body for round-trip byte assertions without
// pulling in a randomness dependency.
func bytesFilled(n int, v byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = v
	}
	return out
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
