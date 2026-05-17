package user

import "errors"

// Reason codes surfaced under error.details.reason in HTTP responses. Producers
// (DTO validators, use-case, controller) and consumers (tests, frontend) must
// reference these constants so a single source of truth defines the wire
// contract. Mixed hyphen/underscore separators are preserved from design.md.
const (
	ReasonValidationFailed         = "validation_failed"
	ReasonPasswordTooLong          = "password_too_long"
	ReasonPasswordPolicyViolation  = "password-policy-violation"
	ReasonInvalidCredentials       = "invalid_credentials"
	ReasonCurrentPasswordIncorrect = "current-password-incorrect"
	ReasonPasswordUnchanged        = "password-unchanged"
)

var (
	ErrNotFound                 = errors.New("user: not found")
	ErrEmailExists              = errors.New("user: email already exists")
	ErrInvalidCredentials       = errors.New("user: invalid credentials")
	ErrCurrentPasswordIncorrect = errors.New("user: current password incorrect")
	ErrPasswordPolicyViolation  = errors.New("user: password violates policy")
	ErrPasswordUnchanged        = errors.New("user: new password equals current password")
)
