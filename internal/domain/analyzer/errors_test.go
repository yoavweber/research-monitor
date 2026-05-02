package analyzer_test

import (
	"net/http"
	"testing"

	analyzer "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// TestSentinels_MatchSentinelMap pins each domain sentinel to the (HTTP code,
// reason) pair documented in design.md's sentinel map. If the design changes,
// this test forces the table to update in lockstep with the values.
func TestSentinels_MatchSentinelMap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		sentinel   error
		wantCode   int
		wantReason string
	}{
		{"ErrExtractionNotFound", analyzer.ErrExtractionNotFound, http.StatusNotFound, "extraction_not_found"},
		{"ErrExtractionNotReady", analyzer.ErrExtractionNotReady, http.StatusConflict, "extraction_not_ready"},
		{"ErrLLMUpstream", analyzer.ErrLLMUpstream, http.StatusBadGateway, "llm_upstream"},
		{"ErrAnalyzerMalformedResponse", analyzer.ErrAnalyzerMalformedResponse, http.StatusBadGateway, "llm_malformed_response"},
		{"ErrAnalysisNotFound", analyzer.ErrAnalysisNotFound, http.StatusNotFound, "analysis_not_found"},
		{"ErrCatalogueUnavailable", analyzer.ErrCatalogueUnavailable, http.StatusInternalServerError, ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+" wraps the documented HTTPError", func(t *testing.T) {
			t.Parallel()

			he := shared.AsHTTPError(tc.sentinel)

			if he == nil {
				t.Fatalf("AsHTTPError returned nil for %s; sentinel must wrap *shared.HTTPError", tc.name)
			}
			if he.Code != tc.wantCode {
				t.Errorf("%s.Code = %d, want %d", tc.name, he.Code, tc.wantCode)
			}
			if he.Reason != tc.wantReason {
				t.Errorf("%s.Reason = %q, want %q", tc.name, he.Reason, tc.wantReason)
			}
			if he.Message == "" {
				t.Errorf("%s.Message is empty; every sentinel must carry a human-readable message", tc.name)
			}
		})
	}
}
