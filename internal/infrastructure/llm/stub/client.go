// Package stub is the placeholder shared.LLMClient adapter wired by
// bootstrap until a real provider (e.g. Anthropic) ships. The same type
// also serves as the scriptable test double — Queue* methods let unit
// tests inject specific outcomes; production leaves the queue empty and
// gets the per-prompt default response.
package stub

import (
	"context"
	"fmt"
	"sync"

	app "github.com/yoavweber/research-monitor/backend/internal/application/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

const modelName = "fake"

// thesisDefault always flags true with a "default" rationale because no
// thesis profile is defined yet — without a definition of "what counts" the
// stub has no basis to discriminate. Until the thesis-profile follow-up
// ships, every paper rides through with this placeholder so the analyses
// table doesn't carry fake-discriminated values.
const thesisDefault = `{"flag": true, "rationale": "default — no thesis profile defined yet; revisit when the thesis-profile spec ships."}`

type Result struct {
	Response *shared.LLMResponse
	Err      error
}

// Client is both the prod default and the unit-test scriptable double.
// Empty queue → per-prompt canned response; Queue* prepends behaviour for
// scenarios that need to fail or return specific text.
type Client struct {
	mu sync.Mutex

	Results []Result

	Calls     []shared.LLMRequest
	CallCount int
}

func New() *Client { return &Client{} }

func (c *Client) QueueResponse(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Results = append(c.Results, Result{Response: &shared.LLMResponse{Text: text, Model: modelName}})
}

func (c *Client) QueueError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Results = append(c.Results, Result{Err: err})
}

func (c *Client) Complete(_ context.Context, req shared.LLMRequest) (*shared.LLMResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.CallCount++
	c.Calls = append(c.Calls, req)

	if len(c.Results) == 0 {
		return &shared.LLMResponse{
			Text:          defaultText(req.PromptVersion),
			Model:         modelName,
			PromptVersion: req.PromptVersion,
		}, nil
	}

	next := c.Results[0]
	c.Results = c.Results[1:]
	if next.Err != nil {
		return nil, next.Err
	}
	if next.Response == nil {
		return &shared.LLMResponse{}, nil
	}
	resp := *next.Response
	if resp.PromptVersion == "" {
		resp.PromptVersion = req.PromptVersion
	}
	return &resp, nil
}

func (c *Client) Snapshot() []shared.LLMRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]shared.LLMRequest, len(c.Calls))
	copy(out, c.Calls)
	return out
}

func defaultText(version string) string {
	switch version {
	case app.PromptVersionShort:
		return "Stub short summary placeholder."
	case app.PromptVersionLong:
		return "Stub long summary placeholder. The stub provider does not read the paper body."
	case app.PromptVersionThesis:
		return thesisDefault
	default:
		return fmt.Sprintf("stub: unsupported prompt version %s", version)
	}
}
