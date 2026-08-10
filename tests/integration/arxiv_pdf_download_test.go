//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	arxivctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/arxiv"
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
	"github.com/yoavweber/research-monitor/backend/tests/ssetest"
)

// smallPDF is the minimal body served for the success paths. Real PDF
// validity is irrelevant for this slice — the local pdf.Store treats any
// non-empty 2xx response body as success and stat()s the on-disk file for
// byte count.
var smallPDF = []byte("%PDF-1.4\n%small stub\n%%EOF\n")

// runScenario drives the full happy-path / failure-variant flow. The path
// shape ("/pdf/<sourceid>") is fixed; pickFailingPath, if non-empty,
// selects exactly one path whose server response is HTTP 500.
//
// The function returns the captured SSE frames and the final status
// snapshot decoded into the wire-shape DTO so each scenario asserts
// whichever properties it cares about with typed field access.
func runScenario(
	t *testing.T,
	entries []paper.Entry,
	pickFailingPath string,
) ([]ssetest.Frame, paperctrl.DownloadJobSnapshotDTO) {
	t.Helper()

	// HTTP server backing the PDF fetches. Distinct paths per entry let the
	// failure variant fail one path and pass the others; the body is a tiny
	// PDF stub so success rows materialize on disk.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pickFailingPath != "" && r.URL.Path == pickFailingPath {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(smallPDF)
	}))
	t.Cleanup(srv.Close)

	// Patch each entry's PDFURL to point at the test server.
	for i := range entries {
		entries[i].PDFURL = srv.URL + entries[i].PDFURL
	}

	fake := &mocks.PaperFetcher{Entries: entries}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher:    fake,
		ArxivQuery:      paper.Query{Categories: []string{"cs.LG"}, MaxResults: 100},
		WirePDFDownload: true,
	})
	t.Cleanup(env.Close)

	// Trigger the fetch.
	fetchResp := doAuthenticatedGet(t, env.Server.URL+"/api/arxiv/fetch")
	defer fetchResp.Body.Close()
	if fetchResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/arxiv/fetch status = %d want 200", fetchResp.StatusCode)
	}

	var fetchBody arxivctrl.FetchEnvelope
	if err := json.NewDecoder(fetchResp.Body).Decode(&fetchBody); err != nil {
		t.Fatalf("decode fetch body: %v", err)
	}
	if fetchBody.Data.Job == nil || fetchBody.Data.Job.JobID == "" {
		t.Fatalf("fetch response missing data.job.job_id; body: %+v", fetchBody)
	}
	jobID := fetchBody.Data.Job.JobID

	// Open the SSE stream. Deadline-bound so a worker hang fails the test
	// instead of stalling the runner.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	streamReq, _ := http.NewRequestWithContext(ctx,
		http.MethodGet, env.Server.URL+"/api/arxiv/downloads/"+jobID+"/stream", nil)
	streamReq.Header.Set(middleware.APITokenHeader, setup.TestToken)
	streamResp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d want 200", streamResp.StatusCode)
	}
	if ct := streamResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("stream content-type = %q want text/event-stream", ct)
	}

	frames := readUntilSummary(t, streamResp.Body)

	// Status snapshot.
	statusResp := doAuthenticatedGet(t, env.Server.URL+"/api/arxiv/downloads/"+jobID)
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint = %d want 200", statusResp.StatusCode)
	}
	var statusBody paperctrl.JobStatusEnvelope
	if err := json.NewDecoder(statusResp.Body).Decode(&statusBody); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return frames, statusBody.Data
}

