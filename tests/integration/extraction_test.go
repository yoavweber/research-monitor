//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

// extractionStatusWire mirrors the controller's ExtractionStatusResponse.
// Defined inline so the test fails loudly if the wire contract drifts. The
// pointer-to-metadata field is what makes pending / running / failed
// responses elide the metadata block via omitempty (Requirement 2.4).
type extractionStatusWire struct {
	ID         string `json:"id"`
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	Status     string `json:"status"`

	Title        string `json:"title,omitempty"`
	BodyMarkdown string `json:"body_markdown,omitempty"`
	Metadata     *struct {
		ContentType string `json:"content_type"`
		WordCount   int    `json:"word_count"`
	} `json:"metadata,omitempty"`

	FailureReason  string `json:"failure_reason,omitempty"`
	FailureMessage string `json:"failure_message,omitempty"`
}

// doExtractionPost issues an authenticated POST /api/extractions with a JSON
// body. The token header is set in one place so individual cases focus on
// their assertions (mirrors the papers_test.go authenticated-request helper).
func doExtractionPost(t *testing.T, env *setup.TestEnv, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, env.Server.URL+"/api/extractions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.APITokenHeader, setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return resp
}

// doExtractionGet issues an authenticated GET /api/extractions/:id.
func doExtractionGet(t *testing.T, env *setup.TestEnv, id string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/extractions/"+id, nil)
	req.Header.Set(middleware.APITokenHeader, setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return resp
}

// decodeExtraction reads the standard { "data": ... } envelope into the wire
// DTO and closes the response body.
func decodeExtraction(t *testing.T, resp *http.Response) extractionStatusWire {
	t.Helper()
	defer resp.Body.Close()
	var body struct {
		Data extractionStatusWire `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Data
}

// pollUntilStatus polls GET /api/extractions/:id every 50ms until the
// observed status is one of `wantAny` or the timeout expires. Returns the
// last observed wire DTO. Failing on timeout makes the failure mode obvious
// rather than racing the assertions.
func pollUntilStatus(t *testing.T, env *setup.TestEnv, id string, timeout time.Duration, wantAny ...string) extractionStatusWire {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last extractionStatusWire
	for {
		resp := doExtractionGet(t, env, id)
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("poll GET status = %d want 200; body=%s", resp.StatusCode, string(body))
		}
		last = decodeExtraction(t, resp)
		for _, w := range wantAny {
			if last.Status == w {
				return last
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for status in %v; last observed status = %q (id=%s)", wantAny, last.Status, id)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// security
// TestExtraction_401_MissingToken covers Requirement 1.4 (auth on Submit) and
// Requirement 2.6 (auth on Get): the APIToken middleware MUST short-circuit
// before either handler runs when X-API-Token is absent.
func TestExtraction_401_MissingToken(t *testing.T) {
	t.Parallel()

	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		Extractor: &mocks.ExtractorFake{},
	})
	t.Cleanup(env.Close)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"post extractions without token", http.MethodPost, "/api/extractions",
			`{"source_type":"paper","source_id":"x","pdf_path":"/tmp/x.pdf"}`},
		{"get extractions by id without token", http.MethodGet, "/api/extractions/anything", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var body io.Reader
			if tc.body != "" {
				body = bytes.NewBufferString(tc.body)
			}
			req, _ := http.NewRequest(tc.method, env.Server.URL+tc.path, body)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d want 401", resp.StatusCode)
			}
		})
	}
}

// TestExtraction_400_MissingPDFPath covers Requirement 1.3: a Submit request
// with an empty / absent pdf_path must surface as 400 with the standard
// error envelope. Gin's `binding:"required"` rejects this before the
// use-case Validate runs, so the test simultaneously locks the controller's
// JSON-bind contract and the envelope shape.
func TestExtraction_400_MissingPDFPath(t *testing.T) {
	t.Parallel()

	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		Extractor: &mocks.ExtractorFake{},
	})
	t.Cleanup(env.Close)

	resp := doExtractionPost(t, env, `{"source_type":"paper","source_id":"src-missing-path"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d want 400; body=%s", resp.StatusCode, string(body))
	}
	assertErrorEnvelope(t, resp, http.StatusBadRequest)
}

// TestExtraction_400_UnsupportedSourceType covers Requirement 1.3 / 4.1:
// source_type values other than "paper" are rejected with
// ErrUnsupportedSourceType (400). The controller's bind succeeds (all three
// required fields are non-empty); the rejection comes from the use-case
// re-validation path.
func TestExtraction_400_UnsupportedSourceType(t *testing.T) {
	t.Parallel()

	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		Extractor: &mocks.ExtractorFake{},
	})
	t.Cleanup(env.Close)

	resp := doExtractionPost(t, env,
		`{"source_type":"html","source_id":"x","pdf_path":"/tmp/x.pdf"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d want 400; body=%s", resp.StatusCode, string(body))
	}
	assertErrorEnvelope(t, resp, http.StatusBadRequest)
}

// TestExtraction_404_UnknownID covers Requirement 2.5: a GET against an id
// that has never been submitted returns 404 with the standard envelope —
// never a 200 with a zero body.
func TestExtraction_404_UnknownID(t *testing.T) {
	t.Parallel()

	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		Extractor: &mocks.ExtractorFake{},
	})
	t.Cleanup(env.Close)

	resp := doExtractionGet(t, env, "nonexistent-id")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d want 404; body=%s", resp.StatusCode, string(body))
	}
	assertErrorEnvelope(t, resp, http.StatusNotFound)
}

// TestExtraction_HappyPath_PollUntilDone covers Requirements 1.1, 1.5, 1.6,
// 2.1, 2.2, 2.3, 3.3 (references-tail truncation), 3.6 (math delimiter
// pass-through), 3.8 (content_type mirroring): the worker drains a freshly
// submitted row, runs Normalize (math `$x+y$` retained, `## References`
// stripped), and writes done with `metadata.content_type=="paper"` plus a
// non-zero word count. Polling locks Requirement 2.7 — the GET keeps
// returning the latest snapshot until the terminal state is observed.
func TestExtraction_HappyPath_PollUntilDone(t *testing.T) {
	t.Parallel()

	fake := &mocks.ExtractorFake{
		Output: extraction.ExtractOutput{
			Markdown: "# Test Title\n\nSome math: $x+y$\n\n## References\nignored\n",
		},
	}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		Extractor: fake,
	})
	t.Cleanup(env.Close)

	resp := doExtractionPost(t, env,
		`{"source_type":"paper","source_id":"happy-1","pdf_path":"/tmp/happy.pdf"}`)
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d want 202; body=%s", resp.StatusCode, string(body))
	}
	submitted := decodeExtraction(t, resp)

	if submitted.Status != string(extraction.JobStatusPending) {
		t.Fatalf("submit response status = %q want %q", submitted.Status, extraction.JobStatusPending)
	}
	if submitted.ID == "" {
		t.Fatalf("submit response id is empty")
	}

	final := pollUntilStatus(t, env, submitted.ID, 5*time.Second, string(extraction.JobStatusDone))

	if final.Title != "Test Title" {
		t.Errorf("title = %q want %q", final.Title, "Test Title")
	}
	if !strings.Contains(final.BodyMarkdown, "$x+y$") {
		t.Errorf("body_markdown missing math delimiters; body=%q", final.BodyMarkdown)
	}
	if strings.Contains(final.BodyMarkdown, "## References") {
		t.Errorf("body_markdown still contains references heading; body=%q", final.BodyMarkdown)
	}
	if final.Metadata == nil {
		t.Fatalf("metadata block missing on done response")
	}
	if final.Metadata.ContentType != "paper" {
		t.Errorf("metadata.content_type = %q want %q", final.Metadata.ContentType, "paper")
	}
	if final.Metadata.WordCount <= 0 {
		t.Errorf("metadata.word_count = %d want > 0", final.Metadata.WordCount)
	}
	if final.FailureReason != "" || final.FailureMessage != "" {
		t.Errorf("done response carries failure fields: reason=%q message=%q",
			final.FailureReason, final.FailureMessage)
	}
}

