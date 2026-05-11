//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

// roundRobinFetcher is a minimal paper.Fetcher test double that returns a
// different batch of entries per call by indexing Batches with a monotonic
// counter. Required because the production arxiv use case only schedules
// IsNew entries — a static fetcher would yield zero IsNew rows on every
// call after the first, defeating the two-concurrent-job scenario.
type roundRobinFetcher struct {
	mu      sync.Mutex
	Batches [][]paper.Entry
	calls   int
}

// Fetch satisfies paper.Fetcher. Returns the Nth batch (modulo len) on the
// Nth call. Guarded by a mutex so concurrent fetch handlers do not race on
// the call counter.
func (f *roundRobinFetcher) Fetch(_ context.Context, _ paper.Query) ([]paper.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.Batches) == 0 {
		return nil, nil
	}
	batch := f.Batches[f.calls%len(f.Batches)]
	f.calls++
	// Return a shallow copy so the production code path that mutates
	// PDFURL via the harness's URL rewrite (if any) doesn't leak across
	// calls.
	out := make([]paper.Entry, len(batch))
	copy(out, batch)
	return out, nil
}

// fireFetch issues a single authenticated GET /api/arxiv/fetch against
// baseURL and returns the parsed job_id and the source_id list the caller
// uses to identify which batch each job covers. source_ids are read from
// the embedded job snapshot's `entries[].paper_id.source_id`, which the
// arxiv use case populates from the IsNew subset (R2.2).
func fireFetch(t *testing.T, baseURL string) (jobID string, sourceIDs []string) {
	t.Helper()
	resp := doAuthenticatedGet(t, baseURL+"/api/arxiv/fetch")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/arxiv/fetch status = %d want 200", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Job struct {
				JobID   string `json:"job_id"`
				Entries []struct {
					PaperID struct {
						SourceID string `json:"source_id"`
					} `json:"paper_id"`
				} `json:"entries"`
			} `json:"job"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode fetch body: %v", err)
	}
	if body.Data.Job.JobID == "" {
		t.Fatalf("fetch missing job.job_id: %+v", body)
	}
	for _, e := range body.Data.Job.Entries {
		sourceIDs = append(sourceIDs, e.PaperID.SourceID)
	}
	return body.Data.Job.JobID, sourceIDs
}

// progressSourceID extracts the source_id from a download.progress event
// payload. The wire shape is `{ "paper_id": { "source_id": "..." }, ... }`.
func progressSourceID(t *testing.T, data string) string {
	t.Helper()
	var d struct {
		PaperID struct {
			SourceID string `json:"source_id"`
		} `json:"paper_id"`
	}
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		t.Fatalf("decode progress payload: %v", err)
	}
	return d.PaperID.SourceID
}

// TestIntegration_ArxivPDFDownload_Concurrency covers task 5.2: concurrent
// job isolation and slow-subscriber safety.
//
// Requirements: 2.4 (concurrent jobs independent), 4.4 (slow subscriber
// dropped without affecting underlying job or other subscribers), 5.4
// (status endpoint consistent with stream even after subscriber drop).
func TestIntegration_ArxivPDFDownload_Concurrency(t *testing.T) {
	t.Parallel()

	t.Run("two concurrent fetches produce isolated streams", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
		// Two disjoint batches. The round-robin fetcher returns batchA on
		// call 1 and batchB on call 2 (the harness shares one fetcher
		// across both concurrent /api/arxiv/fetch invocations). All entries
		// have distinct SourceIDs across batches so IsNew is true for every
		// row in every fetch — required for the scheduler to enqueue work.
		batchA := []paper.Entry{
			{Source: paper.SourceArxiv, SourceID: "2404.0A001", Version: "v1",
				Title: "A1", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/a1"},
			{Source: paper.SourceArxiv, SourceID: "2404.0A002", Version: "v1",
				Title: "A2", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/a2"},
			{Source: paper.SourceArxiv, SourceID: "2404.0A003", Version: "v1",
				Title: "A3", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/a3"},
		}
		batchB := []paper.Entry{
			{Source: paper.SourceArxiv, SourceID: "2404.0B001", Version: "v1",
				Title: "B1", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/b1"},
			{Source: paper.SourceArxiv, SourceID: "2404.0B002", Version: "v1",
				Title: "B2", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/b2"},
			{Source: paper.SourceArxiv, SourceID: "2404.0B003", Version: "v1",
				Title: "B3", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/b3"},
		}

		// One in-process httptest.Server serves every PDF URL referenced by
		// either batch. The body is a tiny stub so success rows materialize
		// without real PDF parsing concerns.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(smallPDF)
		}))
		t.Cleanup(srv.Close)

		// Rewrite PDFURLs so they hit the test server. Must happen before
		// the fetcher hands the entries out.
		for i := range batchA {
			batchA[i].PDFURL = srv.URL + batchA[i].PDFURL
		}
		for i := range batchB {
			batchB[i].PDFURL = srv.URL + batchB[i].PDFURL
		}

		fetcher := &roundRobinFetcher{Batches: [][]paper.Entry{batchA, batchB}}
		env := setup.SetupTestEnv(t, setup.TestEnvOpts{
			ArxivFetcher:    fetcher,
			ArxivQuery:      paper.Query{Categories: []string{"cs.LG"}, MaxResults: 100},
			WirePDFDownload: true,
		})
		t.Cleanup(env.Close)

		// Fire two fetches concurrently. The single shared registry must
		// allocate two distinct job IDs and run their workers in parallel.
		type fetchOut struct {
			jobID     string
			sourceIDs []string
		}
		results := make([]fetchOut, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		for i := 0; i < 2; i++ {
			go func(idx int) {
				defer wg.Done()
				id, srcIDs := fireFetch(t, env.Server.URL)
				results[idx] = fetchOut{jobID: id, sourceIDs: srcIDs}
			}(i)
		}
		wg.Wait()

		jobA, jobB := results[0], results[1]
		if jobA.jobID == jobB.jobID {
			t.Fatalf("concurrent fetches reused job_id %q (must differ per R2.4)", jobA.jobID)
		}
		if len(jobA.sourceIDs) == 0 || len(jobB.sourceIDs) == 0 {
			t.Fatalf("expected both jobs to schedule entries; got A=%v B=%v",
				jobA.sourceIDs, jobB.sourceIDs)
		}
		// The two batches are disjoint, so the source_id sets must not
		// intersect.
		setB := map[string]struct{}{}
		for _, s := range jobB.sourceIDs {
			setB[s] = struct{}{}
		}
		for _, s := range jobA.sourceIDs {
			if _, dup := setB[s]; dup {
				t.Fatalf("source_id %q appeared in both jobs; round-robin batch leak", s)
			}
		}

		// Open both streams concurrently. Each stream must observe only
		// the source_ids that belong to its own job (R4.4 / R2.4) and
		// reach a download.summary.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		type streamOut struct {
			frames []sseEvent
			err    error
		}
		streams := make([]streamOut, 2)
		var sw sync.WaitGroup
		sw.Add(2)
		for i, job := range []fetchOut{jobA, jobB} {
			go func(idx int, job fetchOut) {
				defer sw.Done()
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
					env.Server.URL+"/api/arxiv/downloads/"+job.jobID+"/stream", nil)
				req.Header.Set(middleware.APITokenHeader, setup.TestToken)
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					streams[idx] = streamOut{err: err}
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					streams[idx] = streamOut{err: fmt.Errorf("stream status %d", resp.StatusCode)}
					return
				}
				streams[idx] = streamOut{frames: readSSEUntilSummary(t, resp.Body)}
			}(i, job)
		}
		sw.Wait()

		for i, job := range []fetchOut{jobA, jobB} {
			s := streams[i]
			if s.err != nil {
				t.Fatalf("job %s stream error: %v", job.jobID, s.err)
			}
			expected := map[string]struct{}{}
			for _, sid := range job.sourceIDs {
				expected[sid] = struct{}{}
			}
			seenProgress := map[string]int{}
			var summary map[string]any
			for _, f := range s.frames {
				switch f.Event {
				case "download.progress":
					sid := progressSourceID(t, f.Data)
					if _, ok := expected[sid]; !ok {
						t.Errorf("job %s stream received cross-talk: source_id %q not in own batch %v",
							job.jobID, sid, job.sourceIDs)
					}
					seenProgress[sid]++
				case "download.summary":
					if err := json.Unmarshal([]byte(f.Data), &summary); err != nil {
						t.Fatalf("decode summary: %v", err)
					}
				}
			}
			if summary == nil {
				t.Fatalf("job %s stream did not reach download.summary; frames=%+v",
					job.jobID, s.frames)
			}
			if got, _ := summary["job_id"].(string); got != job.jobID {
				t.Errorf("job %s summary.job_id = %q want %q", job.jobID, got, job.jobID)
			}
			if got, _ := summary["total"].(float64); int(got) != len(job.sourceIDs) {
				t.Errorf("job %s summary.total = %v want %d",
					job.jobID, summary["total"], len(job.sourceIDs))
			}
			if len(seenProgress) != len(expected) {
				t.Errorf("job %s saw %d distinct progress source_ids want %d (seen=%v)",
					job.jobID, len(seenProgress), len(expected), seenProgress)
			}
			if s.frames[len(s.frames)-1].Event != "download.summary" {
				t.Errorf("job %s last frame = %q want download.summary",
					job.jobID, s.frames[len(s.frames)-1].Event)
			}
		}
	})

	t.Run("slow subscriber is dropped but worker completes and status stays correct", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2024, 4, 2, 10, 0, 0, 0, time.UTC)
		entries := []paper.Entry{
			{Source: paper.SourceArxiv, SourceID: "2404.0S001", Version: "v1",
				Title: "S1", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/s1"},
			{Source: paper.SourceArxiv, SourceID: "2404.0S002", Version: "v1",
				Title: "S2", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/s2"},
			{Source: paper.SourceArxiv, SourceID: "2404.0S003", Version: "v1",
				Title: "S3", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/s3"},
			{Source: paper.SourceArxiv, SourceID: "2404.0S004", Version: "v1",
				Title: "S4", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/s4"},
			{Source: paper.SourceArxiv, SourceID: "2404.0S005", Version: "v1",
				Title: "S5", SubmittedAt: now, UpdatedAt: now, PDFURL: "/pdf/s5"},
		}
		want := len(entries)

		// Each PDF GET is held by a per-request gate the test releases in
		// sequence. This widens the worker's progress window beyond the
		// test's stream-open round trip so the slow-subscriber path is
		// actually exercised (a freely-running worker finishes 5 in-process
		// fetches in single-digit milliseconds — too tight to reliably
		// attach a subscriber mid-flight).
		release := make(chan struct{}, want)
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

		fetcher := &roundRobinFetcher{Batches: [][]paper.Entry{entries}}
		env := setup.SetupTestEnv(t, setup.TestEnvOpts{
			ArxivFetcher: fetcher,
			ArxivQuery:   paper.Query{Categories: []string{"cs.LG"}, MaxResults: 100},
			// Tiny buffer guarantees the non-blocking-send fan-out hits
			// the slow-subscriber-drop path quickly: as soon as the
			// worker enqueues a second progress event while the client
			// has not drained the first, the registry closes the
			// subscriber channel.
			WirePDFDownload:          true,
			PDFDownloadSubscriberBuf: 1,
		})
		t.Cleanup(env.Close)

		jobID, scheduledIDs := fireFetch(t, env.Server.URL)
		if len(scheduledIDs) != want {
			t.Fatalf("scheduled %d entries want %d", len(scheduledIDs), want)
		}

		// Open the SSE stream while the worker is gated. The controller
		// flushes response headers only after writing the first event,
		// so http.Client.Do blocks until the worker emits something.
		// We issue the request in a goroutine, then release the first
		// gate; that produces the first progress event which unblocks
		// Do. From that point on the test never reads the body — the
		// per-subscriber channel (buffer=1) overflows as soon as the
		// worker emits a second event the client has not drained, and
		// the registry closes the subscriber channel (R4.4 drop
		// policy).
		streamCtx, streamCancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer streamCancel()
		streamReq, _ := http.NewRequestWithContext(streamCtx, http.MethodGet,
			env.Server.URL+"/api/arxiv/downloads/"+jobID+"/stream", nil)
		streamReq.Header.Set(middleware.APITokenHeader, setup.TestToken)

		type streamResult struct {
			resp *http.Response
			err  error
		}
		streamCh := make(chan streamResult, 1)
		go func() {
			resp, err := http.DefaultClient.Do(streamReq)
			streamCh <- streamResult{resp: resp, err: err}
		}()

		// Release the first PDF fetch so the worker emits the first
		// progress event; the controller flushes it which unblocks the
		// stream Do call.
		release <- struct{}{}

		var streamResp *http.Response
		select {
		case sr := <-streamCh:
			if sr.err != nil {
				t.Fatalf("slow stream request: %v", sr.err)
			}
			streamResp = sr.resp
		case <-time.After(5 * time.Second):
			t.Fatalf("slow stream Do never returned after first event")
		}
		t.Cleanup(func() { _ = streamResp.Body.Close() })
		if streamResp.StatusCode != http.StatusOK {
			t.Fatalf("slow stream status = %d want 200", streamResp.StatusCode)
		}

		// Release the remaining PDF fetches so the worker emits
		// progress events back-to-back. The non-blocking fan-out into
		// the slow client's 1-buffer channel overflows quickly and the
		// registry closes that channel. Meanwhile the worker keeps
		// writing per-entry results into the registry state, which the
		// status endpoint reflects (R5.4).
		for i := 1; i < want; i++ {
			release <- struct{}{}
		}

		// Poll the status endpoint until the worker completes. The
		// worker MUST make progress despite the wedged subscriber.
		deadline := time.Now().Add(5 * time.Second)
		var finalStatus map[string]any
		for {
			if time.Now().After(deadline) {
				t.Fatalf("status endpoint never reported completed=true: last=%+v", finalStatus)
			}
			resp := doAuthenticatedGet(t, env.Server.URL+"/api/arxiv/downloads/"+jobID)
			func() {
				defer resp.Body.Close()
				if err := json.NewDecoder(resp.Body).Decode(&finalStatus); err != nil {
					t.Fatalf("decode status: %v", err)
				}
			}()
			data, _ := finalStatus["data"].(map[string]any)
			if data != nil {
				if done, _ := data["completed"].(bool); done {
					break
				}
			}
			// Short bounded poll; the worker drives PDFs against an
			// in-process httptest.Server so latency is sub-millisecond.
			time.Sleep(25 * time.Millisecond)
		}

		// Status snapshot must report correct totals and per-entry
		// results despite the dropped subscriber (R5.4 / R4.4).
		data, _ := finalStatus["data"].(map[string]any)
		if data == nil {
			t.Fatalf("status body missing data: %+v", finalStatus)
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
		seenIDs := map[string]string{}
		for i, raw := range statusEntries {
			e, _ := raw.(map[string]any)
			pid, _ := e["paper_id"].(map[string]any)
			if pid == nil {
				t.Errorf("status.entries[%d] missing paper_id: %+v", i, e)
				continue
			}
			sid, _ := pid["source_id"].(string)
			st, _ := e["status"].(string)
			seenIDs[sid] = st
			if st != "success" {
				t.Errorf("status.entries[%d].status = %q want success", i, st)
			}
			if got, _ := e["bytes"].(float64); int(got) != len(smallPDF) {
				t.Errorf("status.entries[%d].bytes = %v want %d", i, e["bytes"], len(smallPDF))
			}
		}
		for _, want := range scheduledIDs {
			if _, ok := seenIDs[want]; !ok {
				t.Errorf("status missing scheduled source_id %q", want)
			}
		}

		// The slow client's response body must eventually close —
		// either because the registry dropped the subscriber and the
		// controller emitted a terminal `error` frame, or because the
		// worker delivered the Summary before the buffer overflowed
		// (the design treats either ordering as acceptable). Drain to
		// EOF under a bounded deadline; an indefinite hang here would
		// indicate the controller is still subscribed.
		drainDone := make(chan error, 1)
		go func() {
			_, copyErr := io.Copy(io.Discard, streamResp.Body)
			drainDone <- copyErr
		}()
		select {
		case <-drainDone:
			// stream closed cleanly (EOF or context cancel both fine)
		case <-time.After(5 * time.Second):
			t.Fatalf("slow subscriber stream did not close within deadline; controller may have leaked subscription")
		}

		// Optional sanity check: a fresh subscriber after completion
		// must see the full backlog (5 progress + 1 summary) and the
		// stream must close after the summary. This proves that the
		// dropped slow subscriber did not corrupt the registry's event
		// log (R4.4 / R5.4).
		replayCtx, replayCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer replayCancel()
		replayReq, _ := http.NewRequestWithContext(replayCtx, http.MethodGet,
			env.Server.URL+"/api/arxiv/downloads/"+jobID+"/stream", nil)
		replayReq.Header.Set(middleware.APITokenHeader, setup.TestToken)
		replayResp, err := http.DefaultClient.Do(replayReq)
		if err != nil {
			t.Fatalf("replay stream request: %v", err)
		}
		defer replayResp.Body.Close()
		if replayResp.StatusCode != http.StatusOK {
			t.Fatalf("replay stream status = %d want 200", replayResp.StatusCode)
		}
		if ct := replayResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
			t.Fatalf("replay stream content-type = %q", ct)
		}
		replayFrames := readSSEUntilSummary(t, replayResp.Body)
		var (
			replayProgress int
			replaySummary  bool
		)
		for _, f := range replayFrames {
			switch f.Event {
			case "download.progress":
				replayProgress++
			case "download.summary":
				replaySummary = true
			}
		}
		if replayProgress != want {
			t.Errorf("replay progress count = %d want %d", replayProgress, want)
		}
		if !replaySummary {
			t.Errorf("replay stream missing download.summary frame")
		}
	})
}
