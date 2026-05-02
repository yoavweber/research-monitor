// Package fake is the deterministic, network-free shared.LLMClient adapter
// used by default until a real provider adapter (e.g. Anthropic) ships.
package fake

import (
	"context"

	app "github.com/yoavweber/research-monitor/backend/internal/application/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

const modelName = "fake"

const thesisCannedEnvelope = `{"flag": false, "rationale": "fake provider does not classify; this rationale is canned for development."}`

const (
	cannedShort = "Fake short summary: a deterministic two-sentence stand-in for a real model's short output. The fake provider has no opinion about your paper."
	cannedLong  = "Fake long summary: a deterministic placeholder for a real model's long-form summary. " +
		"The fake provider does not read the paper body; it returns this paragraph regardless of the input. " +
		"Once the Anthropic adapter ships, this text is replaced by a real summary derived from the extracted markdown."
)

type Client struct{}

func New() *Client { return &Client{} }

func (c *Client) Complete(_ context.Context, req shared.LLMRequest) (*shared.LLMResponse, error) {
	var text string
	switch req.PromptVersion {
	case app.PromptVersionShort:
		text = cannedShort
	case app.PromptVersionLong:
		text = cannedLong
	case app.PromptVersionThesis:
		text = thesisCannedEnvelope
	default:
		text = "fake: unsupported prompt version " + req.PromptVersion
	}
	return &shared.LLMResponse{
		Text:          text,
		Model:         modelName,
		PromptVersion: req.PromptVersion,
	}, nil
}
