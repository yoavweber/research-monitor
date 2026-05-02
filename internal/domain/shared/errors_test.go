package shared_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

func TestErrBadStatus_IsNonNilError(t *testing.T) {
	t.Parallel()

	if shared.ErrBadStatus == nil {
		t.Fatal("shared.ErrBadStatus must be a non-nil error sentinel")
	}
	const want = "shared.fetch: upstream returned non-success status"
	if got := shared.ErrBadStatus.Error(); got != want {
		t.Fatalf("shared.ErrBadStatus.Error() = %q, want %q", got, want)
	}
}

func TestErrBadStatus_WrappingIsIdentifiable(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("%w: status=%d", shared.ErrBadStatus, 500)
	if !errors.Is(wrapped, shared.ErrBadStatus) {
		t.Fatalf("errors.Is(wrapped, shared.ErrBadStatus) = false, want true")
	}
	if !strings.Contains(wrapped.Error(), "status=500") {
		t.Fatalf("wrapped.Error() = %q, want it to contain %q", wrapped.Error(), "status=500")
	}
}

func TestHTTPError_NewHTTPError_HasEmptyReasonByDefault(t *testing.T) {
	t.Parallel()

	he := shared.NewHTTPError(502, "upstream failed", nil)

	if he.Reason != "" {
		t.Fatalf("default Reason = %q, want empty string", he.Reason)
	}
}

func TestHTTPError_WithReason_SetsReasonAndReturnsSameValue(t *testing.T) {
	t.Parallel()

	he := shared.NewHTTPError(502, "upstream failed", nil).WithReason("llm_upstream")

	if he.Reason != "llm_upstream" {
		t.Fatalf("Reason after WithReason = %q, want %q", he.Reason, "llm_upstream")
	}
	if he.Code != 502 || he.Message != "upstream failed" {
		t.Fatalf("WithReason mutated other fields: code=%d message=%q", he.Code, he.Message)
	}
}

func TestHTTPError_AsHTTPError_PreservesReasonThroughWrap(t *testing.T) {
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
}
