package extraction_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	domainextraction "github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	extractionctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newEngine wires an in-memory Gin engine with the error envelope middleware
// so sentinel-translation tests see the same rendering as production. The
// routes here mirror what Task 3.6 will register on the /api group.
func newEngine(ctrl *extractionctrl.ExtractionController) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorEnvelope())
	engine.POST("/api/extractions", ctrl.Submit)
	engine.GET("/api/extractions/:id", ctrl.Get)
	return engine
}

func TestExtractionController_Submit_Valid_Returns202WithIDAndPending(t *testing.T) {
	t.Parallel()

	fake := &mocks.ExtractionUseCaseFake{
		SubmitResult: domainextraction.SubmitResult{
			ID:     "ext-123",
			Status: domainextraction.JobStatusPending,
		},
	}
	ctrl := extractionctrl.NewExtractionController(fake)

	body := `{"source_type":"paper","source_id":"2404.12345","pdf_path":"/tmp/p.pdf"}`
	req := httptest.NewRequest(http.MethodPost, "/api/extractions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d, want 202; body=%s", w.Code, w.Body.String())
	}

	submit, _, _ := fake.CallsSnapshot()
	if len(submit) != 1 {
		t.Fatalf("Submit calls=%d, want 1", len(submit))
	}
	if submit[0].SourceType != "paper" || submit[0].SourceID != "2404.12345" || submit[0].PDFPath != "/tmp/p.pdf" {
		t.Fatalf("Submit payload mismatch: %+v", submit[0])
	}

	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v; raw=%s", err, w.Body.String())
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("body.data missing; body=%v", env)
	}
	if got, _ := data["id"].(string); got != "ext-123" {
		t.Fatalf("data.id=%v, want ext-123", data["id"])
	}
	if got, _ := data["status"].(string); got != "pending" {
		t.Fatalf("data.status=%v, want pending", data["status"])
	}
	if got, _ := data["source_type"].(string); got != "paper" {
		t.Fatalf("data.source_type=%v, want paper", data["source_type"])
	}
	if got, _ := data["source_id"].(string); got != "2404.12345" {
		t.Fatalf("data.source_id=%v, want 2404.12345", data["source_id"])
	}
}

