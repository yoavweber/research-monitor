package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// JWTConfig wires the signing key, token lifetime, and time source into
// JWTTokenService. Bootstrap supplies the production values from env;
// tests inject a frozen clock so expiry checks are deterministic.
type JWTConfig struct {
	Secret []byte
	TTL    time.Duration
	Clock  shared.Clock
}

// JWTTokenService implements both shared.TokenSigner and shared.TokenValidator
// over HS256 with a single signing key. Claims are minimal — sub, iat, exp —
// because the middleware only needs to identify the operator; richer claims
// would be coupling, not capability.
type JWTTokenService struct {
	cfg JWTConfig
}

// NewJWTTokenService constructs the service. Env validation in bootstrap
// already guarantees a ≥32-byte secret and a non-zero TTL, so the constructor
// stays error-free; mis-wiring would be a programmer error caught at startup.
func NewJWTTokenService(cfg JWTConfig) *JWTTokenService {
	return &JWTTokenService{cfg: cfg}
}

// Issue produces a signed HS256 token for the given subject and returns its
// absolute expiry. An empty subject is rejected before the library so the
// failure mode is a clear validation error rather than an opaque library one.
func (s *JWTTokenService) Issue(subject string) (string, time.Time, error) {
	if subject == "" {
		return "", time.Time{}, errors.New("jwt token service: subject is empty")
	}

	iat := s.cfg.Clock.Now().UTC()
	exp := iat.Add(s.cfg.TTL)

	claims := jwt.RegisteredClaims{
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(iat),
		ExpiresAt: jwt.NewNumericDate(exp),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.cfg.Secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt token service: sign: %w", err)
	}
	return signed, exp, nil
}

// Verify parses and validates the token, returning the embedded subject on
// success. Errors are mapped onto shared.ErrToken* sentinels so the middleware
// can pick the right reason code without depending on the JWT library's error
// taxonomy. token.Valid is checked alongside the parse error to defend against
// CVE-2024-51744, where a malformed-but-parseable token could otherwise pass.
func (s *JWTTokenService) Verify(token string) (string, error) {
	keyFunc := func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, shared.ErrTokenSignatureInvalid
		}
		return s.cfg.Secret, nil
	}

	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(
		token,
		claims,
		keyFunc,
		jwt.WithTimeFunc(s.cfg.Clock.Now),
	)
	if err != nil {
		return "", mapJWTError(err)
	}
	if parsed == nil || !parsed.Valid {
		return "", shared.ErrTokenMalformed
	}
	return claims.Subject, nil
}

// mapJWTError collapses the library's error tree onto the three sentinels the
// middleware understands. Expiry is checked first because the library wraps
// it under a generic "token is invalid" parent, and we want the more specific
// signal to win. The keyFunc returns shared.ErrTokenSignatureInvalid when the
// signing method is not HS256 (alg=none attack class); we look for that
// sentinel directly because the library wraps it under ErrTokenUnverifiable.
func mapJWTError(err error) error {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return shared.ErrTokenExpired
	case errors.Is(err, shared.ErrTokenSignatureInvalid),
		errors.Is(err, jwt.ErrTokenSignatureInvalid),
		errors.Is(err, jwt.ErrSignatureInvalid):
		return shared.ErrTokenSignatureInvalid
	case errors.Is(err, jwt.ErrTokenMalformed):
		return shared.ErrTokenMalformed
	default:
		return shared.ErrTokenMalformed
	}
}
