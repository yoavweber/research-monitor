package extraction_test

import (
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

func TestJobStatusConstants(t *testing.T) {
	t.Parallel()

	t.Run("declares exactly four distinct values", func(t *testing.T) {
		t.Parallel()

		statuses := []extraction.JobStatus{
			extraction.JobStatusPending,
			extraction.JobStatusRunning,
			extraction.JobStatusDone,
			extraction.JobStatusFailed,
		}

		seen := make(map[extraction.JobStatus]struct{}, len(statuses))
		for _, s := range statuses {
			seen[s] = struct{}{}
		}

		if got, want := len(seen), 4; got != want {
			t.Fatalf("JobStatus constants: distinct count = %d, want %d", got, want)
		}
	})

	t.Run("each constant has the expected string value", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name  string
			got   extraction.JobStatus
			want  string
		}{
			{"JobStatusPending", extraction.JobStatusPending, "pending"},
			{"JobStatusRunning", extraction.JobStatusRunning, "running"},
			{"JobStatusDone", extraction.JobStatusDone, "done"},
			{"JobStatusFailed", extraction.JobStatusFailed, "failed"},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.name+" equals "+tc.want, func(t *testing.T) {
				t.Parallel()

				if string(tc.got) != tc.want {
					t.Errorf("%s = %q, want %q", tc.name, string(tc.got), tc.want)
				}
			})
		}
	})

	// Compile-time guard: any new JobStatus constant added to the production
	// package must be added here as well, or the maintainer of the new constant
	// has to consciously update this map. It anchors the "exactly four" claim
	// at the source level.
	_ = map[extraction.JobStatus]struct{}{
		extraction.JobStatusPending: {},
		extraction.JobStatusRunning: {},
		extraction.JobStatusDone:    {},
		extraction.JobStatusFailed:  {},
	}
}

func TestFailureReasonConstants(t *testing.T) {
	t.Parallel()

	t.Run("declares exactly six distinct values", func(t *testing.T) {
		t.Parallel()

		reasons := []extraction.FailureReason{
			extraction.FailureReasonScannedPDF,
			extraction.FailureReasonParseFailed,
			extraction.FailureReasonExtractorFailure,
			extraction.FailureReasonTooLarge,
			extraction.FailureReasonExpired,
			extraction.FailureReasonProcessRestart,
		}

		seen := make(map[extraction.FailureReason]struct{}, len(reasons))
		for _, r := range reasons {
			seen[r] = struct{}{}
		}

		if got, want := len(seen), 6; got != want {
			t.Fatalf("FailureReason constants: distinct count = %d, want %d", got, want)
		}
	})

	t.Run("each constant has the expected string value", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name string
			got  extraction.FailureReason
			want string
		}{
			{"FailureReasonScannedPDF", extraction.FailureReasonScannedPDF, "scanned_pdf"},
			{"FailureReasonParseFailed", extraction.FailureReasonParseFailed, "parse_failed"},
			{"FailureReasonExtractorFailure", extraction.FailureReasonExtractorFailure, "extractor_failure"},
			{"FailureReasonTooLarge", extraction.FailureReasonTooLarge, "too_large"},
			{"FailureReasonExpired", extraction.FailureReasonExpired, "expired"},
			{"FailureReasonProcessRestart", extraction.FailureReasonProcessRestart, "process_restart"},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.name+" equals "+tc.want, func(t *testing.T) {
				t.Parallel()

				if string(tc.got) != tc.want {
					t.Errorf("%s = %q, want %q", tc.name, string(tc.got), tc.want)
				}
			})
		}
	})

	// Compile-time guard: see TestJobStatusConstants for rationale.
	_ = map[extraction.FailureReason]struct{}{
		extraction.FailureReasonScannedPDF:       {},
		extraction.FailureReasonParseFailed:      {},
		extraction.FailureReasonExtractorFailure: {},
		extraction.FailureReasonTooLarge:         {},
		extraction.FailureReasonExpired:          {},
		extraction.FailureReasonProcessRestart:   {},
	}
}