// TestExtraction_Reextract_OverwritesInPlace covers Requirements 1.5 and 1.6
// (overwrite by composite key with id preserved) plus 2.7 (latest snapshot
// served on read): a second POST against the same (source_type, source_id)
// returns the same row id and, after the worker re-runs, the GET returns
// the new artifact body — never a stale one.
func TestExtraction_Reextract_OverwritesInPlace(t *testing.T) {
	t.Parallel()

	fake := &mocks.ExtractorFake{
		Output: extraction.ExtractOutput{Markdown: "# First Title\n\nfirst run body content here\n"},
	}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		Extractor: fake,
	})
	t.Cleanup(env.Close)

	// First submission. Wait for the worker to drive it to done so the
	// second submit observes a terminal prior row (the overwrite path
	// resets the row from done back to pending).
	resp1 := doExtractionPost(t, env,
		`{"source_type":"paper","source_id":"src-1","pdf_path":"/tmp/x.pdf"}`)
	if resp1.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp1.Body)
		resp1.Body.Close()
		t.Fatalf("first submit status = %d want 202; body=%s", resp1.StatusCode, string(body))
	}
	first := decodeExtraction(t, resp1)
	firstFinal := pollUntilStatus(t, env, first.ID, 5*time.Second, string(extraction.JobStatusDone))
	if !strings.Contains(firstFinal.BodyMarkdown, "first run body") {
		t.Fatalf("first done body unexpected: %q", firstFinal.BodyMarkdown)
	}

	// Reconfigure the fake to emit a different body for the second run.
	fake.Output = extraction.ExtractOutput{
		Markdown: "# Second Title\n\nsecond run body completely different\n",
	}

	resp2 := doExtractionPost(t, env,
		`{"source_type":"paper","source_id":"src-1","pdf_path":"/tmp/y.pdf"}`)
	if resp2.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()
		t.Fatalf("second submit status = %d want 202; body=%s", resp2.StatusCode, string(body))
	}
	second := decodeExtraction(t, resp2)

	if second.ID != first.ID {
		t.Fatalf("re-submit produced new id: first=%s second=%s (R1.5: composite-key overwrite must preserve id)",
			first.ID, second.ID)
	}

	secondFinal := pollUntilStatus(t, env, second.ID, 5*time.Second, string(extraction.JobStatusDone))
	if !strings.Contains(secondFinal.BodyMarkdown, "second run body") {
		t.Errorf("re-extracted body did not refresh: %q", secondFinal.BodyMarkdown)
	}
	if strings.Contains(secondFinal.BodyMarkdown, "first run body") {
		t.Errorf("re-extracted body still carries prior content: %q", secondFinal.BodyMarkdown)
	}
	if secondFinal.Title != "Second Title" {
		t.Errorf("title = %q want %q", secondFinal.Title, "Second Title")
	}
}

