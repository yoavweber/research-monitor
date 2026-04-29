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
			t.Fatalf("distinct count = %d, want %d", got, want)
		}
	})

	t.Run("constants serialise to their lifecycle words", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name string
			got  extraction.JobStatus
			want string
		}{
			{"pending", extraction.JobStatusPending, "pending"},
			{"running", extraction.JobStatusRunning, "running"},
			{"done", extraction.JobStatusDone, "done"},
			{"failed", extraction.JobStatusFailed, "failed"},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.name+" serialises as "+tc.want, func(t *testing.T) {
				t.Parallel()

				if string(tc.got) != tc.want {
					t.Errorf("got %q, want %q", string(tc.got), tc.want)
				}
			})
		}
	})
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
			t.Fatalf("distinct count = %d, want %d", got, want)
		}
	})

	t.Run("constants serialise to their failure words", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name string
			got  extraction.FailureReason
			want string
		}{
			{"scanned pdf", extraction.FailureReasonScannedPDF, "scanned_pdf"},
			{"parse failed", extraction.FailureReasonParseFailed, "parse_failed"},
			{"extractor failure", extraction.FailureReasonExtractorFailure, "extractor_failure"},
			{"too large", extraction.FailureReasonTooLarge, "too_large"},
			{"expired", extraction.FailureReasonExpired, "expired"},
			{"process restart", extraction.FailureReasonProcessRestart, "process_restart"},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.name+" serialises as "+tc.want, func(t *testing.T) {
				t.Parallel()

				if string(tc.got) != tc.want {
					t.Errorf("got %q, want %q", string(tc.got), tc.want)
				}
			})
		}
	})
}
