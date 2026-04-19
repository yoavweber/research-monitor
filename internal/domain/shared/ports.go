package shared

import (
	"context"
	"time"
)

// Logger — structured logging port. Concrete impl wraps slog in infrastructure/observability.
type Logger interface {
	InfoContext(ctx context.Context, msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
	DebugContext(ctx context.Context, msg string, args ...any)
	With(args ...any) Logger
}

// Clock — time source. Real impl = SystemClock{}. Tests inject a frozen clock.
type Clock interface {
	Now() time.Time
}

// LLMClient — abstraction over LLM providers. Concrete adapters live in
// infrastructure/llm/<provider>/. See Plan 3.
type LLMRequest struct {
	SystemPrompt  string
	UserPrompt    string
	Model         string
	PromptVersion string
}

type LLMResponse struct {
	Text          string
	Model         string
	PromptVersion string
}

type LLMClient interface {
	Complete(ctx context.Context, req LLMRequest) (*LLMResponse, error)
}

// Extractor — text extraction from HTML / PDF payloads. See Plan 2.
type Extractor interface {
	FromHTML(ctx context.Context, html string) (string, error)
	FromPDFURL(ctx context.Context, url string) (string, error)
}

// APIFetcher — generic port for JSON API ingestion sources (arXiv, governance forums).
// Deferred: no concrete impl in v1. Defining the port now keeps the domain stable.
type APIFetcher interface {
	Fetch(ctx context.Context, endpoint string) ([]byte, error)
}