// TestExtraction_TooLarge covers Requirements 4.1, 4.4, 4.5 (failure
// taxonomy and message contents): with EXTRACTION_MAX_WORDS=1, a body whose
// normalized word count exceeds the threshold transitions the row to
// `failed: too_large` and the failure_message contains both the actual
// count and the configured threshold.
func TestExtraction_TooLarge(t *testing.T) {
	t.Parallel()

	fake := &mocks.ExtractorFake{
		Output: extraction.ExtractOutput{
			Markdown: "# Title\n\nbody with many words here that exceed the threshold\n",
		},
	}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		Extractor:          fake,
		ExtractionMaxWords: 1,
	})
	t.Cleanup(env.Close)

	resp := doExtractionPost(t, env,
		`{"source_type":"paper","source_id":"too-large-1","pdf_path":"/tmp/big.pdf"}`)
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("submit status = %d want 202; body=%s", resp.StatusCode, string(body))
	}
	submitted := decodeExtraction(t, resp)

	final := pollUntilStatus(t, env, submitted.ID, 5*time.Second, string(extraction.JobStatusFailed))

	if final.FailureReason != string(extraction.FailureReasonTooLarge) {
		t.Errorf("failure_reason = %q want %q", final.FailureReason, extraction.FailureReasonTooLarge)
	}
	// The use case formats the message as `word count <N> exceeds threshold <M>`
	// (usecase.go). We verify both the threshold and that an actual count
	// is present without pinning the exact number — the normalizer's word-
	// count rule is independently covered in domain tests.
	if !strings.Contains(final.FailureMessage, "threshold 1") {
		t.Errorf("failure_message missing configured threshold: %q", final.FailureMessage)
	}
	if !strings.Contains(final.FailureMessage, "word count") {
		t.Errorf("failure_message missing actual word count phrasing: %q", final.FailureMessage)
	}
	// On failed responses the artifact branch must be elided (R2.4).
	if final.Title != "" || final.BodyMarkdown != "" || final.Metadata != nil {
		t.Errorf("failed response leaks artifact fields: title=%q body=%q metadata=%v",
			final.Title, final.BodyMarkdown, final.Metadata)
	}
}

// TestExtraction_ScannedPDF covers Requirement 4.1 (typed-error to
// FailureReason mapping): an extractor returning ErrScannedPDF transitions
// the row to `failed: scanned_pdf` with the artifact branch elided.
func TestExtraction_ScannedPDF(t *testing.T) {
	t.Parallel()

	fake := &mocks.ExtractorFake{Err: extraction.ErrScannedPDF}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		Extractor: fake,
	})
	t.Cleanup(env.Close)

	resp := doExtractionPost(t, env,
		`{"source_type":"paper","source_id":"scanned-1","pdf_path":"/tmp/scanned.pdf"}`)
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("submit status = %d want 202; body=%s", resp.StatusCode, string(body))
	}
	submitted := decodeExtraction(t, resp)

	final := pollUntilStatus(t, env, submitted.ID, 5*time.Second, string(extraction.JobStatusFailed))

	if final.FailureReason != string(extraction.FailureReasonScannedPDF) {
		t.Errorf("failure_reason = %q want %q", final.FailureReason, extraction.FailureReasonScannedPDF)
	}
	if final.FailureMessage == "" {
		t.Errorf("failure_message must be non-empty on failed response")
	}
	// Artifact branch must be elided when status == failed (R2.4).
	if final.Title != "" || final.BodyMarkdown != "" || final.Metadata != nil {
		t.Errorf("failed response leaks artifact fields: title=%q body=%q metadata=%v",
			final.Title, final.BodyMarkdown, final.Metadata)
	}
}
