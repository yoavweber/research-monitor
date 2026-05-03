package shared_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

func TestErrBadStatus(t *testing.T) {
	t.Parallel()

	t.Run("is a non-nil sentinel with the documented message", func(t *testing.T) {
		t.Parallel()

		if shared.ErrBadStatus == nil {
			t.Fatal("shared.ErrBadStatus must be a non-nil error sentinel")
		}
		const want = "shared.fetch: upstream returned non-success status"
		if got := shared.ErrBadStatus.Error(); got != want {
			t.Fatalf("shared.ErrBadStatus.Error() = %q, want %q", got, want)
		}
	})

	t.Run("remains identifiable through error wrapping", func(t *testing.T) {
		t.Parallel()

		wrapped := fmt.Errorf("%w: status=%d", shared.ErrBadStatus, 500)
		if !errors.Is(wrapped, shared.ErrBadStatus) {
			t.Fatalf("errors.Is(wrapped, shared.ErrBadStatus) = false, want true")
		}
		if !strings.Contains(wrapped.Error(), "status=500") {
			t.Fatalf("wrapped.Error() = %q, want it to contain %q", wrapped.Error(), "status=500")
		}
	})
}

func TestHTTPError(t *testing.T) {
	t.Parallel()

	t.Run("constructed via NewHTTPError has empty Reason by default", func(t *testing.T) {
		t.Parallel()

		he := shared.NewHTTPError(502, "upstream failed", nil)

		if he.Reason != "" {
			t.Fatalf("default Reason = %q, want empty string", he.Reason)
		}
	})

	t.Run("WithReason sets Reason and preserves other fields", func(t *testing.T) {
		t.Parallel()

		he := shared.NewHTTPError(502, "upstream failed", nil).WithReason("llm_upstream")

		if he.Reason != "llm_upstream" {
			t.Fatalf("Reason after WithReason = %q, want %q", he.Reason, "llm_upstream")
		}
		if he.Code != 502 || he.Message != "upstream failed" {
			t.Fatalf("WithReason mutated other fields: code=%d message=%q", he.Code, he.Message)
		}
	})

	t.Run("AsHTTPError preserves Reason through error wrapping", func(t *testing.T) {
		t.Parallel()

		cause := errors.New("boom")
		he := shared.NewHTTPError(502, "upstream failed", cause).WithReason("llm_upstream")

		wrapped := fmt.Errorf("usecase: %w", he)

		got := shared.AsHTTPError(wrapped)
		if got == nil {
			t.Fatal("AsHTTPError returned nil for a wrapped *HTTPError")
		}
		if got.Reason != "llm_upstream" {
			t.Fatalf("preserved Reason = %q, want %q", got.Reason, "llm_upstream")
		}
	})
}