func TestExtractionController_Submit_MissingPDFPath_Returns400(t *testing.T) {
	t.Parallel()

	fake := &mocks.ExtractionUseCaseFake{}
	ctrl := extractionctrl.NewExtractionController(fake)

	body := `{"source_type":"paper","source_id":"2404.12345"}`
	req := httptest.NewRequest(http.MethodPost, "/api/extractions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newEngine(ctrl).ServeHTTP(w, req)

	assertErrorEnvelope(t, w, http.StatusBadRequest)

	submit, _, _ := fake.CallsSnapshot()
	if len(submit) != 0 {
		t.Fatalf("Submit calls=%d, want 0 (binding must reject before use case)", len(submit))
	}
}

func TestExtractionController_Submit_UnsupportedSourceType_Returns400(t *testing.T) {
	t.Parallel()

	fake := &mocks.ExtractionUseCaseFake{
		SubmitErr: domainextraction.ErrUnsupportedSourceType,
	}
	ctrl := extractionctrl.NewExtractionController(fake)

	body := `{"source_type":"html","source_id":"abc","pdf_path":"/tmp/p.pdf"}`
	req := httptest.NewRequest(http.MethodPost, "/api/extractions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newEngine(ctrl).ServeHTTP(w, req)

	assertErrorEnvelope(t, w, http.StatusBadRequest)

	submit, _, _ := fake.CallsSnapshot()
	if len(submit) != 1 {
		t.Fatalf("Submit calls=%d, want 1 (use case decides unsupported source type)", len(submit))
	}
}

func TestExtractionController_Get_Done_ReturnsArtifact(t *testing.T) {
	t.Parallel()

	row := domainextraction.Extraction{
		ID:         "ext-done",
		SourceType: "paper",
		SourceID:   "2404.12345",
		Status:     domainextraction.JobStatusDone,
		Artifact: &domainextraction.Artifact{
			Title:        "A sample paper",
			BodyMarkdown: "# A sample paper\n\nbody",
			Metadata: domainextraction.Metadata{
				ContentType: "paper",
				WordCount:   42,
			},
		},
	}
	fake := &mocks.ExtractionUseCaseFake{GetResult: &row}
	ctrl := extractionctrl.NewExtractionController(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/extractions/ext-done", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	_, get, _ := fake.CallsSnapshot()
	if len(get) != 1 || get[0] != "ext-done" {
		t.Fatalf("Get calls=%v, want [ext-done]", get)
	}

	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	data := env["data"].(map[string]any)

	if data["id"] != "ext-done" || data["status"] != "done" {
		t.Fatalf("identity/status mismatch: %+v", data)
	}
	if data["title"] != "A sample paper" {
		t.Fatalf("data.title=%v, want 'A sample paper'", data["title"])
	}
	if data["body_markdown"] != "# A sample paper\n\nbody" {
		t.Fatalf("data.body_markdown=%v", data["body_markdown"])
	}
	meta, ok := data["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("data.metadata missing; data=%v", data)
	}
	if meta["content_type"] != "paper" {
		t.Fatalf("metadata.content_type=%v, want paper", meta["content_type"])
	}
	if got, _ := meta["word_count"].(float64); int(got) != 42 {
		t.Fatalf("metadata.word_count=%v, want 42", meta["word_count"])
	}

	// Failure-side fields must be absent when status == done.
	if _, present := data["failure_reason"]; present {
		t.Fatalf("done response leaked failure_reason; data=%v", data)
	}
	if _, present := data["failure_message"]; present {
		t.Fatalf("done response leaked failure_message; data=%v", data)
	}
}

func TestExtractionController_Get_Failed_ReturnsReasonOmitsArtifact(t *testing.T) {
	t.Parallel()

	row := domainextraction.Extraction{
		ID:         "ext-fail",
		SourceType: "paper",
		SourceID:   "2404.99999",
		Status:     domainextraction.JobStatusFailed,
		Failure: &domainextraction.Failure{
			Reason:  domainextraction.FailureReasonScannedPDF,
			Message: "no extractable text",
		},
	}
	fake := &mocks.ExtractionUseCaseFake{GetResult: &row}
	ctrl := extractionctrl.NewExtractionController(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/extractions/ext-fail", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	data := env["data"].(map[string]any)

	if data["status"] != "failed" {
		t.Fatalf("data.status=%v, want failed", data["status"])
	}
	if data["failure_reason"] != "scanned_pdf" {
		t.Fatalf("data.failure_reason=%v, want scanned_pdf", data["failure_reason"])
	}
	if data["failure_message"] != "no extractable text" {
		t.Fatalf("data.failure_message=%v", data["failure_message"])
	}

	for _, k := range []string{"title", "body_markdown", "metadata"} {
		if _, present := data[k]; present {
			t.Fatalf("failed response leaked %q; data=%v", k, data)
		}
	}
}

func TestExtractionController_Get_Pending_OmitsArtifactAndFailure(t *testing.T) {
	t.Parallel()

	row := domainextraction.Extraction{
		ID:         "ext-pending",
		SourceType: "paper",
		SourceID:   "2404.00001",
		Status:     domainextraction.JobStatusPending,
	}
	fake := &mocks.ExtractionUseCaseFake{GetResult: &row}
	ctrl := extractionctrl.NewExtractionController(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/extractions/ext-pending", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	data := env["data"].(map[string]any)

	if data["status"] != "pending" {
		t.Fatalf("data.status=%v, want pending", data["status"])
	}
	for _, k := range []string{"title", "body_markdown", "metadata", "failure_reason", "failure_message"} {
		if _, present := data[k]; present {
			t.Fatalf("pending response leaked %q; data=%v", k, data)
		}
	}
}

func TestExtractionController_Get_NotFound_Returns404Envelope(t *testing.T) {
	t.Parallel()

	fake := &mocks.ExtractionUseCaseFake{GetErr: domainextraction.ErrNotFound}
	ctrl := extractionctrl.NewExtractionController(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/extractions/missing", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	assertErrorEnvelope(t, w, http.StatusNotFound)
}

// assertErrorEnvelope verifies the response is a well-formed error envelope at
// the expected status code and that no `data` field leaked through alongside it.
func assertErrorEnvelope(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()

	if w.Code != wantStatus {
		t.Fatalf("status=%d, want %d; body=%s", w.Code, wantStatus, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v; raw=%s", err, w.Body.String())
	}
	errEnv, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("body.error missing or wrong type; body=%v", body)
	}
	if code, _ := errEnv["code"].(float64); int(code) != wantStatus {
		t.Fatalf("error.code=%v, want %d", errEnv["code"], wantStatus)
	}
	if msg, _ := errEnv["message"].(string); msg == "" {
		t.Fatalf("error.message empty")
	}
	if _, present := body["data"]; present {
		t.Fatalf("error response must not carry data; body=%v", body)
	}
}
