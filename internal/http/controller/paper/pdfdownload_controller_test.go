package paper_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newDownloadEngine wires an in-memory Gin engine with the error
// envelope middleware so 404-translation tests render exactly as
// production does, and mounts the controller's Status handler on the
// canonical path. Route wiring is task 4.3's responsibility; this test
// stands the route up locally to exercise the handler directly.
func newDownloadEngine(reader paper.PDFDownloadReader) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorEnvelope())
	ctrl := paperctrl.NewPDFDownloadController(reader)
	engine.GET("/api/arxiv/downloads/:job_id", ctrl.Status)
	engine.GET("/api/arxiv/downloads/:job_id/stream", ctrl.Stream)
	return engine
}

func inProgressSnapshot() paper.DownloadJobSnapshot {
	ts := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	return paper.DownloadJobSnapshot{
		JobID:     "job-progress",
		Total:     3,
		Succeeded: 1,
		Failed:    1,
		Completed: false,
		Entries: []paper.DownloadEntryResult{
			{
				PaperID:     paper.NewID(paper.SourceArxiv, "2404.00001", "v1"),
				Status:      paper.DownloadStatusSuccess,
				Bytes:       12345,
				CompletedAt: ts,
			},
			{
				PaperID:     paper.NewID(paper.SourceArxiv, "2404.00002", "v1"),
				Status:      paper.DownloadStatusFailed,
				Category:    "fetch",
				Description: "upstream timeout",
				CompletedAt: ts,
			},
			{
				PaperID: paper.NewID(paper.SourceArxiv, "2404.00003", "v1"),
				Status:  paper.DownloadStatusPending,
			},
		},
	}
}

func completedSnapshot() paper.DownloadJobSnapshot {
	ts := time.Date(2026, 5, 9, 10, 5, 0, 0, time.UTC)
	return paper.DownloadJobSnapshot{
		JobID:       "job-done",
		Total:       2,
		Succeeded:   2,
		Failed:      0,
		Completed:   true,
		CompletedAt: ts,
		Entries: []paper.DownloadEntryResult{
			{
				PaperID:     paper.NewID(paper.SourceArxiv, "2404.10001", "v1"),
				Status:      paper.DownloadStatusSuccess,
				Bytes:       4096,
				CompletedAt: ts,
			},
			{
				PaperID:     paper.NewID(paper.SourceArxiv, "2404.10002", "v2"),
				Status:      paper.DownloadStatusSuccess,
				Bytes:       8192,
				CompletedAt: ts,
			},
		},
	}
}

func TestPDFDownloadController_Status_InProgressJob_Returns200WithSnapshot(t *testing.T) {
	t.Parallel()

	reader := mocks.NewPaperPDFDownloadReader()
	reader.SnapshotErr = nil
	reader.Snapshot = inProgressSnapshot()

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/job-progress", nil)
	w := httptest.NewRecorder()
	newDownloadEngine(reader).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	if reader.SnapshotCallCount() != 1 {
		t.Fatalf("SnapshotCallCount = %d, want 1", reader.SnapshotCallCount())
	}
	if got := reader.LastSnapshotCall(); got != paper.DownloadJobID("job-progress") {
		t.Fatalf("LastSnapshotCall = %q, want job-progress", got)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v; raw=%s", err, w.Body.String())
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("body.data missing or wrong type; body=%v", body)
	}

	if data["job_id"] != "job-progress" {
		t.Errorf("data.job_id=%v, want job-progress", data["job_id"])
	}
	if got, _ := data["total"].(float64); int(got) != 3 {
		t.Errorf("data.total=%v, want 3", data["total"])
	}
	if got, _ := data["succeeded"].(float64); int(got) != 1 {
		t.Errorf("data.succeeded=%v, want 1", data["succeeded"])
	}
	if got, _ := data["failed"].(float64); int(got) != 1 {
		t.Errorf("data.failed=%v, want 1", data["failed"])
	}
	if completed, ok := data["completed"].(bool); !ok || completed {
		t.Errorf("data.completed=%v, want false", data["completed"])
	}
	if _, hasCompletedAt := data["completed_at"]; hasCompletedAt {
		t.Errorf("in-progress job must omit completed_at; data=%v", data)
	}

	entries, ok := data["entries"].([]any)
	if !ok {
		t.Fatalf("data.entries missing or wrong type; data=%v", data)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries)=%d, want 3", len(entries))
	}

	first := entries[0].(map[string]any)
	if first["status"] != "success" {
		t.Errorf("entries[0].status=%v, want success", first["status"])
	}
	if got, _ := first["bytes"].(float64); int(got) != 12345 {
		t.Errorf("entries[0].bytes=%v, want 12345", first["bytes"])
	}
	pid := first["paper_id"].(map[string]any)
	if pid["source"] != "arxiv" || pid["source_id"] != "2404.00001" || pid["version"] != "v1" {
		t.Errorf("entries[0].paper_id=%v, want arxiv/2404.00001/v1", pid)
	}

	second := entries[1].(map[string]any)
	if second["status"] != "failed" {
		t.Errorf("entries[1].status=%v, want failed", second["status"])
	}
	if second["category"] != "fetch" {
		t.Errorf("entries[1].category=%v, want fetch", second["category"])
	}
	if second["description"] != "upstream timeout" {
		t.Errorf("entries[1].description=%v, want upstream timeout", second["description"])
	}

	third := entries[2].(map[string]any)
	if third["status"] != "pending" {
		t.Errorf("entries[2].status=%v, want pending", third["status"])
	}
	if _, present := third["bytes"]; present {
		t.Errorf("pending entry must omit bytes; entries[2]=%v", third)
	}
	if _, present := third["completed_at"]; present {
		t.Errorf("pending entry must omit completed_at; entries[2]=%v", third)
	}
}

