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

// PasswordHasher hashes plaintext passwords and verifies plaintext against a
// stored hash. Concrete implementation lives in internal/infrastructure/auth/
// and uses bcrypt. Verify returns ErrHashMismatch when the plaintext does not
// match the stored hash so callers can distinguish "wrong password" from
// "hasher malfunctioned" via errors.Is.
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(plain, hash string) error // returns ErrHashMismatch on mismatch
}

// TokenSigner issues signed access and refresh tokens for a given subject
// (typically a user UUID string). Access and refresh use separate signing
// keys so a refresh token presented as an access token fails signature
// verification immediately. Returned expiresAt reflects the token's exp claim
// in absolute wall-clock time.
type TokenSigner interface {
	IssueAccess(subject string) (token string, expiresAt time.Time, err error)
	IssueRefresh(subject string) (token string, expiresAt time.Time, err error)
}

// TokenValidator verifies signed access and refresh tokens and returns the
// embedded subject. Implementations distinguish malformed input, invalid
// signature, and expired tokens via the ErrToken* sentinels so callers can
// translate each case into the right HTTP response.
type TokenValidator interface {
	VerifyAccess(token string) (subject string, err error)
	VerifyRefresh(token string) (subject string, err error)
}
