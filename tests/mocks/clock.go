package mocks

import (
	"sync"
	"time"
)

// MovableClock is a thread-safe shared.Clock fake whose value can be advanced
// during a test. Tests that exercise time-sensitive behaviour (expiry, age
// predicates) use it to control whether a row is considered expired without
// depending on wall time. Concurrent reads and writes are guarded by a mutex
// so the production code under test sees a coherent Now() across goroutines.
type MovableClock struct {
	mu sync.Mutex
	t  time.Time
}

// NewMovableClock returns a clock seeded at t.
func NewMovableClock(t time.Time) *MovableClock {
	return &MovableClock{t: t}
}

// Now returns the current clock value.
func (c *MovableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Set replaces the clock value. Subsequent Now() calls observe the new value.
func (c *MovableClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}
