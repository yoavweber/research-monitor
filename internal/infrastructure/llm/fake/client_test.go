package fake_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	app "github.com/yoavweber/research-monitor/backend/internal/application/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/llm/fake"
)

func TestClient_ShortPrompt_ReturnsStableShortText(t *testing.T) {
	t.Parallel()

	c := fake.New()
	req := shared.LLMRequest{PromptVersion: app.PromptVersionShort, UserPrompt: "ignored"}

	first, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete err = %v", err)
	}
	second, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("second Complete err = %v", err)
	}

	if first.Text != second.Text {
		t.Errorf("fake is non-deterministic for the same prompt version: %q vs %q", first.Text, second.Text)
	}
	if !strings.Contains(strings.ToLower(first.Text), "short") {
		t.Errorf("short canned text = %q, want it to look like a short summary", first.Text)
	}
	if first.Model != "fake" {
		t.Errorf("Model = %q, want %q", first.Model, "fake")
	}
}

func TestClient_LongPrompt_ReturnsLongerThanShort(t *testing.T) {
	t.Parallel()

	c := fake.New()

	short, _ := c.Complete(context.Background(), shared.LLMRequest{PromptVersion: app.PromptVersionShort})
	long, _ := c.Complete(context.Background(), shared.LLMRequest{PromptVersion: app.PromptVersionLong})

	if len(long.Text) <= len(short.Text) {
		t.Errorf("long text length %d <= short text length %d", len(long.Text), len(short.Text))
	}
	if long.Model != "fake" {
		t.Errorf("Model = %q, want %q", long.Model, "fake")
	}
}

func TestClient_ThesisPrompt_ReturnsValidEnvelope(t *testing.T) {
	t.Parallel()

	c := fake.New()
	req := shared.LLMRequest{PromptVersion: app.PromptVersionThesis}

	resp, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete err = %v", err)
	}

	// Envelope shape: must decode to {flag: bool, rationale: string} with
	// both fields present and well-typed. Mirrors the analyzer use case's
	// parser contract; if this assertion drifts from the parser, the fake
	// would produce ErrAnalyzerMalformedResponse in production.
	var got struct {
		Flag      *bool   `json:"flag"`
		Rationale *string `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &got); err != nil {
		t.Fatalf("thesis canned text is not valid JSON: %v; text = %q", err, resp.Text)
	}
	if got.Flag == nil {
		t.Errorf("envelope missing flag")
	}
	if got.Rationale == nil || *got.Rationale == "" {
		t.Errorf("envelope missing or empty rationale")
	}
}

func TestClient_PromptVersionEcho_PreservedOnResponse(t *testing.T) {
	t.Parallel()

	c := fake.New()
	want := app.PromptVersionLong

	resp, err := c.Complete(context.Background(), shared.LLMRequest{PromptVersion: want})
	if err != nil {
		t.Fatalf("Complete err = %v", err)
	}

	if resp.PromptVersion != want {
		t.Errorf("PromptVersion echo = %q, want %q", resp.PromptVersion, want)
	}
}
