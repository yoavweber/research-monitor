package extraction

import (
	"context"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// ChannelNotifier is the in-process Notifier: a buffered channel exposed as
// a non-blocking publish to the use case (Notify) and a receive-side handle
// to the Worker (Wait). It owns the only channel so the use case never sees
// chan primitives, and the Worker stays the single drainer.
//
// Notify drops excess signals when the buffer is full — one wake is enough
// for a full drain, so the worker missing a duplicate signal is correct.
type ChannelNotifier struct {
	ch chan struct{}
}

// NewChannelNotifier returns a Notifier backed by a buffered channel. buffer
// bounds the number of in-flight wake signals; pick it from configuration so
// burst Submits never block the caller.
func NewChannelNotifier(buffer int) *ChannelNotifier {
	return &ChannelNotifier{ch: make(chan struct{}, buffer)}
}

// Notify is the use-case-facing publish. It performs a non-blocking send so
// Submit never waits on a saturated buffer. ctx satisfies the domain port
// contract; the in-process implementation has nothing to await on.
func (n *ChannelNotifier) Notify(ctx context.Context) {
	_ = ctx
	select {
	case n.ch <- struct{}{}:
	default:
	}
}

// C returns the receive side of the underlying buffered channel for the
// Worker to select on. Same channel every call, mirroring the time.Timer.C
// idiom so the Worker's wake-loop stays a single select on ctx.Done() and
// the wake signal.
func (n *ChannelNotifier) C() <-chan struct{} {
	return n.ch
}

// Compile-time check that ChannelNotifier satisfies the domain port.
var _ extraction.Notifier = (*ChannelNotifier)(nil)
