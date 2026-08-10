//go:build integration

package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
	"github.com/yoavweber/research-monitor/backend/tests/ssetest"
)

// doAuthenticatedGet issues a GET against the test server with the canonical
// X-API-Token. Each scenario stays focused on its assertions and the auth
// header is set in exactly one place.
func doAuthenticatedGet(t *testing.T, url string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set(middleware.APITokenHeader, setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

// doAuthenticatedPost issues an authenticated POST with a JSON body string.
func doAuthenticatedPost(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.APITokenHeader, setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return resp
}

// readUntilSummary drains body until the PDF-download terminal summary
// frame arrives or the stream closes, then returns every frame seen in
// order. Tests use this when they need the full event log up to and
// including the terminal frame.
func readUntilSummary(t *testing.T, body io.Reader) []ssetest.Frame {
	t.Helper()
	frames, err := ssetest.ReadUntilEvent(body, paperctrl.EventDownloadSummary)
	if err != nil {
		t.Fatalf("read SSE: %v", err)
	}
	return frames
}

// assertErrorEnvelope decodes the standard { "error": { "code": N, "message": "..." } }
// envelope rendered by the ErrorEnvelope middleware from *shared.HTTPError
// sentinels, and verifies the shape. code arrives as float64 after JSON decode.
func assertErrorEnvelope(t *testing.T, resp *http.Response, wantCode int) {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("body.error missing or wrong type: %#v", body)
	}
	gotCode, ok := errObj["code"].(float64)
	if !ok {
		t.Fatalf("body.error.code missing or wrong type: %#v", errObj["code"])
	}
	if int(gotCode) != wantCode {
		t.Errorf("body.error.code = %d want %d", int(gotCode), wantCode)
	}
	msg, ok := errObj["message"].(string)
	if !ok || msg == "" {
		t.Errorf("body.error.message missing or empty: %#v", errObj["message"])
	}
}
