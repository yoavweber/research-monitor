package paper_test

import (
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

func TestErrUpstreamBadStatus(t *testing.T) {
	t.Parallel()

	if paper.ErrUpstreamBadStatus == nil {
		t.Fatal("ErrUpstreamBadStatus must not be nil")
	}
	if got, want := paper.ErrUpstreamBadStatus.Code, 502; got != want {
		t.Errorf("ErrUpstreamBadStatus.Code = %d, want %d", got, want)
	}
	if got, want := paper.ErrUpstreamBadStatus.Message, "paper source returned non-success status"; got != want {
		t.Errorf("ErrUpstreamBadStatus.Message = %q, want %q", got, want)
	}
	if got := shared.AsHTTPError(paper.ErrUpstreamBadStatus); got != paper.ErrUpstreamBadStatus {
		t.Errorf("shared.AsHTTPError(ErrUpstreamBadStatus) = %v, want same pointer as sentinel", got)
	}
}

func TestErrUpstreamMalformed(t *testing.T) {
	t.Parallel()

	if paper.ErrUpstreamMalformed == nil {
		t.Fatal("ErrUpstreamMalformed must not be nil")
	}
	if got, want := paper.ErrUpstreamMalformed.Code, 502; got != want {
		t.Errorf("ErrUpstreamMalformed.Code = %d, want %d", got, want)
	}
	if got, want := paper.ErrUpstreamMalformed.Message, "paper source returned malformed response"; got != want {
		t.Errorf("ErrUpstreamMalformed.Message = %q, want %q", got, want)
	}
	if got := shared.AsHTTPError(paper.ErrUpstreamMalformed); got != paper.ErrUpstreamMalformed {
		t.Errorf("shared.AsHTTPError(ErrUpstreamMalformed) = %v, want same pointer as sentinel", got)
	}
}

func TestErrUpstreamUnavailable(t *testing.T) {
	t.Parallel()

	if paper.ErrUpstreamUnavailable == nil {
		t.Fatal("ErrUpstreamUnavailable must not be nil")
	}
	if got, want := paper.ErrUpstreamUnavailable.Code, 504; got != want {
		t.Errorf("ErrUpstreamUnavailable.Code = %d, want %d", got, want)
	}
	if got, want := paper.ErrUpstreamUnavailable.Message, "paper source unavailable"; got != want {
		t.Errorf("ErrUpstreamUnavailable.Message = %q, want %q", got, want)
	}
	if got := shared.AsHTTPError(paper.ErrUpstreamUnavailable); got != paper.ErrUpstreamUnavailable {
		t.Errorf("shared.AsHTTPError(ErrUpstreamUnavailable) = %v, want same pointer as sentinel", got)
	}
}

func TestErrNotFound(t *testing.T) {
	t.Parallel()

	if paper.ErrNotFound == nil {
		t.Fatal("ErrNotFound must not be nil")
	}
	if got, want := paper.ErrNotFound.Code, 404; got != want {
		t.Errorf("ErrNotFound.Code = %d, want %d", got, want)
	}
	if got, want := paper.ErrNotFound.Message, "paper not found"; got != want {
		t.Errorf("ErrNotFound.Message = %q, want %q", got, want)
	}
	if got := shared.AsHTTPError(paper.ErrNotFound); got != paper.ErrNotFound {
		t.Errorf("shared.AsHTTPError(ErrNotFound) = %v, want same pointer as sentinel", got)
	}
}

func TestErrCatalogueUnavailable(t *testing.T) {
	t.Parallel()

	if paper.ErrCatalogueUnavailable == nil {
		t.Fatal("ErrCatalogueUnavailable must not be nil")
	}
	if got, want := paper.ErrCatalogueUnavailable.Code, 500; got != want {
		t.Errorf("ErrCatalogueUnavailable.Code = %d, want %d", got, want)
	}
	if got, want := paper.ErrCatalogueUnavailable.Message, "paper catalogue unavailable"; got != want {
		t.Errorf("ErrCatalogueUnavailable.Message = %q, want %q", got, want)
	}
	if got := shared.AsHTTPError(paper.ErrCatalogueUnavailable); got != paper.ErrCatalogueUnavailable {
		t.Errorf("shared.AsHTTPError(ErrCatalogueUnavailable) = %v, want same pointer as sentinel", got)
	}
}
