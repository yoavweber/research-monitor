package mocks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

// Compile-time conformance — surfaces pdf.Store and pdf.Locator port
// drift at build time.
var _ pdf.Store = (*PDFStore)(nil)
var _ pdf.Locator = (*pdfLocator)(nil)

// PDFStoreResponse configures a single Ensure outcome. Exactly one of
// Body or Err should be set: Body materializes a real file under the
// fake's RootDir so callers that os.Stat(locator.Path()) get a true byte
// count; Err is returned as-is so tests can inject the same wrapped
// sentinels (pdf.ErrInvalidKey, pdf.ErrFetch, pdf.ErrStore) the real
// store would produce.
type PDFStoreResponse struct {
	Body []byte
	Err  error
}

// PDFStore is a hand-written pdf.Store fake for unit tests. It mirrors
// the structure of other mocks in this package: a sync.Mutex guards
// recorded state, exported fields configure the response, and a Calls
// slice plus per-key response map let tests assert call patterns and
// inject deterministic outcomes.
//
// Default behavior: a key that has no entry in Responses returns an
// empty PDFStoreResponse — i.e. a zero-byte file. Tests should populate
// Responses for every key they expect to be Ensured.
type PDFStore struct {
	mu sync.Mutex

	// RootDir is where Body bytes are materialized. Tests typically pass
	// t.TempDir() so files are auto-cleaned.
	RootDir string

	// Responses maps a pdf.Key to the configured outcome. Tests set
	// entries before invoking the system under test.
	Responses map[pdf.Key]PDFStoreResponse

	// Calls records the Keys received by Ensure, in call order.
	Calls []pdf.Key
}

// NewPDFStore returns a PDFStore rooted at rootDir. rootDir must be a
// pre-existing writable directory (typically t.TempDir()).
func NewPDFStore(rootDir string) *PDFStore {
	return &PDFStore{
		RootDir:   rootDir,
		Responses: map[pdf.Key]PDFStoreResponse{},
	}
}

// Ensure satisfies pdf.Store. On a configured Body, it writes the bytes
// to a deterministic path under RootDir and returns a pdf.Locator over
// that path so os.Stat / Open both work as in production. On a
// configured Err, it returns that error verbatim (callers must wrap
// with the appropriate pdf.Err* sentinel themselves).
func (s *PDFStore) Ensure(_ context.Context, key pdf.Key) (pdf.Locator, error) {
	s.mu.Lock()
	resp, ok := s.Responses[key]
	s.Calls = append(s.Calls, key)
	s.mu.Unlock()

	if !ok {
		// Tests that forget to configure the key get an empty body. The
		// worker will see zero bytes; tests that care should configure
		// Responses explicitly.
		resp = PDFStoreResponse{}
	}

	if resp.Err != nil {
		return nil, resp.Err
	}

	dir := filepath.Join(s.RootDir, key.SourceType)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, errors.Join(pdf.ErrStore, fmt.Errorf("mocks pdf store: mkdir %q: %w", dir, err))
	}
	path := filepath.Join(dir, key.SourceID+".pdf")
	if err := os.WriteFile(path, resp.Body, 0o644); err != nil {
		return nil, errors.Join(pdf.ErrStore, fmt.Errorf("mocks pdf store: write %q: %w", path, err))
	}
	return &pdfLocator{path: path}, nil
}

// pdfLocator is a minimal pdf.Locator backed by a real file written
// under the fake store's RootDir. Path() and Open() behave the same as
// the production locator, so production code that uses os.Stat or
// io.Copy works against this fake unchanged.
type pdfLocator struct{ path string }

func (l *pdfLocator) Path() string { return l.path }

func (l *pdfLocator) Open(ctx context.Context) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(l.path)
	if err != nil {
		return nil, errors.Join(pdf.ErrStore, err)
	}
	return f, nil
}
