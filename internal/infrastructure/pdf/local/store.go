package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// Ensure is implemented by Task 3.3. The panic stub guarantees that any
// accidental call before the real implementation lands fails loudly rather
// than silently returning a misleading nil error. Satisfies the pdf.Store
// interface so the rest of the package compiles in the meantime.
func (s *localStore) Ensure(ctx context.Context, key pdf.Key) (pdf.Locator, error) {
	panic("pdf local store: Ensure not implemented; task 3.3 owns this")
}