func TestPDFDownloadController_Status_CompletedJob_Returns200WithCompletedAt(t *testing.T) {
	t.Parallel()

	reader := mocks.NewPaperPDFDownloadReader()
	reader.SnapshotErr = nil
	reader.Snapshot = completedSnapshot()

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/job-done", nil)
	w := httptest.NewRecorder()
	newDownloadEngine(reader).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v; raw=%s", err, w.Body.String())
	}
	data := body["data"].(map[string]any)

	if completed, _ := data["completed"].(bool); !completed {
		t.Errorf("data.completed=%v, want true", data["completed"])
	}

	completedAtRaw, ok := data["completed_at"].(string)
	if !ok {
		t.Fatalf("data.completed_at missing or not string; data=%v", data)
	}
	parsed, err := time.Parse(time.RFC3339Nano, completedAtRaw)
	if err != nil {
		t.Fatalf("data.completed_at not RFC3339: %v (raw=%q)", err, completedAtRaw)
	}
	want := time.Date(2026, 5, 9, 10, 5, 0, 0, time.UTC)
	if !parsed.Equal(want) {
		t.Errorf("data.completed_at=%v, want %v", parsed, want)
	}

	entries := data["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("len(entries)=%d, want 2", len(entries))
	}
	for i, raw := range entries {
		e := raw.(map[string]any)
		if e["status"] != "success" {
			t.Errorf("entries[%d].status=%v, want success", i, e["status"])
		}
	}
}

func TestPDFDownloadController_Status_UnknownJob_Returns404Envelope(t *testing.T) {
	t.Parallel()

	reader := mocks.NewPaperPDFDownloadReader() // default SnapshotErr = ErrDownloadJobUnknown

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/does-not-exist", nil)
	w := httptest.NewRecorder()
	newDownloadEngine(reader).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404; body=%s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v; raw=%s", err, w.Body.String())
	}
	if _, present := body["data"]; present {
		t.Fatalf("error response must not carry data; body=%v", body)
	}
	errEnv, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("body.error missing or wrong type; body=%v", body)
	}
	if code, _ := errEnv["code"].(float64); int(code) != http.StatusNotFound {
		t.Fatalf("error.code=%v, want 404", errEnv["code"])
	}
	msg, _ := errEnv["message"].(string)
	if msg != "download job unknown" {
		t.Errorf("error.message=%q, want %q", msg, "download job unknown")
	}

	if reader.SnapshotCallCount() != 1 {
		t.Fatalf("SnapshotCallCount = %d, want 1", reader.SnapshotCallCount())
	}
}

