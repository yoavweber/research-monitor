package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Compile-time check that *localStore satisfies pdf.Store. If the domain
// port drifts, this fails at build time rather than at first use.
var _ pdf.Store = (*localStore)(nil)

// localStore is the filesystem-backed implementation of pdf.Store. It owns
// a root directory under which artifacts are laid out as
// <root>/<source_type>/<source_id>.pdf. The struct is unexported because
// callers depend on the pdf.Store interface returned by NewStore — never on
// the concrete type — so the wiring layer can swap implementations
// (in-memory fake, S3-backed, etc.) without rippling through the codebase.
type localStore struct {
	root    string
	fetcher shared.Fetcher
	logger  shared.Logger
}

// NewStore constructs a filesystem-backed pdf.Store rooted at root.
//
// The constructor is fail-fast: it ensures the root exists (creating it
// with mode 0o755 if missing), verifies that the resolved path is a
// directory, and probes writability by creating and removing a temp file.
// Any failure returns an error that joins pdf.ErrStore so callers can
// classify the failure via errors.Is(err, pdf.ErrStore), and includes the
// offending root in the message so misconfiguration is diagnosable from a
// single log line.
//
// fetcher and logger are stored unmodified for use by Ensure (Task 3.3).
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

	// Writability probe. MkdirAll succeeds on a directory we cannot write
	// into (e.g. mode 0o555); without this probe, the misconfiguration
	// would only surface on the first Ensure call, far from the bootstrap
	// log line. Creating and immediately removing a temp file is the
	// canonical Go check.
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

// canonicalPath returns the on-disk path for key under the store root,
// following the documented layout <root>/<source_type>/<source_id>.pdf.
//
// The helper assumes key has already been validated via pdf.Key.Validate
// (which rejects path-separator and traversal characters), so the joined
// path is guaranteed to remain under root for any well-formed input.
func (s *localStore) canonicalPath(key pdf.Key) string {
	return filepath.Join(s.root, key.SourceType, key.SourceID+".pdf")
}

