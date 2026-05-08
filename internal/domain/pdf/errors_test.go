package pdf_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

func TestErrorSentinels(t *testing.T) {
	t.Parallel()

	t.Run("the three sentinels are non-nil and distinct", func(t *testing.T) {
		t.Parallel()

		sentinels := map[string]error{
			"ErrInvalidKey": pdf.ErrInvalidKey,
			"ErrFetch":      pdf.ErrFetch,
			"ErrStore":      pdf.ErrStore,
		}

		for name, err := range sentinels {
			if err == nil {
				t.Fatalf("%s must be non-nil", name)
			}
		}

		if errors.Is(pdf.ErrInvalidKey, pdf.ErrFetch) {
			t.Fatalf("ErrInvalidKey must be distinct from ErrFetch")
		}
		if errors.Is(pdf.ErrInvalidKey, pdf.ErrStore) {
			t.Fatalf("ErrInvalidKey must be distinct from ErrStore")
		}
		if errors.Is(pdf.ErrFetch, pdf.ErrStore) {
			t.Fatalf("ErrFetch must be distinct from ErrStore")
		}
	})

	t.Run("each sentinel is identifiable through %w wrapping via errors.Is", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name     string
			sentinel error
		}{
			{"ErrInvalidKey", pdf.ErrInvalidKey},
			{"ErrFetch", pdf.ErrFetch},
			{"ErrStore", pdf.ErrStore},
		}

		for _, c := range cases {
			wrapped := fmt.Errorf("context: %w", c.sentinel)

			if !errors.Is(wrapped, c.sentinel) {
				t.Fatalf("errors.Is must identify %s through %%w wrapping", c.name)
			}
		}
	})
}
