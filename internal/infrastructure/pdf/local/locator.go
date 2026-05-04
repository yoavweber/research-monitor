// Package local provides a filesystem-backed implementation of the
// pdf.Store and pdf.Locator ports defined in internal/domain/pdf. The
// locator addresses a single artifact that has already been materialized
// onto the local disk, exposing both a path view (for path-only tools
// like mineru) and a stream view (for io.Reader plumbing) over the same
// bytes.
package local

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

// Compile-time check that *localLocator satisfies pdf.Locator. If the
// domain port drifts, this fails at build time rather than at first use.
var _ pdf.Locator = (*localLocator)(nil)

// localLocator is the on-disk implementation of pdf.Locator. It carries
// only the canonical absolute path; the file at that path is owned by
// the surrounding localStore for the lifetime of this locator.
type localLocator struct {
	path string
}

// newLocator returns a pdf.Locator over the file at path. The constructor
// is unexported because locators are only ever produced by localStore;
// callers depend on the pdf.Locator interface, never on the concrete type.
func newLocator(path string) pdf.Locator {
	return &localLocator{path: path}
}

// Path returns the canonical on-disk path. It performs no I/O and never
// returns an error, in line with the pdf.Locator contract.
func (l *localLocator) Path() string {
	return l.path
}

// Open returns an io.ReadCloser over the file at Path. The ctx parameter
// is accepted for conformance with the pdf.Locator interface and future
// remote-backed locators; v1 does not consult it because os.Open is a
// non-blocking syscall on a local filesystem and there is no pre-read
// work that could observe cancellation. We deliberately do not start a
// goroutine to watch ctx: with no reader yet attached, racing a Close
// against a caller that never reads would risk a double-close on the
// returned handle.
//
// On failure the underlying os error is joined with pdf.ErrStore so that
// callers can identify the category via errors.Is(err, pdf.ErrStore)
// while still inspecting the wrapped os error (e.g. fs.ErrNotExist) for
// finer-grained handling.
func (l *localLocator) Open(ctx context.Context) (io.ReadCloser, error) {
	_ = ctx
	f, err := os.Open(l.path)
	if err != nil {
		return nil, errors.Join(pdf.ErrStore, err)
	}
	return f, nil
}
