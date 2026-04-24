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
