package local

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

var _ pdf.Store = (*localStore)(nil)

// localStore is the filesystem-backed implementation of pdf.Store. The struct
// is unexported so callers depend on pdf.Store rather than the concrete type;
// the wiring layer can swap implementations (in-memory fake, S3-backed, etc.)
// without rippling through the codebase.
type localStore struct {
	root    string
	fetcher shared.Fetcher
	logger  shared.Logger
}

// NewStore constructs a filesystem-backed pdf.Store rooted at root.
// Fail-fast at startup so a misconfigured root cannot surface mid-request:
// the constructor creates the directory if missing, verifies it is a writable
// directory, and returns an error joining pdf.ErrStore with the misconfigured
// path on any violation.
func NewStore(root string, fetcher shared.Fetcher, logger shared.Logger) (pdf.Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: mkdir root %q: %w", root, err))
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: stat root %q: %w", root, err))
	}
	if !info.IsDir() {
		return nil, errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: root %q is not a directory", root))
	}

	// MkdirAll succeeds on a directory we cannot write into (e.g. mode 0o555);
	// without this probe, the misconfiguration would only surface on the first
	// Ensure call, far from the bootstrap log line.
	probe, err := os.CreateTemp(root, ".pdfstore-probe-*")
	if err != nil {
		return nil, errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: root %q is not writable: %w", root, err))
	}
	probePath := probe.Name()
	_ = probe.Close()
	if rmErr := os.Remove(probePath); rmErr != nil {
		return nil, errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: cleanup writability probe under %q: %w", root, rmErr))
	}

	return &localStore{
		root:    root,
		fetcher: fetcher,
		logger:  logger,
	}, nil
}

// canonicalPath returns the on-disk path for key under the store root.
// Layout: <root>/<source_type>/<source_id>.pdf. Assumes key has already been
// validated via pdf.Key.Validate, which rejects path-separator and traversal
// characters so the joined path is guaranteed to remain under root.
func (s *localStore) canonicalPath(key pdf.Key) string {
	return filepath.Join(s.root, key.SourceType, key.SourceID+".pdf")
}

// Ensure materializes the artifact identified by key onto the local
// filesystem and returns a Locator over the canonical path.
//
// An empty fetcher response body is treated as a fetch failure rather than
// written to disk: a zero-byte canonical file would be wasted disk state that
// the next Ensure call would re-fetch anyway, and would violate the contract
// that successful Ensure always yields readable bytes.
func (s *localStore) Ensure(ctx context.Context, key pdf.Key) (pdf.Locator, error) {
	if err := key.Validate(); err != nil {
		s.logger.WarnContext(ctx, pdf.EventFailed,
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"category", pdf.CategoryInvalidKey,
			"error", err.Error(),
		)
		return nil, err
	}

	canonical := s.canonicalPath(key)

	// Cache gate. Non-nil os.Stat errors (including fs.ErrNotExist) are
	// treated as misses; the subsequent Mkdir/Create path will surface any
	// genuine filesystem trouble as ErrStore with a concrete cause, which is
	// more diagnostic than a stat error here.
	if info, err := os.Stat(canonical); err == nil && info.Size() > 0 {
		s.logger.InfoContext(ctx, pdf.EventCacheHit,
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"bytes", int(info.Size()),
		)
		return newLocator(canonical), nil
	}

	parent := filepath.Dir(canonical)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, s.failStore(ctx, key, "mkdir %q: %w", parent, err)
	}

	start := time.Now()

	body, err := s.fetcher.Fetch(ctx, key.URL)
	if err != nil {
		return nil, s.failFetch(ctx, key, "fetch %s/%s: %w", key.SourceType, key.SourceID, err)
	}
	if len(body) == 0 {
		return nil, s.failFetch(ctx, key, "fetch %s/%s: empty response body", key.SourceType, key.SourceID)
	}

	// Same-directory temp ensures the rename is atomic; cross-device renames
	// would not be. The randomized suffix from CreateTemp lets concurrent
	// Ensure calls for the same key coexist without colliding on the temp
	// filename — the loser's rename simply overwrites the winner's canonical
	// bytes (idempotent for identical content).
	tmp, err := os.CreateTemp(parent, key.SourceID+".*.pdf.tmp")
	if err != nil {
		return nil, s.failStore(ctx, key, "create temp in %q: %w", parent, err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return nil, s.failStore(ctx, key, "write temp %q: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, s.failStore(ctx, key, "close temp %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, canonical); err != nil {
		_ = os.Remove(tmpPath)
		return nil, s.failStore(ctx, key, "rename %q -> %q: %w", tmpPath, canonical, err)
	}

	s.logger.InfoContext(ctx, pdf.EventFetched,
		"source_type", key.SourceType,
		"source_id", key.SourceID,
		"bytes", len(body),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return newLocator(canonical), nil
}

// failStore wraps a filesystem failure with pdf.ErrStore + cause, emits a
// pdf.store.failed log at error level, and returns the wrapped error so the
// caller can `return nil, s.failStore(...)` in a single line.
func (s *localStore) failStore(ctx context.Context, key pdf.Key, format string, args ...any) error {
	wrapped := errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: "+format, args...))
	s.emitFailed(ctx, key, slog.LevelError, pdf.CategoryStore, wrapped)
	return wrapped
}

// failFetch wraps an upstream/fetch failure with pdf.ErrFetch + cause, emits
// a pdf.store.failed log at warn level, and returns the wrapped error.
func (s *localStore) failFetch(ctx context.Context, key pdf.Key, format string, args ...any) error {
	wrapped := errors.Join(pdf.ErrFetch, fmt.Errorf("pdf local store: "+format, args...))
	s.emitFailed(ctx, key, slog.LevelWarn, pdf.CategoryFetch, wrapped)
	return wrapped
}

func (s *localStore) emitFailed(ctx context.Context, key pdf.Key, level slog.Level, category string, err error) {
	args := []any{
		"source_type", key.SourceType,
		"source_id", key.SourceID,
		"category", category,
		"error", err.Error(),
	}
	switch level {
	case slog.LevelError:
		s.logger.ErrorContext(ctx, pdf.EventFailed, args...)
	default:
		s.logger.WarnContext(ctx, pdf.EventFailed, args...)
	}
}