// TestIntegration_ArxivPDFDownload_HappyPath exercises the full flow end-
// to-end: arxiv fetch returns N entries -> registry schedules N downloads
// -> worker fetches PDFs over HTTP -> SSE stream replays + delivers N
// progress events and a terminal summary -> status endpoint reports the
// same per-entry results.
//
// Requirements covered: 1.1, 2.1, 2.2, 3.1, 3.2, 4.1, 4.2, 4.3, 5.1, 5.4.
func TestIntegration_ArxivPDFDownload_HappyPath(t *testing.T) {
	t.Parallel()

	t.Run("emits N progress events and one summary, status matches stream", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
		entries := []paper.Entry{
			{Source: paper.SourceArxiv, SourceID: "2404.00001", Version: "v1",
				Title: "Paper A", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/a"},
			{Source: paper.SourceArxiv, SourceID: "2404.00002", Version: "v1",
				Title: "Paper B", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/b"},
			{Source: paper.SourceArxiv, SourceID: "2404.00003", Version: "v1",
				Title: "Paper C", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/c"},
		}
		want := len(entries)

		frames, snapshot := runScenario(t, entries, "")

		var (
			progressCount int
			summary       paperctrl.DownloadSummaryEventDTO
			gotSummary    bool
		)
		for _, f := range frames {
			switch f.Event {
			case paperctrl.EventDownloadProgress:
				progressCount++
				var p paperctrl.DownloadProgressEventDTO
				if err := json.Unmarshal([]byte(f.Data), &p); err != nil {
					t.Fatalf("decode progress: %v", err)
				}
				if p.Status != "success" {
					t.Errorf("progress status = %q want success", p.Status)
				}
				if p.Bytes != len(smallPDF) {
					t.Errorf("progress bytes = %d want %d", p.Bytes, len(smallPDF))
				}
			case paperctrl.EventDownloadSummary:
				if err := json.Unmarshal([]byte(f.Data), &summary); err != nil {
					t.Fatalf("decode summary: %v", err)
				}
				gotSummary = true
			}
		}
		if progressCount != want {
			t.Errorf("progress event count = %d want %d (frames: %+v)", progressCount, want, frames)
		}
		if !gotSummary {
			t.Fatal("missing download.summary frame")
		}
		if summary.Total != want {
			t.Errorf("summary.total = %d want %d", summary.Total, want)
		}
		if summary.Succeeded != want {
			t.Errorf("summary.succeeded = %d want %d", summary.Succeeded, want)
		}
		if summary.Failed != 0 {
			t.Errorf("summary.failed = %d want 0", summary.Failed)
		}
		if frames[len(frames)-1].Event != paperctrl.EventDownloadSummary {
			t.Errorf("last frame = %q want %s", frames[len(frames)-1].Event, paperctrl.EventDownloadSummary)
		}

		// Status snapshot must be byte-consistent with the stream
		// (R5.4): same totals, same per-entry statuses.
		if !snapshot.Completed {
			t.Errorf("status.completed = false want true")
		}
		if snapshot.Total != want {
			t.Errorf("status.total = %d want %d", snapshot.Total, want)
		}
		if snapshot.Succeeded != want {
			t.Errorf("status.succeeded = %d want %d", snapshot.Succeeded, want)
		}
		if snapshot.Failed != 0 {
			t.Errorf("status.failed = %d want 0", snapshot.Failed)
		}
		if len(snapshot.Entries) != want {
			t.Fatalf("status.entries len = %d want %d", len(snapshot.Entries), want)
		}
		for i, e := range snapshot.Entries {
			if e.Status != "success" {
				t.Errorf("status.entries[%d].status = %q want success", i, e.Status)
			}
		}
	})
}

// TestIntegration_ArxivPDFDownload_FailureVariant verifies failure
// isolation: one URL returns 500 and the rest succeed. Exactly one entry
// must surface status=failed, category=fetch, and the rest success — both
// in the SSE stream and the status endpoint.
//
// Requirements covered: 3.2, 3.3, 4.2, 5.1, 5.4.
func TestIntegration_ArxivPDFDownload_FailureVariant(t *testing.T) {
	t.Parallel()

	t.Run("one 500 produces one failed/fetch entry, rest succeed", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
		entries := []paper.Entry{
			{Source: paper.SourceArxiv, SourceID: "2404.10001", Version: "v1",
				Title: "Paper A", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/a"},
			{Source: paper.SourceArxiv, SourceID: "2404.10002", Version: "v1",
				Title: "Paper B (will fail)", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/fail"},
			{Source: paper.SourceArxiv, SourceID: "2404.10003", Version: "v1",
				Title: "Paper C", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/c"},
		}

		frames, snapshot := runScenario(t, entries, "/pdf/fail")

		var (
			gotFailed     int
			gotSucceeded  int
			summary       paperctrl.DownloadSummaryEventDTO
			gotSummary    bool
			progressOrder []string
		)
		for _, f := range frames {
			switch f.Event {
			case paperctrl.EventDownloadProgress:
				var p paperctrl.DownloadProgressEventDTO
				if err := json.Unmarshal([]byte(f.Data), &p); err != nil {
					t.Fatalf("decode progress: %v", err)
				}
				progressOrder = append(progressOrder, p.Status)
				switch p.Status {
				case "failed":
					gotFailed++
					if p.Category != "fetch" {
						t.Errorf("failed entry category = %q want fetch", p.Category)
					}
					if p.Description == "" {
						t.Errorf("failed entry description must not be empty")
					}
				case "success":
					gotSucceeded++
				default:
					t.Errorf("unexpected progress status %q", p.Status)
				}
			case paperctrl.EventDownloadSummary:
				if err := json.Unmarshal([]byte(f.Data), &summary); err != nil {
					t.Fatalf("decode summary: %v", err)
				}
				gotSummary = true
			}
		}
		if gotFailed != 1 {
			t.Errorf("failed count = %d want 1 (order: %+v)", gotFailed, progressOrder)
		}
		if gotSucceeded != 2 {
			t.Errorf("succeeded count = %d want 2 (order: %+v)", gotSucceeded, progressOrder)
		}
		if !gotSummary {
			t.Fatal("missing summary frame")
		}
		if summary.Failed != 1 {
			t.Errorf("summary.failed = %d want 1", summary.Failed)
		}
		if summary.Succeeded != 2 {
			t.Errorf("summary.succeeded = %d want 2", summary.Succeeded)
		}
		if summary.Total != 3 {
			t.Errorf("summary.total = %d want 3", summary.Total)
		}

		// Status snapshot mirrors the stream exactly (R5.4).
		if snapshot.Failed != 1 {
			t.Errorf("status.failed = %d want 1", snapshot.Failed)
		}
		if snapshot.Succeeded != 2 {
			t.Errorf("status.succeeded = %d want 2", snapshot.Succeeded)
		}
		if len(snapshot.Entries) != 3 {
			t.Fatalf("status.entries len = %d want 3", len(snapshot.Entries))
		}
		var statusFailedCount int
		for _, e := range snapshot.Entries {
			if e.Status == "failed" {
				statusFailedCount++
				if e.Category != "fetch" {
					t.Errorf("status failed entry category = %q want fetch", e.Category)
				}
			}
		}
		if statusFailedCount != 1 {
			t.Errorf("status failed entry count = %d want 1", statusFailedCount)
		}
	})
}
