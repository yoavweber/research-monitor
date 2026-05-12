package pdfdownload

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	t.Run("returns success status and empty category for a nil error", func(t *testing.T) {
		t.Parallel()

		status, category, desc := classify(nil)

		if status != StatusSuccess {
			t.Errorf("status = %q, want %q", status, StatusSuccess)
		}
		if category != "" {
			t.Errorf("category = %q, want empty", category)
		}
		if desc != "" {
			t.Errorf("description = %q, want empty", desc)
		}
	})

	t.Run("maps a wrapped ErrInvalidKey error to the invalid_key category", func(t *testing.T) {
		t.Parallel()
		err := fmt.Errorf("key check: %w", pdf.ErrInvalidKey)

		status, category, desc := classify(err)

		if status != StatusFailed {
			t.Errorf("status = %q, want %q", status, StatusFailed)
		}
		if category != pdf.CategoryInvalidKey {
			t.Errorf("category = %q, want %q", category, pdf.CategoryInvalidKey)
		}
		if desc == "" {
			t.Errorf("description must not be empty")
		}
	})

	t.Run("maps a wrapped ErrFetch error to the fetch category", func(t *testing.T) {
		t.Parallel()
		err := fmt.Errorf("upstream timeout: %w", pdf.ErrFetch)

		status, category, _ := classify(err)

		if status != StatusFailed {
			t.Errorf("status = %q, want %q", status, StatusFailed)
		}
		if category != pdf.CategoryFetch {
			t.Errorf("category = %q, want %q", category, pdf.CategoryFetch)
		}
	})

	t.Run("maps a wrapped ErrStore error to the store category", func(t *testing.T) {
		t.Parallel()
		err := fmt.Errorf("disk write: %w", pdf.ErrStore)

		status, category, _ := classify(err)

		if status != StatusFailed {
			t.Errorf("status = %q, want %q", status, StatusFailed)
		}
		if category != pdf.CategoryStore {
			t.Errorf("category = %q, want %q", category, pdf.CategoryStore)
		}
	})

	t.Run("falls back to the unknown category for a plain error", func(t *testing.T) {
		t.Parallel()
		err := errors.New("something else")

		status, category, _ := classify(err)

		if status != StatusFailed {
			t.Errorf("status = %q, want %q", status, StatusFailed)
		}
		if category != "unknown" {
			t.Errorf("category = %q, want %q", category, "unknown")
		}
	})
}

func TestSanitize(t *testing.T) {
	t.Parallel()

	t.Run("removes absolute filesystem paths from the description", func(t *testing.T) {
		t.Parallel()

		got := sanitize(errors.New("read /var/lib/pdfs/arxiv/2404.pdf: permission denied"))

		if strings.Contains(got, "/var/lib/pdfs/arxiv/2404.pdf") {
			t.Errorf("sanitize leaked absolute path: %q", got)
		}
		if !strings.Contains(got, "permission denied") {
			t.Errorf("sanitize stripped too much, lost the underlying message: %q", got)
		}
	})

	t.Run("redacts credential-like tokens", func(t *testing.T) {
		t.Parallel()

		got := sanitize(errors.New("auth failed: Bearer abcdef1234567890 against upstream"))

		if strings.Contains(strings.ToLower(got), "abcdef1234567890") {
			t.Errorf("sanitize leaked bearer token: %q", got)
		}
	})

	t.Run("redacts inline password assignments", func(t *testing.T) {
		t.Parallel()

		got := sanitize(errors.New("connect failed for password=super-secret-9000 on upstream"))

		if strings.Contains(got, "super-secret-9000") {
			t.Errorf("sanitize leaked password value: %q", got)
		}
	})

	t.Run("truncates very long messages to a bounded length", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("x", 4000)

		got := sanitize(errors.New(long))

		if len(got) > sanitizedMaxLen {
			t.Errorf("sanitized length = %d, want <= %d", len(got), sanitizedMaxLen)
		}
	})
}
