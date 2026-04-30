//go:build mineru

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/extraction/mineru"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

// e2eExtractionWire mirrors the controller's ExtractionStatusResponse. Defined
// inline (rather than imported) so a wire-contract drift fails this test
// loudly even when the hermetic suite is not part of the same build. Pointer
// metadata captures the omitempty contract for pending / running / failed
// rows (R2.4).
type e2eExtractionWire struct {
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

// TestMineruE2E exercises the full extraction stack — controller, use case,
// worker, repository, and the real MinerU adapter — against the committed
// AMM-arbitrage paper fixture. Opt-in via the `mineru` build tag for the
// same reason as the adapter test (Task 2.2): MinerU is a heavy external
// dependency (model weights, multi-minute cold start) the default `task test`
// run cannot assume is installed.
//
// No t.Parallel(): MinerU is resource-heavy (CPU, memory, GPU/CPU model
// files) and concurrent invocations conflict on disk + compute.
//
// The final body_markdown is logged BEFORE any assertion so the operator can
// audit what the full pipeline (adapter -> normalizer -> persistence) actually
// produced even when assertions fail. Per the design's Testing Strategy:
// initial assertions are expected to fail and serve as the verification gate
// — ratchet assertions or update the normalizer until both sides agree.
func TestMineruE2E(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	pdfPath := filepath.Join(wd, "testdata", "amm_arbitrage_with_fees.pdf")
	if _, statErr := os.Stat(pdfPath); statErr != nil {
		// Distinguish "fixture moved" from "MinerU broken" — skip rather than
		// fail so the operator gets an unambiguous signal.
		if errors.Is(statErr, os.ErrNotExist) {
			t.Skipf("fixture missing at %s; restore it before running this test", pdfPath)
		}
		t.Fatalf("stat fixture %s: %v", pdfPath, statErr)
	}

	mineruPath := os.Getenv("MINERU_PATH")
	if mineruPath == "" {
		mineruPath = "mineru"
	}

	// Cold-start model loading can dominate the first invocation, so 5
	// minutes is the realistic upper bound used both for the per-call timeout
	// inside the adapter and for the polling deadline below.
	const callTimeout = 5 * time.Minute

	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		Extractor:              mineru.NewMineruExtractor(mineruPath, callTimeout),
		ExtractionMaxWords:     50000,
		ExtractionJobExpiry:    1 * time.Hour,
		ExtractionSignalBuffer: 10,
	})
	t.Cleanup(env.Close)

	// Submit. The body shape mirrors the hermetic suite's POST contract so a
	// drift in the controller's bind tags will surface here as well.
	body := `{"source_type":"paper","source_id":"amm-arbitrage-fees","pdf_path":"` + pdfPath + `"}`
	req, _ := http.NewRequest(http.MethodPost, env.Server.URL+"/api/extractions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.APITokenHeader, setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("submit status = %d want 202; body=%s", resp.StatusCode, string(raw))
	}
	var submitEnvelope struct {
		Data e2eExtractionWire `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&submitEnvelope); err != nil {
		resp.Body.Close()
		t.Fatalf("decode submit: %v", err)
	}
	resp.Body.Close()
	submitted := submitEnvelope.Data
	if submitted.ID == "" {
		t.Fatalf("submit response id is empty")
	}

	// Poll every 2 seconds (per design "every 2 seconds") until terminal or
	// deadline. We treat both `done` and `failed` as terminal so a failed
	// run still surfaces its body_markdown via the assertion phase rather
	// than hanging until timeout.
	pollCtx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	var final e2eExtractionWire
	final.Status = submitted.Status
	deadline := time.Now().Add(callTimeout)
	for {
		getReq, _ := http.NewRequestWithContext(pollCtx, http.MethodGet, env.Server.URL+"/api/extractions/"+submitted.ID, nil)
		getReq.Header.Set(middleware.APITokenHeader, setup.TestToken)
		getResp, getErr := http.DefaultClient.Do(getReq)
		if getErr != nil {
			t.Fatalf("poll GET: %v", getErr)
		}
		if getResp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(getResp.Body)
			getResp.Body.Close()
			t.Fatalf("poll GET status = %d want 200; body=%s", getResp.StatusCode, string(raw))
		}
		var envelope struct {
			Data e2eExtractionWire `json:"data"`
		}
		if err := json.NewDecoder(getResp.Body).Decode(&envelope); err != nil {
			getResp.Body.Close()
			t.Fatalf("decode poll: %v", err)
		}
		getResp.Body.Close()
		final = envelope.Data

		if final.Status == "done" || final.Status == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout after %v waiting for terminal status; last observed: status=%q id=%s failure_reason=%q failure_message=%q",
				callTimeout, final.Status, submitted.ID, final.FailureReason, final.FailureMessage)
		}
		time.Sleep(2 * time.Second)
	}

	// Log the markdown BEFORE any assertion so the operator sees what the
	// full pipeline produced even when assertions fail. Per design Testing
	// Strategy: this is the headline observable for Task 5.2.
	t.Logf("=== End-to-End body_markdown (status=%s id=%s) ===\n%s\n=== End body_markdown ===",
		final.Status, submitted.ID, final.BodyMarkdown)
	if final.FailureReason != "" || final.FailureMessage != "" {
		t.Logf("failure_reason=%q failure_message=%q", final.FailureReason, final.FailureMessage)
	}

	// Assertions use t.Errorf (not t.Fatalf) so a single run surfaces every
	// gap; the design's "ratchet assertions to match observed reality" loop
	// requires seeing all violations at once rather than one-at-a-time.

	// Headline: terminal state must be `done`. Everything below is undefined
	// when status is `failed` — the artifact branch is elided per R2.4 — but
	// keeping the remaining assertions live still pins their wire contract.
	if final.Status != "done" {
		t.Errorf("status = %q want %q (failure_reason=%q failure_message=%q)",
			final.Status, "done", final.FailureReason, final.FailureMessage)
	}

	// R3.5: title is the first H1 of the normalized body, surfaced as a
	// dedicated artifact field.
	if final.Title == "" {
		t.Errorf("title is empty; the normalizer must lift the first H1 into title")
	}

	// R3.6: math delimiters survive normalization. Either inline ($...$) or
	// display ($$...$$) is acceptable — the AMM paper fixture has both, but
	// we only require the OR to leave room for MinerU output drift.
	mathPattern := regexp.MustCompile(`\$\$[^$]+\$\$|\$[^$]+\$`)
	if !mathPattern.MatchString(final.BodyMarkdown) {
		t.Errorf("body_markdown contains no $...$ inline or $$...$$ display math span; expected at least one")
	}

	// R3.3: references / bibliography / works-cited section is truncated by
	// the normalizer regardless of heading level (# through ######) and
	// regardless of case. Asserting on heading text rather than the literal
	// "## References" makes the test resilient to MinerU choosing a different
	// heading level.
	refsPattern := regexp.MustCompile(`(?im)^#{1,6}\s*(references|bibliography|works cited)\s*$`)
	if refsPattern.MatchString(final.BodyMarkdown) {
		t.Errorf("body_markdown still contains a references/bibliography/works-cited heading; the normalizer must truncate it")
	}

	// R3.7 / R3.8: metadata block is present on done responses, content_type
	// mirrors source_type, and word_count is bounded above by the configured
	// max (50000 in this test) and strictly positive.
	if final.Metadata == nil {
		t.Errorf("metadata block missing on done response")
	} else {
		if final.Metadata.WordCount <= 0 {
			t.Errorf("metadata.word_count = %d want > 0", final.Metadata.WordCount)
		}
		if final.Metadata.WordCount > 50000 {
			t.Errorf("metadata.word_count = %d exceeds configured max 50000", final.Metadata.WordCount)
		}
		if final.Metadata.ContentType != "paper" {
			t.Errorf("metadata.content_type = %q want %q", final.Metadata.ContentType, "paper")
		}
	}

	// Defense-in-depth: a `done` response must not carry failure fields
	// (R2.4 elision contract).
	if final.Status == "done" && (final.FailureReason != "" || final.FailureMessage != "") {
		t.Errorf("done response carries failure fields: reason=%q message=%q",
			final.FailureReason, final.FailureMessage)
	}
}
