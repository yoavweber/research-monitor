# Requirements Document

## Project Description (Input)

Replace the existing single static `X-API-Token` middleware with username + password authentication on the thesis backend. Operator (currently one researcher, with the possibility of a second equal-role user later) authenticates with email + password and receives a stateless JWT in `Authorization: Bearer`. Protected `/api/*` routes accept the JWT via a new auth middleware; the static-token middleware is deleted, not coexistent. Passwords are bcrypt hashed. Users are seeded via the existing `cmd/seed` entry point — there is no public registration endpoint, no roles, and no email/2FA/OAuth scope. Endpoints in scope: `POST /auth/login`, `GET /auth/session`, `POST /auth/change-password`. There is no refresh-token endpoint, no cookie, and no logout endpoint — the token is short enough that re-login is acceptable when it expires. Tokens are stateless: no rotation, no server-side revocation. Stack stays on Go 1.25 + Gin + GORM/SQLite + Viper + slog per existing steering. Full discovery in `brief.md`.

## Introduction

The thesis backend currently authenticates every `/api/*` request with a single static token compared via the `X-API-Token` header. That mechanism cannot identify which operator is calling, cannot be rotated without redeploy, cannot be revoked, and offers no path to adding a second operator. This feature replaces the static-token middleware with username + password authentication backed by a stateless JSON Web Token, supports an in-app password change, exposes the active session for client display, and allows additional equal-role operators to be added via the existing `cmd/seed` command. The token is carried in an `Authorization: Bearer` header on every authenticated request. After this change there is exactly one auth scheme: the static-token middleware is deleted, not toggled.

## Boundary Context

- **In scope**: Login (`POST /auth/login`), current session (`GET /auth/session`), password change (`POST /auth/change-password`); bcrypt password hashing; a stateless JWT issued at login; JWT middleware protecting `/api/*`; seed-based user creation through `cmd/seed`; removal of the existing `X-API-Token` middleware and its product-steering description.
- **Out of scope**: Public registration endpoint; roles and role-based authorization; password reset by email or any out-of-band mechanism; email/SMS/2FA flows; OAuth or SSO; refresh tokens, refresh cookies, logout endpoints, CSRF token plumbing, `Origin`/`Referer` defense-in-depth; brute-force rate limiting and account lockout; session listing or "log out everywhere"; machine-to-machine API keys; Postgres migration; frontend code.
- **Adjacent expectations**: Existing controllers under `/api/*` (`arxiv`, `paper`, `extraction`, `analyzer`, `pdf-storage`, `llm-analyzer`) continue to function unchanged once the JWT middleware replaces the static-token middleware. The `cmd/seed` command already exists and is extended (not replaced) to support user provisioning. The integration-test harness at `tests/integration/setup/` is reused for new auth integration tests rather than duplicated.

## Requirements

### Requirement 1: User Login with Email and Password

**Objective:** As an operator, I want to authenticate with my email and password and receive a JWT, so that I can call protected endpoints without re-entering my password on every request.

#### Acceptance Criteria

1. When the operator submits `POST /auth/login` with a body containing an email and matching password for an existing user, the User Auth Module shall return HTTP 200 with the issued JWT and its expiry time in the response body.
2. If the submitted email does not correspond to a stored user, the User Auth Module shall return HTTP 401 with a generic invalid-credentials error envelope.
3. If the submitted password does not match the stored hash for the given email, the User Auth Module shall return HTTP 401 with the same generic invalid-credentials error envelope used for an unknown email, so that the response does not distinguish between unknown email and wrong password.
4. If the request body is missing the email or password field, or the email is not a syntactically valid address, the User Auth Module shall return HTTP 400 with a validation error envelope.
5. The JWT issued by login shall expire 24 hours after issuance.
6. When login succeeds, the User Auth Module shall record an `info`-level log line containing the user id and request id, and no credential material.
7. When login fails for any reason, the User Auth Module shall record a `warn`-level log line containing the submitted email and request id, and no credential material.

### Requirement 2: Authenticated Access to Protected Routes

**Objective:** As an operator, I want every `/api/*` route to require a valid JWT, so that the backend cannot be called by anonymous clients.

#### Acceptance Criteria

1. When a request to any `/api/*` route carries a valid, non-expired JWT in the `Authorization: Bearer <token>` header, the User Auth Module shall allow the request to proceed to the downstream handler.
2. If a request to any `/api/*` route is missing the `Authorization` header, the User Auth Module shall reject the request with HTTP 401 and an error envelope whose reason code identifies a missing credential.
3. If the `Authorization` header is present but lacks the `Bearer ` prefix or contains a malformed token, the User Auth Module shall reject the request with HTTP 401 and an error envelope whose reason code identifies a malformed credential.
4. If the JWT signature does not verify against the configured signing key, the User Auth Module shall reject the request with HTTP 401 and an error envelope whose reason code identifies an invalid credential.
5. If the JWT is expired, the User Auth Module shall reject the request with HTTP 401 and an error envelope whose reason code identifies an expired token, distinct from missing, malformed, or invalid, so that a client can prompt for re-login.
6. When the JWT is valid, the authenticated user's id shall be available to downstream handlers for the remainder of the request lifecycle without those handlers re-validating the token.
7. The User Auth Module shall validate JWTs by signature and expiry alone and shall not consult any database or external service to authenticate an `/api/*` request.

### Requirement 3: Current Session Lookup

**Objective:** As an operator (and any client UI), I want to query the currently authenticated user, so that I can confirm who is logged in and detect when my token has expired.

#### Acceptance Criteria

