//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

// TestIntegration_ArxivPDFDownload_Retention covers task 5.3:
// retention-window eviction of completed jobs and in-progress-job
// immunity against the same sweep.
//
// Both subtests drive the registry's Sweep(ctx, now) directly (the
// concrete *Registry is exposed on TestEnv.PDFDownloadRegistry). Sweep
// takes the "now" instant as a parameter, so the test picks a value far
// past completedAt + Retention to force eviction deterministically —
// no clock injection or harness changes needed.
//
// Requirements covered:
//   - 6.1 In-progress jobs retained regardless of age.
//   - 6.2 Completed jobs retained for at least the retention window.
//   - 6.3 After eviction, status and stream return unknown (404).
//   - 6.5 Eviction is part of the lifecycle that gets logged
//     (pdfdownload.job.evicted in the registry's logger). The
//     wire-level 404 from both endpoints is the load-bearing
//     observable here; the unit tests in task 2.5 prove the log key.
func TestIntegration_ArxivPDFDownload_Retention(t *testing.T) {
	t.Parallel()

	t.Run("completed job past retention yields 404 on both endpoints", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2024, 4, 3, 10, 0, 0, 0, time.UTC)
		entries := []paper.Entry{
			{Source: paper.SourceArxiv, SourceID: "2404.0R001", Version: "v1",
				Title: "R1", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/r1"},
			{Source: paper.SourceArxiv, SourceID: "2404.0R002", Version: "v1",
				Title: "R2", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/r2"},
		}

		// Trivial PDF server: no gating, every entry succeeds quickly so
		// the job reaches `completed: true` well before the test's poll
		// deadline.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(smallPDF)
		}))
		t.Cleanup(srv.Close)

		for i := range entries {
			entries[i].PDFURL = srv.URL + entries[i].PDFURL
		}

		fetcher := &mocks.PaperFetcher{Batches: [][]paper.Entry{entries}}
		retention := 100 * time.Millisecond
		env := setup.SetupTestEnv(t, setup.TestEnvOpts{
			ArxivFetcher: fetcher,
			ArxivQuery:   paper.Query{Categories: []string{"cs.LG"}, MaxResults: 100},
			// Retention is supplied for documentation; the actual eviction
			// timing comes from the `now` we hand to Sweep, not from wall
			// clock advance.
			WirePDFDownload:      true,
			PDFDownloadRetention: retention,
		})
		t.Cleanup(env.Close)

		jobID, scheduledIDs := fireFetch(t, env.Server.URL)
		if len(scheduledIDs) != len(entries) {
			t.Fatalf("scheduled %d entries want %d", len(scheduledIDs), len(entries))
		}

		// Wait for the job to reach completed=true via the status
		// endpoint. Bounded so a worker hang fails the test rather than
		// stalling the runner.
		waitForCompleted(t, env.Server.URL, jobID, 5*time.Second)

		// Force eviction by driving Sweep with a `now` that is comfortably
		// beyond completedAt + Retention. The registry's job uses wall-
		// clock SystemClock to stamp completedAt, so we anchor on
		// time.Now() and add a window large enough to exceed retention
		// regardless of small clock skew during test execution.
		if env.PDFDownloadRegistry == nil {
			t.Fatal("PDFDownloadRegistry not exposed by harness")
		}
		env.PDFDownloadRegistry.Sweep(context.Background(), time.Now().Add(10*time.Minute))

		// Status endpoint: must now return 404 + standard error envelope
		// (R6.3 + ErrorEnvelope middleware shape).
		statusResp := doAuthenticatedGet(t, env.Server.URL+"/api/arxiv/downloads/"+jobID)
		defer statusResp.Body.Close()
		if statusResp.StatusCode != http.StatusNotFound {
			t.Fatalf("status after eviction = %d want %d", statusResp.StatusCode, http.StatusNotFound)
		}
		assertErrorEnvelope(t, statusResp, http.StatusNotFound)

		// Stream endpoint: must also surface 404 + envelope BEFORE any
		// SSE frame is written. The controller wraps
		// paper.ErrDownloadJobUnknown into shared.HTTPError(404,
		// "download job unknown"), so the response body is JSON, not
		// text/event-stream.
		streamCtx, streamCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer streamCancel()
		streamReq, _ := http.NewRequestWithContext(streamCtx, http.MethodGet,
			env.Server.URL+"/api/arxiv/downloads/"+jobID+"/stream", nil)
		streamReq.Header.Set(middleware.APITokenHeader, setup.TestToken)
		streamResp, err := http.DefaultClient.Do(streamReq)
		if err != nil {
			t.Fatalf("stream request: %v", err)
		}
		defer streamResp.Body.Close()
		if streamResp.StatusCode != http.StatusNotFound {
			t.Fatalf("stream after eviction = %d want %d", streamResp.StatusCode, http.StatusNotFound)
		}
		assertErrorEnvelope(t, streamResp, http.StatusNotFound)
	})

	t.Run("in-progress job is not evicted regardless of clock advance", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2024, 4, 4, 10, 0, 0, 0, time.UTC)
		entries := []paper.Entry{
			{Source: paper.SourceArxiv, SourceID: "2404.0P001", Version: "v1",
				Title: "P1", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/p1"},
			{Source: paper.SourceArxiv, SourceID: "2404.0P002", Version: "v1",
				Title: "P2", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/p2"},
		}

		// Per-entry gate: the test releases each PDF fetch one at a time
		// so the worker is reliably mid-flight when we drive Sweep with a
		// far-future `now`. R6.1 says active jobs must NOT be evicted —
		// the only way to exercise that is to keep `completed == false`
		// while we sweep.
		release := make(chan struct{}, len(entries))
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(smallPDF)
		}))
		t.Cleanup(srv.Close)

		for i := range entries {
			entries[i].PDFURL = srv.URL + entries[i].PDFURL
		}

		fetcher := &mocks.PaperFetcher{Batches: [][]paper.Entry{entries}}
		// A tiny retention window makes the assertion sharp: if the
		// in-progress branch in sweepLocked is ever buggy and tries to
		// evict, even a 50ms-old job would qualify under the 10-minute
		// `now` we hand to Sweep. The fact that the job survives proves
		// the `!completed` short-circuit in sweepLocked (R6.1).
		env := setup.SetupTestEnv(t, setup.TestEnvOpts{
			ArxivFetcher:         fetcher,
			ArxivQuery:           paper.Query{Categories: []string{"cs.LG"}, MaxResults: 100},
			WirePDFDownload:      true,
			PDFDownloadRetention: 50 * time.Millisecond,
		})
		t.Cleanup(env.Close)

		jobID, scheduledIDs := fireFetch(t, env.Server.URL)
		if len(scheduledIDs) != len(entries) {
			t.Fatalf("scheduled %d entries want %d", len(scheduledIDs), len(entries))
		}

		// Drive Sweep BEFORE releasing any gate. The job exists in the
		// registry but completed==false, so sweepLocked must skip it
		// even with `now` far past the (very small) retention window.
		if env.PDFDownloadRegistry == nil {
			t.Fatal("PDFDownloadRegistry not exposed by harness")
		}
		env.PDFDownloadRegistry.Sweep(context.Background(), time.Now().Add(10*time.Minute))

		// Status endpoint must still return 200 with completed=false.
		// This is the R6.1 observable: an active job remains reachable
		// past the retention window.
		statusResp := doAuthenticatedGet(t, env.Server.URL+"/api/arxiv/downloads/"+jobID)
		var statusBody paperctrl.JobStatusEnvelope
		func() {
			defer statusResp.Body.Close()
			if statusResp.StatusCode != http.StatusOK {
				t.Fatalf("status during in-progress = %d want 200", statusResp.StatusCode)
			}
			if err := json.NewDecoder(statusResp.Body).Decode(&statusBody); err != nil {
				t.Fatalf("decode status: %v", err)
			}
		}()
		if statusBody.Data.Completed {
			t.Errorf("in-progress status.completed = true want false")
		}
		if statusBody.Data.Total != len(entries) {
			t.Errorf("status.total = %d want %d", statusBody.Data.Total, len(entries))
		}

		// Release the gates so the worker can complete. After that, the
		// job is eligible for eviction; we don't assert eviction again
		// here — Test C already covers that path. The release here is
		// purely so the harness's shutdown drain completes cleanly
		// rather than hanging on the gated httptest.Server.
		for range entries {
			release <- struct{}{}
		}
		waitForCompleted(t, env.Server.URL, jobID, 5*time.Second)
	})
}

// waitForCompleted polls the status endpoint until data.completed is
// true or the deadline elapses. Bounded so a worker hang fails the
// test rather than stalling the runner. The poll interval is small
// (25ms) because the worker drives PDFs against an in-process
// httptest.Server with sub-millisecond latency.
func waitForCompleted(t *testing.T, baseURL, jobID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("status endpoint never reported completed=true for job %s", jobID)
		}
		resp := doAuthenticatedGet(t, baseURL+"/api/arxiv/downloads/"+jobID)
		var body paperctrl.JobStatusEnvelope
		func() {
			defer resp.Body.Close()
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode status: %v", err)
			}
		}()
		if body.Data.Completed {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}
