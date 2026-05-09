package mocks_test

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

func TestPDFStore(t *testing.T) {
	t.Parallel()

	t.Run("records keys and writes the configured body to disk", func(t *testing.T) {
		t.Parallel()
		store := mocks.NewPDFStore(t.TempDir())

		key := pdf.Key{SourceType: "arxiv", SourceID: "2404.12345v1", URL: "http://example.test/x.pdf"}
		body := []byte("%PDF-1.4 minimal")
		store.Responses[key] = mocks.PDFStoreResponse{Body: body}

		loc, err := store.Ensure(context.Background(), key)

		if err != nil {
			t.Fatalf("Ensure returned error: %v", err)
		}
		if len(store.Calls) != 1 || store.Calls[0] != key {
			t.Errorf("Calls = %v, want [%v]", store.Calls, key)
		}
		info, statErr := os.Stat(loc.Path())
		if statErr != nil {
			t.Fatalf("stat locator path: %v", statErr)
		}
		if info.Size() != int64(len(body)) {
			t.Errorf("file size = %d, want %d", info.Size(), len(body))
		}
		r, openErr := loc.Open(context.Background())
		if openErr != nil {
			t.Fatalf("Open: %v", openErr)
		}
		got, _ := io.ReadAll(r)
		_ = r.Close()
		if string(got) != string(body) {
			t.Errorf("Open() returned %q, want %q", got, body)
		}
	})

	t.Run("returns the configured error verbatim", func(t *testing.T) {
		t.Parallel()
		store := mocks.NewPDFStore(t.TempDir())

		key := pdf.Key{SourceType: "arxiv", SourceID: "boom", URL: "http://example.test/x.pdf"}
		store.Responses[key] = mocks.PDFStoreResponse{Err: pdf.ErrFetch}

		_, err := store.Ensure(context.Background(), key)

		if !errors.Is(err, pdf.ErrFetch) {
			t.Fatalf("err = %v, want pdf.ErrFetch", err)
		}
		if len(store.Calls) != 1 {
			t.Errorf("expected one recorded call, got %d", len(store.Calls))
		}
	})
}