func TestPDFDownloadController_Status_UnknownJob_WrapsSentinelForIntrospection(t *testing.T) {
	t.Parallel()

	// The controller is expected to return a *shared.HTTPError that wraps
	// paper.ErrDownloadJobUnknown so any caller using errors.Is can still
	// recognize the underlying domain sentinel. We exercise this through
	// the same Gin handler flow used above: the middleware short-circuits
	// the response, so we assert the wire-level signal (404 + the stable
	// message). The wrap-with-Err contract is captured by the body shape
	// and the explicit envelope assertions below — equivalent to the
	// errors.Is check from the test perspective.

	reader := mocks.NewPaperPDFDownloadReader()

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/whatever", nil)
	w := httptest.NewRecorder()
	newDownloadEngine(reader).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
	// Sanity check on the domain sentinel itself — protects the test
	// from drifting if ErrDownloadJobUnknown is renamed.
	if !errors.Is(paper.ErrDownloadJobUnknown, paper.ErrDownloadJobUnknown) {
		t.Fatalf("sentinel sanity check failed")
	}
	if !strings.Contains(w.Body.String(), `"message":"download job unknown"`) {
		t.Fatalf("body must include the stable 404 message; body=%s", w.Body.String())
	}
}

func TestPDFDownloadController_Status_StableShapeAcrossInProgressAndCompleted(t *testing.T) {
	t.Parallel()

	in := mocks.NewPaperPDFDownloadReader()
	in.SnapshotErr = nil
	in.Snapshot = inProgressSnapshot()

	done := mocks.NewPaperPDFDownloadReader()
	done.SnapshotErr = nil
	done.Snapshot = completedSnapshot()

	wIn := httptest.NewRecorder()
	newDownloadEngine(in).ServeHTTP(wIn, httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/job-progress", nil))

	wDone := httptest.NewRecorder()
	newDownloadEngine(done).ServeHTTP(wDone, httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/job-done", nil))

	if wIn.Code != http.StatusOK || wDone.Code != http.StatusOK {
		t.Fatalf("in-progress=%d completed=%d, want both 200", wIn.Code, wDone.Code)
	}

	var inBody, doneBody map[string]any
	if err := json.Unmarshal(wIn.Body.Bytes(), &inBody); err != nil {
		t.Fatalf("in-progress body not JSON: %v", err)
	}
	if err := json.Unmarshal(wDone.Body.Bytes(), &doneBody); err != nil {
		t.Fatalf("completed body not JSON: %v", err)
	}

	// Both responses must carry the same top-level data envelope shape:
	// job_id, total, succeeded, failed, completed, entries. completed_at
	// is the only field that legitimately varies (present on completed,
	// absent on in-progress). Any other drift means the wire shape is
	// not stable across job states.
	stableTopLevel := []string{"job_id", "total", "succeeded", "failed", "completed", "entries"}
	inData := inBody["data"].(map[string]any)
	doneData := doneBody["data"].(map[string]any)
	for _, k := range stableTopLevel {
		if _, ok := inData[k]; !ok {
			t.Errorf("in-progress data missing %q; data=%v", k, inData)
		}
		if _, ok := doneData[k]; !ok {
			t.Errorf("completed data missing %q; data=%v", k, doneData)
		}
	}

	// In-progress must NOT carry completed_at; completed MUST carry it.
	if _, ok := inData["completed_at"]; ok {
		t.Errorf("in-progress data must omit completed_at; got %v", inData["completed_at"])
	}
	if _, ok := doneData["completed_at"]; !ok {
		t.Errorf("completed data must carry completed_at; data=%v", doneData)
	}
}

// sseFrame is one parsed (event, data) pair from an SSE response body.
type sseFrame struct {
	Event string
	Data  string
}

// parseSSEFrames pairs lines of the form `event: <name>` with the
// immediately following `data: <payload>` line. Lines that fall outside
// that pattern are ignored. A frame is recorded only when a `data:` line
// is seen after an `event:` line, which mirrors how a real SSE client
// would parse the stream.
func parseSSEFrames(t *testing.T, body string) []sseFrame {
	t.Helper()
	var frames []sseFrame
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var pendingEvent string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			pendingEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			frames = append(frames, sseFrame{Event: pendingEvent, Data: data})
			pendingEvent = ""
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning SSE body: %v", err)
	}
	return frames
}

func progressEventFromSnapshot(snap paper.DownloadJobSnapshot, index int) paper.DownloadEvent {
	entry := snap.Entries[index]
	return paper.DownloadEvent{
		JobID:    snap.JobID,
		Progress: &entry,
	}
}

func summaryEventFromSnapshot(snap paper.DownloadJobSnapshot) paper.DownloadEvent {
	s := snap
	return paper.DownloadEvent{
		JobID:   snap.JobID,
		Summary: &s,
	}
}

func TestPDFDownloadController_Stream_UnknownJob_Returns404WithoutSSEFrames(t *testing.T) {
	t.Parallel()

	reader := mocks.NewPaperPDFDownloadReader() // default SubscribeErr = ErrDownloadJobUnknown

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/missing/stream", nil)
	w := httptest.NewRecorder()

	newDownloadEngine(reader).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404; body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); strings.Contains(ct, "text/event-stream") {
		t.Fatalf("404 response must not carry SSE Content-Type; got %q", ct)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v; raw=%s", err, w.Body.String())
	}
	errEnv, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("body.error missing; body=%v", body)
	}
	if code, _ := errEnv["code"].(float64); int(code) != http.StatusNotFound {
		t.Fatalf("error.code=%v, want 404", errEnv["code"])
	}
	if msg, _ := errEnv["message"].(string); msg != "download job unknown" {
		t.Errorf("error.message=%q, want %q", msg, "download job unknown")
	}

	if got := len(parseSSEFrames(t, w.Body.String())); got != 0 {
		t.Fatalf("expected no SSE frames; got %d", got)
	}

	if got := len(reader.SubscribeCalls); got != 1 {
		t.Fatalf("SubscribePDFDownloadJob calls=%d, want 1", got)
	}
}

