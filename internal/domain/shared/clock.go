package shared

import "time"

// SystemClock is the production Clock — wraps time.Now.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }
