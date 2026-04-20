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

// Fetcher is a generic byte-level HTTP GET port. Implementations return the
// response body on 2xx. On non-2xx, the error wraps shared.ErrBadStatus via
// fmt.Errorf("%w: status=%d", ErrBadStatus, code). On transport failure,
// implementations return stdlib-identifiable errors (context.DeadlineExceeded,
// *url.Error, ...). Higher layers translate these into their own error
// vocabulary.
type Fetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, error)
}