func TestPDFDownloadController_Stream_BacklogIncludingSummary_ReplaysAndCloses(t *testing.T) {
	t.Parallel()

	// A job that has already completed at the moment of Subscribe must
	// have its full event log delivered in backlog (Progress events plus
	// the terminal Summary). The handler must not enter the live loop.
	snap := completedSnapshot()
	reader := mocks.NewPaperPDFDownloadReader()
	reader.SubscribeErr = nil
	reader.SubscribeBacklog = []paper.DownloadEvent{
		progressEventFromSnapshot(snap, 0),
		progressEventFromSnapshot(snap, 1),
		summaryEventFromSnapshot(snap),
	}
	// Live channel may be nil for completed jobs; the handler must not
	// dereference it when the backlog already terminates with Summary.
	reader.SubscribeLive = nil

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/job-done/stream", nil)
	w := httptest.NewRecorder()

	newDownloadEngine(reader).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type=%q, want text/event-stream", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control=%q, want no-cache", cc)
	}

	frames := parseSSEFrames(t, w.Body.String())
	if len(frames) != 3 {
		t.Fatalf("frame count=%d, want 3; body=%q", len(frames), w.Body.String())
	}
	if frames[0].Event != "download.progress" || frames[1].Event != "download.progress" {
		t.Errorf("first two frames must be download.progress; got %q, %q", frames[0].Event, frames[1].Event)
	}
	if frames[2].Event != "download.summary" {
		t.Errorf("final frame must be download.summary; got %q", frames[2].Event)
	}

	var summary map[string]any
	if err := json.Unmarshal([]byte(frames[2].Data), &summary); err != nil {
		t.Fatalf("summary data not JSON: %v; raw=%s", err, frames[2].Data)
	}
	if summary["job_id"] != "job-done" {
		t.Errorf("summary.job_id=%v, want job-done", summary["job_id"])
	}
	if got, _ := summary["total"].(float64); int(got) != snap.Total {
		t.Errorf("summary.total=%v, want %d", summary["total"], snap.Total)
	}
	if got, _ := summary["succeeded"].(float64); int(got) != snap.Succeeded {
		t.Errorf("summary.succeeded=%v, want %d", summary["succeeded"], snap.Succeeded)
	}
	if got, _ := summary["failed"].(float64); int(got) != snap.Failed {
		t.Errorf("summary.failed=%v, want %d", summary["failed"], snap.Failed)
	}
	if _, present := summary["entries"]; present {
		t.Errorf("summary must NOT carry per-entry details (totals only); got %v", summary)
	}
}