1. When a request to `GET /auth/session` carries a valid JWT, the User Auth Module shall return HTTP 200 with a response body containing the user's id, email, and account creation timestamp.
2. If the JWT is missing, malformed, expired, or signature-invalid on `GET /auth/session`, the User Auth Module shall reject the request with the same status codes and reason codes used in Requirement 2.
3. The User Auth Module shall not include a password hash, signing key, or any other credential material in the `GET /auth/session` response.

### Requirement 4: Change Password

**Objective:** As an operator, I want to change my password in-app by supplying my current password and a new password, so that I can rotate a credential without editing the database.

#### Acceptance Criteria

1. When a request to `POST /auth/change-password` carries a valid JWT and a body containing a correct current password plus a new password that satisfies the password policy, the User Auth Module shall replace the stored password hash for the authenticated user and return HTTP 204.
2. If the JWT is missing, expired, or signature-invalid, the User Auth Module shall reject the request with HTTP 401 using the same reason codes as Requirement 2.
3. If the supplied current password does not match the stored hash for the authenticated user, the User Auth Module shall return HTTP 400 with a `current-password-incorrect` reason code.
4. If the new password fails any rule of the password policy, the User Auth Module shall return HTTP 400 with a `password-policy-violation` reason code and a message identifying the violated rule.
5. The User Auth Module shall require a new password of at least 12 characters and at most 72 bytes, with no character-class composition rules.
6. If the new password is byte-equal to the current password, the User Auth Module shall return HTTP 400 with a `password-unchanged` reason code.
7. After a successful password change, the User Auth Module shall continue to accept JWTs issued before the change until they reach their normal expiry, because tokens are stateless and there is no server-side revocation.

### Requirement 5: User Provisioning via Seed Command

**Objective:** As the system operator, I want to create users by running the existing seed command with an email and password, so that I can bootstrap the first user and add a second equal-role user later without exposing a public registration endpoint.

#### Acceptance Criteria

1. When the operator runs the seed command with an email and password and no user with that email exists, the seed command shall create a single user row with the password stored as a bcrypt hash and shall exit 0 with a success message on stdout.
2. When the operator runs the seed command with an email that already exists, the seed command shall leave the existing user unchanged and shall exit 0 with an idempotent no-op message on stdout, so that re-runs are safe.
3. If the supplied password fails the password policy (Requirement 4.5), the seed command shall exit non-zero with a clear error message and shall not create or modify any user row.
4. If the supplied email is not a syntactically valid address, the seed command shall exit non-zero with a clear error message and shall not create or modify any user row.
5. The User Auth Module shall enforce email uniqueness at the persistence layer so that no two users can share an email address, regardless of how the row was created.
6. The seed command shall be the sole mechanism through which users are created in this feature; no HTTP endpoint introduced by this feature shall create users.

### Requirement 6: Replacement of the Static API Token Middleware

**Objective:** As the maintainer, I want the legacy `X-API-Token` middleware removed entirely once JWT auth is in place, so that the codebase has exactly one auth scheme and the static shared-secret model cannot be re-enabled by accident.

#### Acceptance Criteria

1. The User Auth Module shall protect every route currently mounted under the `/api` group with the new JWT middleware introduced by this feature.
2. The User Auth Module shall ensure that no route in the running server accepts the `X-API-Token` header as a valid credential.
3. The `X-API-Token` middleware source file and every reference to it in route wiring shall be removed from the codebase (no retention behind feature flags or environment toggles).
4. The product-steering description of the auth model shall be updated so that the documented state matches the running state after this feature ships.
5. `POST /auth/login` shall not require a JWT; `GET /auth/session` and `POST /auth/change-password` shall require a valid JWT.

### Requirement 7: Password Hashing Posture

**Objective:** As the security-conscious operator, I want passwords stored in a form that resists offline cracking, so that a database leak does not immediately yield plaintext credentials.

#### Acceptance Criteria

1. The User Auth Module shall store every user password as a bcrypt hash and shall never persist a plaintext password to any table, file, or log.
2. The User Auth Module shall use a bcrypt cost factor aligned with current published password-storage guidance for the year of implementation, chosen explicitly rather than relying on a library-supplied default.
3. The User Auth Module shall verify passwords using the bcrypt library's constant-time verification primitive and shall not implement its own comparison.
4. If a stored password hash is malformed or unreadable, the User Auth Module shall treat the login attempt as a credentials failure (Requirement 1.3) and shall not expose the underlying cause to the caller.
5. The User Auth Module shall reject any password input longer than 72 bytes at the validation layer with HTTP 400 and a `password-too-long` reason code, rather than silently truncating to bcrypt's input limit.

### Requirement 8: Credential Confidentiality in Logs and Responses

**Objective:** As the security-conscious operator, I want to ensure that no credential material (plaintext passwords or signed tokens) appears in logs, error envelopes, or any response that exceeds its intended channel, so that operational artifacts cannot be turned into a credential leak.

#### Acceptance Criteria

1. The User Auth Module shall never write a plaintext password to any log line, structured-trace field, error-envelope body, or response body emitted from any layer.
2. The User Auth Module shall never write a complete or partial JWT or signing key to any log line, structured-trace field, or error-envelope body.
3. Where the platform's HTTP middleware logs request bodies for diagnostic purposes, the User Auth Module shall ensure password fields and token fields in those bodies are redacted before the log line is emitted.
4. The User Auth Module shall not return a stored password hash in any response body of any endpoint introduced by this feature.
5. The seed command shall not echo the supplied password to stdout, stderr, or any log destination.
