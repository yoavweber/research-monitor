package httpclient

import (
	"context"
	"errors"
	"net"
	"net/url"
)

// IsTransportError translates stdlib networking errors into a boolean classification.
// It returns true if the error represents a transport timeout or unavailability
// (e.g. context timeout, context canceled, network dial timeout, url error).
func IsTransportError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	return false
}