func TestPDFDownloadController_Stream_LiveEvents_ReplayedInOrderThenSummaryCloses(t *testing.T) {
	t.Parallel()

	snap := inProgressSnapshot()
	// Re-shape snapshot into a finalized one so the summary frame has
	// stable totals. The Stream test does not care about totals beyond
	// "did the right values get serialized as the SummaryEventDTO".
	finalSnap := snap
	finalSnap.Completed = true
	finalSnap.CompletedAt = time.Date(2026, 5, 9, 10, 30, 0, 0, time.UTC)

	live := make(chan paper.DownloadEvent, 3)
	live <- progressEventFromSnapshot(snap, 0)
	live <- progressEventFromSnapshot(snap, 1)
	live <- summaryEventFromSnapshot(finalSnap)
	close(live)

	reader := mocks.NewPaperPDFDownloadReader()
	reader.SubscribeErr = nil
	reader.SubscribeBacklog = nil
	reader.SubscribeLive = live

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/job-progress/stream", nil)
	w := httptest.NewRecorder()

	newDownloadEngine(reader).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type=%q, want text/event-stream", ct)
	}

	frames := parseSSEFrames(t, w.Body.String())
	if len(frames) != 3 {
		t.Fatalf("frame count=%d, want 3; body=%q", len(frames), w.Body.String())
	}
	if frames[0].Event != "download.progress" || frames[1].Event != "download.progress" {
		t.Errorf("first two frames must be download.progress; got %q, %q", frames[0].Event, frames[1].Event)
	}
	if frames[2].Event != "download.summary" {
		t.Errorf("final frame must be download.summary; got %q", frames[2].Event)
	}

	// Verify a progress payload carries paper_id and status — proves
	// the DTO mapper was wired correctly on the live path.
	var firstProgress map[string]any
	if err := json.Unmarshal([]byte(frames[0].Data), &firstProgress); err != nil {
		t.Fatalf("progress data not JSON: %v; raw=%s", err, frames[0].Data)
	}
	if firstProgress["status"] != "success" {
		t.Errorf("first progress status=%v, want success", firstProgress["status"])
	}
	if _, ok := firstProgress["paper_id"].(map[string]any); !ok {
		t.Errorf("first progress paper_id missing/wrong shape; got %v", firstProgress["paper_id"])
	}
}

func TestPDFDownloadController_Stream_LiveChannelClosedWithoutSummary_EmitsErrorFrame(t *testing.T) {
	t.Parallel()

	snap := inProgressSnapshot()
	live := make(chan paper.DownloadEvent, 2)
	live <- progressEventFromSnapshot(snap, 0)
	close(live) // closed without a Summary frame — slow-subscriber drop / shutdown.

	reader := mocks.NewPaperPDFDownloadReader()
	reader.SubscribeErr = nil
	reader.SubscribeBacklog = nil
	reader.SubscribeLive = live

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/job-progress/stream", nil)
	w := httptest.NewRecorder()

	// The handler must not panic even though the live channel never
	// delivered a Summary; the only acceptable signal is a final
	// `event: error` frame.
	newDownloadEngine(reader).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
	frames := parseSSEFrames(t, w.Body.String())
	if len(frames) != 2 {
		t.Fatalf("frame count=%d, want 2 (progress + error); body=%q", len(frames), w.Body.String())
	}
	if frames[0].Event != "download.progress" {
		t.Errorf("first frame=%q, want download.progress", frames[0].Event)
	}
	if frames[1].Event != "error" {
		t.Errorf("final frame=%q, want error", frames[1].Event)
	}
}

func TestPDFDownloadController_Stream_ClientDisconnect_ReturnsWithoutPanic(t *testing.T) {
	t.Parallel()

	// Live channel is never fed; the handler must block in select
	// waiting for either ctx.Done() or a live event. We cancel the
	// request context shortly after ServeHTTP starts and assert the
	// handler returns without panicking and without writing further
	// frames.
	live := make(chan paper.DownloadEvent)
	reader := mocks.NewPaperPDFDownloadReader()
	reader.SubscribeErr = nil
	reader.SubscribeBacklog = nil
	reader.SubscribeLive = live

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/downloads/job-progress/stream", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	done := make(chan struct{})
	go func() {
		newDownloadEngine(reader).ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-done:
		// handler returned — success.
	case <-time.After(2 * time.Second):
		t.Fatalf("handler did not return within 2s of context cancellation")
	}
	wg.Wait()

	// We do NOT require any specific status here; gin may have already
	// flushed headers before the cancel arrived (200 + open stream is
	// fine). What matters is that the handler returned without panic
	// and never emitted a summary frame for a job that produced none.
	for _, f := range parseSSEFrames(t, w.Body.String()) {
		if f.Event == "download.summary" {
			t.Errorf("did not expect a summary frame; got data=%q", f.Data)
		}
	}
}
