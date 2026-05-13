package user

import "errors"

// Aggregate-specific sentinels. The controller wraps these as *shared.HTTPError
// with the appropriate reason code; the use-case and repository return them
// raw so callers can branch via errors.Is without coupling to HTTP semantics.
var (
	ErrNotFound                 = errors.New("user: not found")
	ErrEmailExists              = errors.New("user: email already exists")
	ErrInvalidCredentials       = errors.New("user: invalid credentials")
	ErrCurrentPasswordIncorrect = errors.New("user: current password incorrect")
	ErrPasswordPolicyViolation  = errors.New("user: password violates policy")
	ErrPasswordUnchanged        = errors.New("user: new password equals current password")
)