// Ensure materializes the artifact identified by key onto the local
// filesystem and returns a Locator over the canonical path. It is the
// fetch-and-cache entry point promised by pdf.Store.
//
// Order of operations (each step exists for a reason; see comments inline):
//  1. Validate key. Reject without I/O — bad inputs must never touch the
//     filesystem or the upstream fetcher.
//  2. Existence gate. A non-zero canonical file is treated as a cache
//     hit; we return immediately without invoking the fetcher. A
//     zero-byte file is a left-over sentinel from an earlier failure
//     and we deliberately fall through to fetch (Req 2.4).
//  3. Fetch. Errors here are wrapped with both pdf.ErrFetch and the
//     underlying cause via errors.Join so callers can classify with
//     errors.Is against pdf.ErrFetch, shared.ErrBadStatus, or
//     context.Canceled / context.DeadlineExceeded.
//  4. Atomic write. Create a sibling temp in the same directory as the
//     canonical path, write, close, then rename. Same-directory rename
//     is atomic on POSIX, so the canonical path either points at a
//     fully-written file or at nothing — never at a partial.
//  5. Cleanup. On any error after temp creation, remove the temp file
//     so we never leak *.tmp siblings.
//
// An empty fetcher response body is treated as a fetch failure rather
// than written to disk: a zero-byte canonical file would be wasted disk
// state that the next Ensure call would re-fetch anyway, and would
// violate the contract that successful Ensure always yields readable
// bytes.
func (s *localStore) Ensure(ctx context.Context, key pdf.Key) (pdf.Locator, error) {
	if err := key.Validate(); err != nil {
		// Validation failures originate in caller bugs rather than in
		// infrastructure: warn level matches "client error" semantics
		// (the third category alongside fetch/store in the Monitoring table).
		s.logger.WarnContext(ctx, "pdf.store.failed",
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"category", "invalid_key",
			"error", err.Error(),
		)
		return nil, err
	}

	canonical := s.canonicalPath(key)

	// Cache gate. We deliberately ignore non-nil os.Stat errors (including
	// fs.ErrNotExist) and treat them as misses; the subsequent Mkdir/Create
	// path will surface any genuine filesystem trouble as ErrStore with a
	// concrete cause, which is more diagnostic than a stat error here.
	if info, err := os.Stat(canonical); err == nil && info.Size() > 0 {
		s.logger.InfoContext(ctx, "pdf.store.cache_hit",
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"bytes", int(info.Size()),
		)
		return newLocator(canonical), nil
	}

	parent := filepath.Dir(canonical)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		wrapped := errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: mkdir %q: %w", parent, err))
		s.logger.ErrorContext(ctx, "pdf.store.failed",
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"category", "store",
			"error", wrapped.Error(),
		)
		return nil, wrapped
	}

	// Time the fetch+write phase. A clock is not injected on localStore by
	// design — adding one would expand NewStore's signature beyond what the
	// design's Components and Interfaces section lists (Fetcher + Logger).
	// time.Now() inline is acceptable here because duration_ms is an
	// observability field, not a domain decision.
	start := time.Now()

	body, err := s.fetcher.Fetch(ctx, key.URL)
	if err != nil {
		// errors.Join preserves both directions of errors.Is so callers
		// can identify the category (pdf.ErrFetch) and the underlying
		// cause (shared.ErrBadStatus, context.Canceled, *url.Error, ...).
		wrapped := errors.Join(pdf.ErrFetch, fmt.Errorf("pdf local store: fetch %s/%s: %w", key.SourceType, key.SourceID, err))
		s.logger.WarnContext(ctx, "pdf.store.failed",
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"category", "fetch",
			"error", wrapped.Error(),
		)
		return nil, wrapped
	}
	if len(body) == 0 {
		wrapped := errors.Join(pdf.ErrFetch, fmt.Errorf("pdf local store: fetch %s/%s: empty response body", key.SourceType, key.SourceID))
		s.logger.WarnContext(ctx, "pdf.store.failed",
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"category", "fetch",
			"error", wrapped.Error(),
		)
		return nil, wrapped
	}

	// Same-directory temp ensures the rename is atomic; cross-device
	// renames would not be. The randomized suffix from CreateTemp lets
	// concurrent Ensure calls for the same key coexist without colliding
	// on the temp filename — the loser's rename simply overwrites the
	// winner's canonical bytes (idempotent for identical content).
	tmp, err := os.CreateTemp(parent, key.SourceID+".*.pdf.tmp")
	if err != nil {
		wrapped := errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: create temp in %q: %w", parent, err))
		s.logger.ErrorContext(ctx, "pdf.store.failed",
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"category", "store",
			"error", wrapped.Error(),
		)
		return nil, wrapped
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		wrapped := errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: write temp %q: %w", tmpPath, err))
		s.logger.ErrorContext(ctx, "pdf.store.failed",
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"category", "store",
			"error", wrapped.Error(),
		)
		return nil, wrapped
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		wrapped := errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: close temp %q: %w", tmpPath, err))
		s.logger.ErrorContext(ctx, "pdf.store.failed",
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"category", "store",
			"error", wrapped.Error(),
		)
		return nil, wrapped
	}

	if err := os.Rename(tmpPath, canonical); err != nil {
		_ = os.Remove(tmpPath)
		wrapped := errors.Join(pdf.ErrStore, fmt.Errorf("pdf local store: rename %q -> %q: %w", tmpPath, canonical, err))
		s.logger.ErrorContext(ctx, "pdf.store.failed",
			"source_type", key.SourceType,
			"source_id", key.SourceID,
			"category", "store",
			"error", wrapped.Error(),
		)
		return nil, wrapped
	}

	s.logger.InfoContext(ctx, "pdf.store.fetched",
		"source_type", key.SourceType,
		"source_id", key.SourceID,
		"bytes", len(body),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return newLocator(canonical), nil
}
