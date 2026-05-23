package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/domain/user"
)

// requestIDContextKey is the context-value key the HTTP controller layer uses
// to forward the per-request id (set by middleware.RequestID) down into the
// use-case. The middleware writes the id to the Gin context only; the
// controller is responsible for lifting it into context.Context via
// application.WithRequestID before invoking any use-case method. Exported via
// WithRequestID / read via requestIDFromContext so the type stays unexported
// and the wire convention is concentrated in one file.
type requestIDContextKey struct{}

// WithRequestID returns a context carrying the request id. The controller
// calls this once per request after RequestID middleware runs, so use-case
// log lines that key off request_id stay correlated with the access log.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey{}, id)
}

// requestIDFromContext returns the request id stored by WithRequestID, or
// the empty string if none is set. The empty string is intentional — log
// fields are emitted as-is and downstream slog handlers omit empty string
// values consistently with the rest of the codebase.
func requestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDContextKey{}).(string); ok {
		return id
	}
	return ""
}

// userUseCase orchestrates authentication for the operator: login, current
// session lookup, and in-app password rotation. It depends only on domain
// ports — the repository and the cross-cutting hasher / signer / clock /
// logger interfaces — so this layer can be tested with real adapters over an
// in-memory database without booting the HTTP server.
//
// Failure handling pivots on a single principle: the wire response for any
// authentication-time failure must be indistinguishable from the
// unknown-email case. The use-case folds both password mismatches and
// unreadable stored hashes into user.ErrInvalidCredentials, while preserving
// the structured-log signal so operators can tell the failure modes apart in
// the log stream.
type userUseCase struct {
	repo   user.Repository
	hasher shared.PasswordHasher
	signer shared.TokenSigner
	clock  shared.Clock
	log    shared.Logger
}

// NewUserUseCase wires the auth use-case. The constructor returns the domain
// interface so callers depend on the port, not the implementation.
func NewUserUseCase(
	repo user.Repository,
	hasher shared.PasswordHasher,
	signer shared.TokenSigner,
	clock shared.Clock,
	log shared.Logger,
) user.UseCase {
	return &userUseCase{
		repo:   repo,
		hasher: hasher,
		signer: signer,
		clock:  clock,
		log:    log,
	}
}

// Login authenticates a user by email + password and returns a fresh JWT.
// Validation runs before the repository call so a malformed DTO never
// reaches the database; the returned *shared.HTTPError is propagated as-is
// for the error envelope middleware to render.
func (uc *userUseCase) Login(ctx context.Context, req user.LoginRequest) (user.LoginResult, error) {
	if err := req.Validate(); err != nil {
		return user.LoginResult{}, err
	}

	u, err := uc.repo.FindByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			uc.log.WarnContext(ctx, "auth.login.failed",
				"reason", user.ReasonInvalidCredentials,
				"email", req.Email,
				"request_id", requestIDFromContext(ctx),
			)
			return user.LoginResult{}, user.ErrInvalidCredentials
		}
		return user.LoginResult{}, err
	}

	if err := uc.hasher.Verify(req.Password, u.PasswordHash); err != nil {
		// Two paths collapse here: a real password mismatch
		// (shared.ErrHashMismatch) and a corrupt/unreadable stored hash
		// (anything else). The wire response stays invalid_credentials in
		// both cases so storage anomalies never surface as 500; the log
		// keeps the two modes distinguishable for operators.
		reason := user.ReasonInvalidCredentials
		logFields := []any{
			"reason", reason,
			"email", req.Email,
			"request_id", requestIDFromContext(ctx),
		}
		if !errors.Is(err, shared.ErrHashMismatch) {
			logFields = []any{
				"reason", "hash_unreadable",
				"user_id", u.ID.String(),
				"email", req.Email,
				"request_id", requestIDFromContext(ctx),
			}
		}
		uc.log.WarnContext(ctx, "auth.login.failed", logFields...)
		return user.LoginResult{}, user.ErrInvalidCredentials
	}

	token, expiresAt, err := uc.signer.Issue(u.ID.String())
	if err != nil {
		return user.LoginResult{}, err
	}

	uc.log.InfoContext(ctx, "auth.login.ok",
		"user_id", u.ID.String(),
		"request_id", requestIDFromContext(ctx),
	)

	return user.LoginResult{Token: token, ExpiresAt: expiresAt, User: u}, nil
}

// Session resolves the authenticated user from a verified subject id. The
// controller layer strips PasswordHash before serialization (the
// SessionResponse DTO has no such field); this layer returns the aggregate.
func (uc *userUseCase) Session(ctx context.Context, userID uuid.UUID) (*user.User, error) {
	return uc.repo.FindByID(ctx, userID)
}

// ChangePassword rotates the stored hash after re-verifying the current
// password. Pre-change tokens remain valid until their normal expiry: the
// JWT model is stateless and there is no server-side revocation.
func (uc *userUseCase) ChangePassword(ctx context.Context, userID uuid.UUID, req user.ChangePasswordRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	u, err := uc.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := uc.hasher.Verify(req.CurrentPassword, u.PasswordHash); err != nil {
		// Corrupt/unreadable hash is folded into the same wire failure as a
		// wrong password so a storage anomaly never surfaces as 500. The log
		// keeps the underlying mode visible for operators.
		if !errors.Is(err, shared.ErrHashMismatch) {
			uc.log.WarnContext(ctx, "auth.change_password.failed",
				"reason", "hash_unreadable",
				"user_id", u.ID.String(),
				"request_id", requestIDFromContext(ctx),
			)
		}
		return user.ErrCurrentPasswordIncorrect
	}

	if req.NewPassword == req.CurrentPassword {
		return user.ErrPasswordUnchanged
	}

	newHash, err := uc.hasher.Hash(req.NewPassword)
	if err != nil {
		return err
	}

	if err := uc.repo.UpdatePasswordHash(ctx, u.ID, newHash); err != nil {
		return err
	}

	uc.log.InfoContext(ctx, "auth.change_password.ok",
		"user_id", u.ID.String(),
		"request_id", requestIDFromContext(ctx),
	)
	return nil
}
