// Package fake is the deterministic, network-free shared.LLMClient adapter
// used by default until a real provider adapter (e.g. Anthropic) ships. The
// adapter switches on the request's PromptVersion so the analyzer use case's
// short / long / thesis calls each see a stable canned reply, including a
// valid thesis-angle JSON envelope so the parser path is exercised
// end-to-end without a real provider.
package fake

import (
	"context"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// modelName is the stable provider id the fake reports on every response.
// Persisted onto Analysis.Model when the analyzer use case runs against
// this client, which lets test assertions key on a known string.
const modelName = "fake"

// thesisCannedEnvelope satisfies the analyzer's strict {flag, rationale}
// contract so the use case's parser path succeeds without bespoke wiring.
const thesisCannedEnvelope = `{"flag": false, "rationale": "fake provider does not classify; this rationale is canned for development."}`

// Known prompt versions the fake recognises. They mirror the constants in
// internal/application/analyzer/prompts.go without taking a runtime import
// dependency on the application package — the relationship is pinned by the
// fake's unit test, which round-trips its thesis output through the
// application parser.
const (
	versionShort  = "analyzer.short.v1"
	versionLong   = "analyzer.long.v1"
	versionThesis = "analyzer.thesis.v1"
)

const (
	cannedShort = "Fake short summary: a deterministic two-sentence stand-in for a real model's short output. The fake provider has no opinion about your paper."
	cannedLong  = "Fake long summary: a deterministic placeholder for a real model's long-form summary. " +
		"The fake provider does not read the paper body; it returns this paragraph regardless of the input. " +
		"Once the Anthropic adapter ships, this text is replaced by a real summary derived from the extracted markdown."
)

// Client is the default shared.LLMClient adapter. Zero value is usable.
type Client struct{}

// New returns the fake client.
func New() *Client { return &Client{} }

// Complete returns a deterministic response keyed by req.PromptVersion. No
// network I/O is performed and the input prompts are ignored — the fake
// exists to exercise the analyzer's full path without a real provider.
func (c *Client) Complete(_ context.Context, req shared.LLMRequest) (*shared.LLMResponse, error) {
	var text string
	switch req.PromptVersion {
	case versionShort:
		text = cannedShort
	case versionLong:
		text = cannedLong
	case versionThesis:
		text = thesisCannedEnvelope
	default:
		// The use case never sends an unknown version; this branch keeps the
		// adapter from accidentally returning empty output if a future caller
		// forgets to set PromptVersion.
		text = "fake: unsupported prompt version " + req.PromptVersion
	}
	return &shared.LLMResponse{
		Text:          text,
		Model:         modelName,
		PromptVersion: req.PromptVersion,
	}, nil
}
