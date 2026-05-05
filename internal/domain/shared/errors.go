package shared

import (
	"errors"
	"fmt"
)

// HTTPError wraps a domain error with an HTTP status code and a user-safe message.
// Middleware in internal/http/middleware/error_envelope.go translates this to the
// standard response envelope. Reason is an optional machine-readable discriminator
// that surfaces under error.details.reason on the wire when non-empty; it lets
// callers tell apart sentinels that share the same HTTP code (e.g. two distinct
// 502 modes) without parsing the human-readable Message.
type HTTPError struct {
	Code    int
	Message string
	Reason  string
	Err     error
}

func (e *HTTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%d %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%d %s", e.Code, e.Message)
}

func (e *HTTPError) Unwrap() error { return e.Err }

// WithReason sets the machine-readable discriminator and returns the same
// pointer so call sites can chain it onto NewHTTPError.
func (e *HTTPError) WithReason(reason string) *HTTPError {
	e.Reason = reason
	return e
}

func NewHTTPError(code int, message string, err error) *HTTPError {
	return &HTTPError{Code: code, Message: message, Err: err}
}

// AsHTTPError unwraps err until it finds an *HTTPError, or returns nil.
func AsHTTPError(err error) *HTTPError {
	var he *HTTPError
	if errors.As(err, &he) {
		return he
	}
	return nil
}

// ErrBadStatus is a source-neutral sentinel that byte-level Fetcher
// implementations wrap (via fmt.Errorf("%w: status=%d", ErrBadStatus, code))
// to signal a non-2xx HTTP response. Adapters use errors.Is to identify it.
var ErrBadStatus = errors.New("shared.fetch: upstream returned non-success status")
