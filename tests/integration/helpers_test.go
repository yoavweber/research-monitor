//go:build integration

package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
	"github.com/yoavweber/research-monitor/backend/tests/ssetest"
)

// doAuthenticatedGet issues a GET against path (relative to env's server, not
// a full URL) with a valid bearer token. Each scenario stays focused on its
// assertions and the auth header is set in exactly one place.
func doAuthenticatedGet(t *testing.T, env *setup.TestEnv, path string) *http.Response {
	t.Helper()
	req := setup.AuthorizedRequest(t, env, http.MethodGet, path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

// doAuthenticatedPost issues an authenticated POST with a raw JSON body
// string against path (relative to env's server). The body is set directly
// (not passed through AuthorizedRequest's json.Marshal) because callers
// already hand in pre-formatted JSON literals, and re-marshaling a string
// would quote it into an invalid body.
func doAuthenticatedPost(t *testing.T, env *setup.TestEnv, path, body string) *http.Response {
	t.Helper()
	req := setup.AuthorizedRequest(t, env, http.MethodPost, path, nil)
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
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
