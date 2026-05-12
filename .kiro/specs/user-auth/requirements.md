# Requirements Document

## Project Description (Input)

Replace the existing single static `X-API-Token` middleware with username + password authentication on the thesis backend. Operator (currently one researcher, with the possibility of a second equal-role user later) authenticates with email + password and receives a short-lived stateless access JWT in `Authorization: Bearer` plus a longer-lived stateless refresh JWT in an `HttpOnly`+`Secure`+`SameSite=Strict` cookie scoped to the refresh endpoint. Protected `/api/*` routes accept the access token via a new JWT middleware; the static-token middleware is deleted, not coexistent. Passwords are bcrypt hashed. Users are seeded via the existing `cmd/seed` entry point — there is no public registration endpoint, no roles, and no email/2FA/OAuth scope. Endpoints in scope: `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `GET /auth/session`, `POST /auth/change-password`. Tokens are stateless with no rotation or server-side revocation — short refresh TTL is the accepted mitigation. Stack stays on Go 1.25 + Gin + GORM/SQLite + Viper + slog per existing steering. Full discovery in `brief.md`.

## Introduction

The thesis backend currently authenticates every `/api/*` request with a single static token compared via the `X-API-Token` header. That mechanism cannot identify which operator is calling, cannot be rotated without redeploy, cannot be revoked, and offers no path to adding a second operator. This feature replaces the static-token middleware with username + password authentication backed by stateless JSON Web Tokens, supports an in-app password change, exposes the active session for client display, and allows additional equal-role operators to be added via the existing `cmd/seed` command. Refresh tokens live in an `HttpOnly` cookie scoped to a single refresh endpoint; access tokens are carried in an `Authorization: Bearer` header. After this change there is exactly one auth scheme: the static-token middleware is deleted, not toggled.

## Boundary Context

- **In scope**: Login (`POST /auth/login`), refresh (`POST /auth/refresh`), logout (`POST /auth/logout`), current session (`GET /auth/session`), password change (`POST /auth/change-password`); bcrypt password hashing; stateless JWT access and refresh tokens; JWT middleware protecting `/api/*`; seed-based user creation through `cmd/seed`; removal of the existing `X-API-Token` middleware and its product-steering description.
- **Out of scope**: Public registration endpoint; roles and role-based authorization; password reset by email or any out-of-band mechanism; email/SMS/2FA flows; OAuth or SSO; refresh-token rotation, reuse detection, or server-side revocation; brute-force rate limiting and account lockout; session listing or "log out everywhere"; machine-to-machine API keys; CSRF token plumbing beyond `SameSite=Strict` plus `Origin`/`Referer` checking on the refresh endpoint; Postgres migration; frontend code.
- **Adjacent expectations**: Existing controllers under `/api/*` (`arxiv`, `paper`, `extraction`, `analyzer`, `pdf-storage`, `llm-analyzer`) continue to function unchanged once the JWT middleware replaces the static-token middleware. The `cmd/seed` command already exists and is extended (not replaced) to support user provisioning. The integration-test harness at `tests/integration/setup/` is reused for new auth integration tests rather than duplicated.

## Requirements

### Requirement 1: User Login with Email and Password

**Objective:** As an operator, I want to authenticate with my email and password and receive a short-lived access token plus a refresh cookie, so that I can call protected endpoints without re-entering my password on every request.

#### Acceptance Criteria

1. When the operator submits `POST /auth/login` with a body containing an email and matching password for an existing user, the User Auth Module shall return HTTP 200 with an access token in the response body and a refresh-token cookie set on the response.
2. The User Auth Module shall set the refresh-token cookie with `HttpOnly` enabled, `SameSite=Strict`, a `Path` scoped to the refresh endpoint, and no `Domain` attribute.
3. Where the operator's environment is configured to permit non-TLS local development, the User Auth Module shall expose configuration that allows the refresh cookie's `Secure` flag to be disabled for that environment only; the flag shall be enabled by default in any other environment.
4. If the submitted email does not correspond to a stored user, the User Auth Module shall return HTTP 401 with a generic invalid-credentials error envelope.
5. If the submitted password does not match the stored hash for the given email, the User Auth Module shall return HTTP 401 with the same generic invalid-credentials error envelope used for an unknown email, so that the response does not distinguish between unknown email and wrong password.
6. If the request body is missing the email or password field, or the email is not a syntactically valid address, the User Auth Module shall return HTTP 400 with a validation error envelope.
7. The access token issued by login shall expire 15 minutes after issuance.
8. The refresh token issued by login shall expire 24 hours after issuance.
9. When login succeeds, the User Auth Module shall record an `info`-level log line containing the user id and request id, and no credential material.
10. When login fails for any reason, the User Auth Module shall record a `warn`-level log line containing the submitted email and request id, and no credential material.

### Requirement 2: Authenticated Access to Protected Routes

**Objective:** As an operator, I want every `/api/*` route to require a valid access token, so that the backend cannot be called by anonymous clients.

#### Acceptance Criteria

1. When a request to any `/api/*` route carries a valid, non-expired access token in the `Authorization: Bearer <token>` header, the User Auth Module shall allow the request to proceed to the downstream handler.
2. If a request to any `/api/*` route is missing the `Authorization` header, the User Auth Module shall reject the request with HTTP 401 and an error envelope whose reason code identifies a missing credential.
3. If the `Authorization` header is present but lacks the `Bearer ` prefix or contains a malformed token, the User Auth Module shall reject the request with HTTP 401 and an error envelope whose reason code identifies a malformed credential.
4. If the access token signature does not verify against the configured signing key, the User Auth Module shall reject the request with HTTP 401 and an error envelope whose reason code identifies an invalid credential.
5. If the access token is expired, the User Auth Module shall reject the request with HTTP 401 and an error envelope whose reason code identifies an expired access token, distinct from the reason code used for missing, malformed, or invalid tokens, so that a client can decide whether to retry via refresh or restart from login.
6. When the access token is valid, the authenticated user's id shall be available to downstream handlers for the remainder of the request lifecycle without those handlers re-validating the token.
7. The User Auth Module shall validate access tokens by signature and expiry alone and shall not consult any database or external service to authenticate an `/api/*` request.

### Requirement 3: Refresh Access Token via Refresh Cookie

**Objective:** As an operator, I want to obtain a new access token using only my refresh cookie, so that my session can continue past the 15-minute access-token lifetime without re-entering my password.

#### Acceptance Criteria

1. When the operator submits `POST /auth/refresh` with a valid, non-expired refresh-token cookie and an `Origin` or `Referer` header matching the configured backend origin, the User Auth Module shall return HTTP 200 with a new access token in the response body.
2. If the refresh-token cookie is missing from the request, the User Auth Module shall return HTTP 401 with an error envelope whose reason code identifies a missing refresh token.
3. If the refresh-token signature does not verify against the configured refresh signing key, the User Auth Module shall return HTTP 401 with an error envelope whose reason code identifies an invalid refresh token.
4. If the refresh token is expired, the User Auth Module shall return HTTP 401 with an error envelope whose reason code identifies an expired refresh token, distinct from missing or invalid, so that the client can prompt for re-login.
5. If both the `Origin` and `Referer` headers are absent, or one is present but does not match the configured backend origin, the User Auth Module shall return HTTP 403 and shall not issue a new access token.
6. The User Auth Module shall not issue a new refresh token on `POST /auth/refresh` (refresh tokens are not rotated).
7. The User Auth Module shall validate refresh tokens by signature and expiry alone and shall not consult any database or external service.
8. The new access token issued by `POST /auth/refresh` shall expire 15 minutes after issuance.
9. The refresh signing key shall be distinct from the access signing key.

### Requirement 4: Logout

**Objective:** As an operator, I want a logout endpoint that clears my refresh cookie, so that the cookie cannot be replayed from the same browser after I leave the workstation.

#### Acceptance Criteria

1. When the operator submits `POST /auth/logout`, the User Auth Module shall return HTTP 204 and shall instruct the client to delete the refresh-token cookie by setting a cookie of the same name with an expired `Max-Age` and the same `Path` used at issuance.
2. The User Auth Module shall accept `POST /auth/logout` without requiring an access token in the `Authorization` header, so that logout works even when the access token has expired.
3. The User Auth Module shall accept `POST /auth/logout` whether or not a refresh-token cookie is present on the request, and shall return HTTP 204 in either case.
4. The User Auth Module shall not retain any server-side record of logout (no revocation list, no database write), because tokens are stateless and the cookie deletion is the only observable side effect.

### Requirement 5: Current Session Lookup

**Objective:** As an operator (and any client UI), I want to query the currently authenticated user, so that I can display who is logged in and detect when my access token has expired.

#### Acceptance Criteria

1. When a request to `GET /auth/session` carries a valid access token, the User Auth Module shall return HTTP 200 with a response body containing the user's id, email, and account creation timestamp.
2. If the access token is missing, malformed, expired, or signature-invalid on `GET /auth/session`, the User Auth Module shall reject the request with the same status codes and reason codes used in Requirement 2.
3. The User Auth Module shall not include a password hash, refresh token, signing key, or any other credential material in the `GET /auth/session` response.

### Requirement 6: Change Password

**Objective:** As an operator, I want to change my password in-app by supplying my current password and a new password, so that I can rotate a credential without editing the database.

#### Acceptance Criteria

1. When a request to `POST /auth/change-password` carries a valid access token and a body containing a correct current password plus a new password that satisfies the password policy, the User Auth Module shall replace the stored password hash for the authenticated user and return HTTP 204.
2. If the access token is missing, expired, or signature-invalid, the User Auth Module shall reject the request with HTTP 401 using the same reason codes as Requirement 2.
3. If the supplied current password does not match the stored hash for the authenticated user, the User Auth Module shall return HTTP 400 with a `current-password-incorrect` reason code.
4. If the new password fails any rule of the password policy, the User Auth Module shall return HTTP 400 with a `password-policy-violation` reason code and a message identifying the violated rule.
5. The User Auth Module shall require a new password of at least 12 characters and at most 72 bytes, with no character-class composition rules.
6. If the new password is byte-equal to the current password, the User Auth Module shall return HTTP 400 with a `password-unchanged` reason code.
7. After a successful password change, the User Auth Module shall continue to accept access tokens issued before the change until they reach their normal expiry, because tokens are stateless and there is no server-side revocation.

### Requirement 7: User Provisioning via Seed Command

**Objective:** As the system operator, I want to create users by running the existing seed command with an email and password, so that I can bootstrap the first user and add a second equal-role user later without exposing a public registration endpoint.

#### Acceptance Criteria

1. When the operator runs the seed command with an email and password and no user with that email exists, the seed command shall create a single user row with the password stored as a bcrypt hash and shall exit 0 with a success message on stdout.
2. When the operator runs the seed command with an email that already exists, the seed command shall leave the existing user unchanged and shall exit 0 with an idempotent no-op message on stdout, so that re-runs are safe.
3. If the supplied password fails the password policy (Requirement 6.5), the seed command shall exit non-zero with a clear error message and shall not create or modify any user row.
4. If the supplied email is not a syntactically valid address, the seed command shall exit non-zero with a clear error message and shall not create or modify any user row.
5. The User Auth Module shall enforce email uniqueness at the persistence layer so that no two users can share an email address, regardless of how the row was created.
6. The seed command shall be the sole mechanism through which users are created in this feature; no HTTP endpoint introduced by this feature shall create users.

### Requirement 8: Replacement of the Static API Token Middleware

**Objective:** As the maintainer, I want the legacy `X-API-Token` middleware removed entirely once JWT auth is in place, so that the codebase has exactly one auth scheme and the static shared-secret model cannot be re-enabled by accident.

#### Acceptance Criteria

1. The User Auth Module shall protect every route currently mounted under the `/api` group with the new JWT middleware introduced by this feature.
2. The User Auth Module shall ensure that no route in the running server accepts the `X-API-Token` header as a valid credential.
3. The `X-API-Token` middleware source file and every reference to it in route wiring shall be removed from the codebase (no retention behind feature flags or environment toggles).
4. The product-steering description of the auth model shall be updated so that the documented state matches the running state after this feature ships.
5. The `/auth/*` routes introduced by this feature shall not require an access token for `POST /auth/login`, `POST /auth/refresh`, or `POST /auth/logout`; `GET /auth/session` and `POST /auth/change-password` shall require a valid access token.

### Requirement 9: Password Hashing Posture

**Objective:** As the security-conscious operator, I want passwords stored in a form that resists offline cracking, so that a database leak does not immediately yield plaintext credentials.

#### Acceptance Criteria

1. The User Auth Module shall store every user password as a bcrypt hash and shall never persist a plaintext password to any table, file, or log.
2. The User Auth Module shall use a bcrypt cost factor aligned with current published password-storage guidance for the year of implementation, chosen explicitly rather than relying on a library-supplied default.
3. The User Auth Module shall verify passwords using the bcrypt library's constant-time verification primitive and shall not implement its own comparison.
4. If a stored password hash is malformed or unreadable, the User Auth Module shall treat the login attempt as a credentials failure (Requirement 1.5) and shall not expose the underlying cause to the caller.
5. The User Auth Module shall reject any password input longer than 72 bytes at the validation layer with HTTP 400 and a `password-too-long` reason code, rather than silently truncating to bcrypt's input limit.

### Requirement 10: Credential Confidentiality in Logs and Responses

**Objective:** As the security-conscious operator, I want to ensure that no credential material (plaintext passwords or signed tokens) appears in logs, error envelopes, or any response that exceeds its intended channel, so that operational artifacts cannot be turned into a credential leak.

#### Acceptance Criteria

1. The User Auth Module shall never write a plaintext password to any log line, structured-trace field, error-envelope body, or response body emitted from any layer.
2. The User Auth Module shall never write a complete or partial access token, refresh token, or signing key to any log line, structured-trace field, or error-envelope body.
3. Where the platform's HTTP middleware logs request bodies for diagnostic purposes, the User Auth Module shall ensure password fields and token fields in those bodies are redacted before the log line is emitted.
4. The User Auth Module shall not return a stored password hash in any response body of any endpoint introduced by this feature.
5. The seed command shall not echo the supplied password to stdout, stderr, or any log destination.
