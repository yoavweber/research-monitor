//go:build integration

package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

// smallPDF is the minimal body served for the success paths. Real PDF
// validity is irrelevant for this slice — the local pdf.Store treats any
// non-empty 2xx response body as success and stat()s the on-disk file for
// byte count.
var smallPDF = []byte("%PDF-1.4\n%small stub\n%%EOF\n")

// sseEvent is one parsed SSE frame: the value after `event: ` and the
// concatenated value after `data: ` lines. The terminal empty line ends a
// frame.
type sseEvent struct {
	Event string
	Data  string
}

// readSSEUntilSummary consumes the SSE body until a `download.summary`
// frame arrives or the stream closes. Returns the ordered list of frames
// seen. A deadline-bound context makes the test fail loudly on a hang
// rather than blocking the test runner.
func readSSEUntilSummary(t *testing.T, body io.Reader) []sseEvent {
	t.Helper()
	scanner := bufio.NewScanner(body)
	// Bump buffer cap; SSE frames carry small JSON, but the default 64K
	// scanner buffer is generous enough already — explicit set guards
	// future expansion of the data payload.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	var (
		frames    []sseEvent
		curEvent  string
		curDataSB strings.Builder
	)
	flush := func() {
		if curEvent == "" && curDataSB.Len() == 0 {
			return
		}
		frames = append(frames, sseEvent{Event: curEvent, Data: curDataSB.String()})
		curEvent = ""
		curDataSB.Reset()
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			if len(frames) > 0 && frames[len(frames)-1].Event == "download.summary" {
				return frames
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			curEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if curDataSB.Len() > 0 {
				curDataSB.WriteByte('\n')
			}
			curDataSB.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	return frames
}

// runScenario drives the full happy-path / failure-variant flow. The path
// shape ("/pdf/<sourceid>") is fixed; pickFailingPath, if non-empty,
// selects exactly one path whose server response is HTTP 500.
//
// The function returns the captured SSE frames and the final JSON status
// snapshot so each scenario asserts whichever properties it cares about.
func runScenario(
	t *testing.T,
	entries []paper.Entry,
	pickFailingPath string,
) ([]sseEvent, map[string]any) {
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

	var fetchBody struct {
		Data struct {
			Job struct {
				JobID string `json:"job_id"`
				Total int    `json:"total"`
			} `json:"job"`
		} `json:"data"`
	}
	if err := json.NewDecoder(fetchResp.Body).Decode(&fetchBody); err != nil {
		t.Fatalf("decode fetch body: %v", err)
	}
	jobID := fetchBody.Data.Job.JobID
	if jobID == "" {
		t.Fatalf("fetch response missing data.job.job_id; body: %+v", fetchBody)
	}

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

	frames := readSSEUntilSummary(t, streamResp.Body)

	// Status snapshot.
	statusResp := doAuthenticatedGet(t, env.Server.URL+"/api/arxiv/downloads/"+jobID)
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint = %d want 200", statusResp.StatusCode)
	}
	var statusBody map[string]any
	if err := json.NewDecoder(statusResp.Body).Decode(&statusBody); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return frames, statusBody
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

		frames, statusBody := runScenario(t, entries, "")

		var (
			progressCount int
			summary       map[string]any
		)
		for _, f := range frames {
			switch f.Event {
			case "download.progress":
				progressCount++
				var d map[string]any
				if err := json.Unmarshal([]byte(f.Data), &d); err != nil {
					t.Fatalf("decode progress: %v", err)
				}
				if got := d["status"]; got != "success" {
					t.Errorf("progress status = %v want success", got)
				}
				if got, _ := d["bytes"].(float64); int(got) != len(smallPDF) {
					t.Errorf("progress bytes = %v want %d", d["bytes"], len(smallPDF))
				}
			case "download.summary":
				if err := json.Unmarshal([]byte(f.Data), &summary); err != nil {
					t.Fatalf("decode summary: %v", err)
				}
			}
		}
		if progressCount != want {
			t.Errorf("progress event count = %d want %d (frames: %+v)", progressCount, want, frames)
		}
		if summary == nil {
			t.Fatal("missing download.summary frame")
		}
		if got, _ := summary["total"].(float64); int(got) != want {
			t.Errorf("summary.total = %v want %d", summary["total"], want)
		}
		if got, _ := summary["succeeded"].(float64); int(got) != want {
			t.Errorf("summary.succeeded = %v want %d", summary["succeeded"], want)
		}
		if got, _ := summary["failed"].(float64); int(got) != 0 {
			t.Errorf("summary.failed = %v want 0", summary["failed"])
		}
		if frames[len(frames)-1].Event != "download.summary" {
			t.Errorf("last frame = %q want download.summary", frames[len(frames)-1].Event)
		}

		// Status snapshot must be byte-consistent with the stream
		// (R5.4): same totals, same per-entry statuses.
		data, _ := statusBody["data"].(map[string]any)
		if data == nil {
			t.Fatalf("status body missing data: %+v", statusBody)
		}
		if got, _ := data["completed"].(bool); !got {
			t.Errorf("status.completed = %v want true", data["completed"])
		}
		if got, _ := data["total"].(float64); int(got) != want {
			t.Errorf("status.total = %v want %d", data["total"], want)
		}
		if got, _ := data["succeeded"].(float64); int(got) != want {
			t.Errorf("status.succeeded = %v want %d", data["succeeded"], want)
		}
		if got, _ := data["failed"].(float64); int(got) != 0 {
			t.Errorf("status.failed = %v want 0", data["failed"])
		}
		statusEntries, _ := data["entries"].([]any)
		if len(statusEntries) != want {
			t.Fatalf("status.entries len = %d want %d", len(statusEntries), want)
		}
		for i, raw := range statusEntries {
			e, _ := raw.(map[string]any)
			if got, _ := e["status"].(string); got != "success" {
				t.Errorf("status.entries[%d].status = %q want success", i, got)
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

		frames, statusBody := runScenario(t, entries, "/pdf/fail")

		var (
			gotFailed     int
			gotSucceeded  int
			summary       map[string]any
			progressOrder []string
		)
		for _, f := range frames {
			if f.Event != "download.progress" && f.Event != "download.summary" {
				continue
			}
			var d map[string]any
			if err := json.Unmarshal([]byte(f.Data), &d); err != nil {
				t.Fatalf("decode %s: %v", f.Event, err)
			}
			if f.Event == "download.summary" {
				summary = d
				continue
			}

			status, _ := d["status"].(string)
			progressOrder = append(progressOrder, status)
			switch status {
			case "failed":
				gotFailed++
				if cat, _ := d["category"].(string); cat != "fetch" {
					t.Errorf("failed entry category = %q want fetch", cat)
				}
				if desc, _ := d["description"].(string); desc == "" {
					t.Errorf("failed entry description must not be empty")
				}
			case "success":
				gotSucceeded++
			default:
				t.Errorf("unexpected progress status %q", status)
			}
		}
		if gotFailed != 1 {
			t.Errorf("failed count = %d want 1 (order: %+v)", gotFailed, progressOrder)
		}
		if gotSucceeded != 2 {
			t.Errorf("succeeded count = %d want 2 (order: %+v)", gotSucceeded, progressOrder)
		}
		if summary == nil {
			t.Fatal("missing summary frame")
		}
		if got, _ := summary["failed"].(float64); int(got) != 1 {
			t.Errorf("summary.failed = %v want 1", summary["failed"])
		}
		if got, _ := summary["succeeded"].(float64); int(got) != 2 {
			t.Errorf("summary.succeeded = %v want 2", summary["succeeded"])
		}
		if got, _ := summary["total"].(float64); int(got) != 3 {
			t.Errorf("summary.total = %v want 3", summary["total"])
		}

		// Status snapshot mirrors the stream exactly (R5.4).
		data, _ := statusBody["data"].(map[string]any)
		if data == nil {
			t.Fatalf("status missing data: %+v", statusBody)
		}
		if got, _ := data["failed"].(float64); int(got) != 1 {
			t.Errorf("status.failed = %v want 1", data["failed"])
		}
		if got, _ := data["succeeded"].(float64); int(got) != 2 {
			t.Errorf("status.succeeded = %v want 2", data["succeeded"])
		}
		statusEntries, _ := data["entries"].([]any)
		if len(statusEntries) != 3 {
			t.Fatalf("status.entries len = %d want 3", len(statusEntries))
		}
		var statusFailedCount int
		for _, raw := range statusEntries {
			e, _ := raw.(map[string]any)
			if got, _ := e["status"].(string); got == "failed" {
				statusFailedCount++
				if cat, _ := e["category"].(string); cat != "fetch" {
					t.Errorf("status failed entry category = %q want fetch", cat)
				}
			}
		}
		if statusFailedCount != 1 {
			t.Errorf("status failed entry count = %d want 1", statusFailedCount)
		}
	})
}
