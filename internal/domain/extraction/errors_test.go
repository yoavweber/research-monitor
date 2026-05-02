package extraction_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// sentinelCase pairs a sentinel with its expected HTTP status code so the
// table-driven tests below can iterate over every extraction error in one place.
type sentinelCase struct {
	name     string
	sentinel *shared.HTTPError
	code     int
}

func extractionSentinels() []sentinelCase {
	return []sentinelCase{
		{"ErrInvalidRequest is 400", extraction.ErrInvalidRequest, http.StatusBadRequest},
		{"ErrUnsupportedSourceType is 400", extraction.ErrUnsupportedSourceType, http.StatusBadRequest},
		{"ErrNotFound is 404", extraction.ErrNotFound, http.StatusNotFound},
		{"ErrCatalogueUnavailable is 500", extraction.ErrCatalogueUnavailable, http.StatusInternalServerError},
		{"ErrInvalidTransition is 500", extraction.ErrInvalidTransition, http.StatusInternalServerError},
		{"ErrScannedPDF is 500", extraction.ErrScannedPDF, http.StatusInternalServerError},
		{"ErrParseFailed is 500", extraction.ErrParseFailed, http.StatusInternalServerError},
		{"ErrExtractorFailure is 500", extraction.ErrExtractorFailure, http.StatusInternalServerError},
	}
}

func TestExtractionSentinels_AsHTTPErrorRecoversCode(t *testing.T) {
	t.Parallel()

	for _, tc := range extractionSentinels() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("%w: while doing X", tc.sentinel)

			he := shared.AsHTTPError(wrapped)

			if he == nil {
				t.Fatalf("AsHTTPError returned nil for wrapped %v", tc.sentinel)
			}
			if he.Code != tc.code {
				t.Errorf("code = %d, want %d", he.Code, tc.code)
			}
			if he != tc.sentinel {
				t.Errorf("AsHTTPError did not recover the original sentinel pointer")
			}
		})
	}
}

func TestExtractionSentinels_ErrorsAsRecoversSentinel(t *testing.T) {
	t.Parallel()

	for _, tc := range extractionSentinels() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("%w: contextual detail", tc.sentinel)

			var he *shared.HTTPError
			ok := errors.As(wrapped, &he)

			if !ok {
				t.Fatalf("errors.As failed to recover *shared.HTTPError from wrapped sentinel")
			}
			if he.Code != tc.code {
				t.Errorf("recovered code = %d, want %d", he.Code, tc.code)
			}
			if he != tc.sentinel {
				t.Errorf("errors.As recovered a different pointer than the sentinel")
			}
		})
	}
}

func TestExtractionSentinels_AreNonNilAndCarryMessages(t *testing.T) {
	t.Parallel()

	for _, tc := range extractionSentinels() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.sentinel == nil {
				t.Fatalf("%s must not be nil", tc.name)
			}
			if tc.sentinel.Message == "" {
				t.Errorf("%s must carry a non-empty human-readable message", tc.name)
			}
		})
	}
}
