package shared

import (
	"errors"
	"fmt"
)

// HTTPError wraps a domain error with an HTTP status code and a user-safe message.
// Middleware in interface/http/middleware/error_envelope.go translates this to the
// standard response envelope.
type HTTPError struct {
	Code    int
	Message string
	Err     error
}

func (e *HTTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%d %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%d %s", e.Code, e.Message)
}

func (e *HTTPError) Unwrap() error { return e.Err }

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
