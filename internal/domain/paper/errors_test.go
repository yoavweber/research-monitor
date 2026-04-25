package paper_test

import (
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

func TestSentinels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  *shared.HTTPError
		code int
		msg  string
	}{
		{"ErrUpstreamBadStatus", paper.ErrUpstreamBadStatus, 502, "paper source returned non-success status"},
		{"ErrUpstreamMalformed", paper.ErrUpstreamMalformed, 502, "paper source returned malformed response"},
		{"ErrUpstreamUnavailable", paper.ErrUpstreamUnavailable, 504, "paper source unavailable"},
		{"ErrNotFound", paper.ErrNotFound, 404, "paper not found"},
		{"ErrCatalogueUnavailable", paper.ErrCatalogueUnavailable, 500, "paper catalogue unavailable"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil {
				t.Fatalf("%s must not be nil", tc.name)
			}
			if tc.err.Code != tc.code {
				t.Errorf("%s.Code = %d, want %d", tc.name, tc.err.Code, tc.code)
			}
			if tc.err.Message != tc.msg {
				t.Errorf("%s.Message = %q, want %q", tc.name, tc.err.Message, tc.msg)
			}
			if got := shared.AsHTTPError(tc.err); got != tc.err {
				t.Errorf("shared.AsHTTPError(%s) = %v, want same pointer as sentinel", tc.name, got)
			}
		})
	}
}
