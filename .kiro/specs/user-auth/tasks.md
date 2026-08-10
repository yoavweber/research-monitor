# Implementation Plan

> **Test doubles**: per [testing.md](../../steering/testing.md), this plan uses real implementations (in-memory SQLite, real bcrypt at cost 4 in tests, real JWT signer with test keys) instead of hand-written fakes. No mock files are introduced for `PasswordHasher`, `TokenSigner`, or `TokenValidator`.
>
> **Package naming**: the existing codebase uses `<entity>persist` as the import alias for `infrastructure/persistence/<entity>` packages — bare `user` for `internal/domain/user`, `userpersist` for `internal/infrastructure/persistence/user`.

## 1. Foundation: dependencies, ports, env, schema

- [x] 1.1 Add JWT v5 and bcrypt module dependencies
  - Add `github.com/golang-jwt/jwt/v5` pinned at `v5.2.2` or later (CVE-2025-30204 fix).
  - Add `golang.org/x/crypto/bcrypt`.
  - Run `go mod tidy`; commit `go.mod` and `go.sum`.
  - **Observable**: `go build ./...` succeeds and both modules appear in the `require` block.
  - _Requirements: 7.1, 7.2_
  - _Boundary: go.mod_

- [x] 1.2 (P) Add cross-cutting auth ports to the shared domain
  - Append `PasswordHasher`, `TokenSigner` (single `Issue` method), `TokenValidator` (single `Verify` method) interfaces to [internal/domain/shared/ports.go](../../../internal/domain/shared/ports.go).
  - Append `ErrHashMismatch`, `ErrTokenMalformed`, `ErrTokenSignatureInvalid`, `ErrTokenExpired` sentinels to `internal/domain/shared/errors.go`.
  - **Observable**: the three interfaces and four sentinels compile under `go build ./internal/domain/shared/...` with no concrete implementations yet.
  - _Requirements: 7.1, 7.3, 2.4, 2.5_
  - _Boundary: domain/shared_

- [x] 1.3 (P) Add auth env-configuration fields and startup validation
  - Add two fields to the env struct in [internal/bootstrap/env.go](../../../internal/bootstrap/env.go): `JWTSecret` (env `AUTH_JWT_SECRET`, **required**) and `JWTTTL` (env `AUTH_JWT_TTL`, default `24h`).
  - Use `v.SetDefault` for the TTL. Bootstrap fails fast at startup when `AUTH_JWT_SECRET` is empty or shorter than 32 bytes.
  - The existing `APIToken` field and its required check **stay in place** until task 4.5 deletes them.
  - **Observable**: starting the app without `AUTH_JWT_SECRET` yields a clear startup error naming the variable; setting it to a 32-byte+ value starts cleanly.
  - _Requirements: 1.5_
  - _Boundary: bootstrap/env_

- [x] 1.4 (P) Create the User domain aggregate
  - Create `internal/domain/user/{model.go, ports.go, requests.go, responses.go, errors.go}`.
  - `ports.go`: `user.UseCase` (`Login`, `Session`, `ChangePassword` — no `Logout`, no `Refresh`) and `user.Repository` (`FindByEmail`, `FindByID`, `Save`, `UpdatePasswordHash`).
  - `requests.go`: `LoginRequest`, `ChangePasswordRequest` with `Validate()` enforcing email syntax (`net/mail.ParseAddress`), 72-byte cap on every password, 12-byte minimum on new passwords.
  - `responses.go`: `SessionResponse{ID, Email, CreatedAt}`, `LoginResponse{AccessToken, ExpiresAt, User}`, `LoginResult{Token, ExpiresAt, User *User}`. No password-hash field on any type.
  - `errors.go`: six aggregate sentinels plus the `Reason*` constants for wire-format reason strings.
  - **Observable**: `go vet` and `go test ./internal/domain/user/...` pass; validator subtests cover all branches.
  - _Requirements: 1.4, 3.1, 3.3, 4.3, 4.5, 4.6, 5.5, 7.5_
  - _Boundary: domain/user_

- [x] 1.5 Add User persistence model and register with AutoMigrate
  - Create `internal/infrastructure/persistence/user/model.go` (package `user`, imported as `userpersist`) with GORM tags matching design.md and `TableName() = "users"`. `ToDomain`/`FromDomain` translate `uuid.UUID` ↔ `string`; malformed stored ID returns error.
  - Append `&userpersist.Model{}` to `AutoMigrate` in `persistence/migrate.go`, importing under the alias `userpersist`.
  - **Observable**: schema test verifies the table exists and rejects a duplicate email at the persistence layer (via `gorm.ErrDuplicatedKey`).
  - _Requirements: 5.5, 3.1_
  - _Boundary: infrastructure/persistence/user_

## 2. Core adapters

- [x] 2.1 (P) Bcrypt password hasher with colocated unit test
  - `internal/infrastructure/auth/bcrypt_hasher.go`: `NewBcryptHasher(cost int) shared.PasswordHasher`. `Hash` uses `bcrypt.GenerateFromPassword(pw, cost)`. `Verify` returns `shared.ErrHashMismatch` on `bcrypt.ErrMismatchedHashAndPassword`.
  - Bootstrap will construct with cost 12; tests with cost 4 for speed.
  - Colocated test: round-trip + mismatch returns `ErrHashMismatch` + empty-input rejection.
  - **Observable**: `go test ./internal/infrastructure/auth/...` passes.
  - _Requirements: 7.1, 7.2, 7.3_
  - _Boundary: infrastructure/auth_

- [x] 2.2 (P) JWT token service with colocated unit test
  - `internal/infrastructure/auth/jwt_token_service.go`: `JWTConfig{Secret []byte; TTL time.Duration; Clock shared.Clock}`. `NewJWTTokenService(cfg JWTConfig)` implements both `shared.TokenSigner` and `shared.TokenValidator`. Claims: `sub`, `iat`, `exp`. HS256. Always check `token.Valid` alongside the parse error (CVE-2024-51744).
  - Error mapping: `jwt.ErrTokenExpired` → `shared.ErrTokenExpired`; signature errors → `shared.ErrTokenSignatureInvalid`; parse/malformed → `shared.ErrTokenMalformed`.
  - Colocated test driving a deterministic clock: `Issue then Verify round-trips`; `expired token returns ErrTokenExpired`; `token whose signature does not match the key returns ErrTokenSignatureInvalid`; `token that parses but has Valid=false fails`.
  - **Observable**: `go test ./internal/infrastructure/auth/...` passes.
  - _Requirements: 1.5, 2.4, 2.5, 2.7_
  - _Boundary: infrastructure/auth_

- [x] 2.3 (P) User repository (GORM) with colocated unit test
  - `internal/infrastructure/persistence/user/repo.go`: `NewRepository(db *gorm.DB) user.Repository`. Maps `gorm.ErrRecordNotFound` → `user.ErrNotFound`; SQLite unique-constraint violation → `user.ErrEmailExists`. `UpdatePasswordHash` updates only the `password_hash` and `updated_at` columns explicitly.
  - Colocated test using `tests/testdb.New(t)`: `Save persists`; `Save returns ErrEmailExists on duplicate`; `FindByEmail` / `FindByID` hit and miss paths; `UpdatePasswordHash` replaces only the hash and bumps `updated_at`.
  - **Observable**: `go test ./internal/infrastructure/persistence/user/...` passes against a temp SQLite DB.
  - _Requirements: 5.5, 3.1, 4.1, 1.1_
  - _Boundary: infrastructure/persistence/user_

- [x] 2.4 (P) JWTAuth middleware with colocated unit test
  - `internal/http/middleware/jwt_auth.go`: `JWTAuth(validator shared.TokenValidator) gin.HandlerFunc`. Reads `Authorization: Bearer <token>`. Maps errors to `*shared.HTTPError` with reason codes `credentials_missing`, `credentials_malformed`, `invalid_access_token`, `expired_access_token`, `malformed_access_token`. On success: `c.Set("user_id", uuid.UUID(subject))` and `c.Next()`. Never consults a DB.
  - Colocated test using real `JWTTokenService` to mint tokens with controlled expiry: every error branch + happy path.
  - **Observable**: `go test ./internal/http/middleware/...` passes; assertions check both status and `details.reason`.
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_
  - _Boundary: http/middleware_
  - _Depends: 2.2_

## 3. Application + HTTP surface

- [x] 3.1 User use-case implementation with colocated unit test
  - `internal/application/user_usecase.go`: `NewUserUseCase(repo user.Repository, hasher shared.PasswordHasher, signer shared.TokenSigner, clock shared.Clock, log shared.Logger) user.UseCase`. Implements `Login`, `Session`, `ChangePassword` only.
  - Login flow: lookup by email; verify password; on either failure return `user.ErrInvalidCredentials` (same error for unknown email and wrong password). On match, issue a JWT and return `user.LoginResult`. Emit info log on success and warn log on failure; never log password material or token strings.
  - ChangePassword flow: load user by id; verify current password (wrong → `ErrCurrentPasswordIncorrect`); enforce policy via `Validate()`; reject byte-equal new password (`ErrPasswordUnchanged`); hash new password; call `UpdatePasswordHash`. Pre-change tokens stay valid until expiry (Req 4.7).
  - Treat any `Verify` error other than `shared.ErrHashMismatch` as `user.ErrInvalidCredentials` and log a warn event with `reason="hash_unreadable"` (Req 7.4).
  - Colocated test using **real** in-memory SQLite repo + real bcrypt (cost 4) + real `JWTTokenService` + a deterministic clock: covers account-enumeration parity, ChangePassword incorrect/policy/unchanged, post-change login succeeds, pre-change tokens still validate until expiry, info/warn logs on each branch.
  - **Observable**: `go test ./internal/application/...` passes.
  - _Requirements: 1.2, 1.3, 1.6, 1.7, 3.1, 4.1, 4.3, 4.4, 4.5, 4.6, 4.7, 7.4_
  - _Boundary: application/user_
  - _Depends: 2.1, 2.2, 2.3_

- [x] 3.2 Auth controller and swag annotations
  - Create `internal/http/controller/auth_controller.go` and `auth_responses.go`. Constructor: `NewAuthController(uc user.UseCase, validator shared.TokenValidator) *AuthController` — no cookie config, no origin config needed.
  - Login: bind body; call `req.Validate()`; call `uc.Login`. On success serialize `LoginResponse` (access token + expiry + user). No cookie, no Set-Cookie header.
  - Session: read `user_id` from the Gin context (set by `JWTAuth`); call `uc.Session`; return `SessionResponse`.
  - ChangePassword: read `user_id` from context; bind body; call `req.Validate()`; call `uc.ChangePassword`. 204 on success.
  - Error path everywhere: `c.Error(shared.NewHTTPError(...).WithReason(user.Reason...))`.
  - Never log access tokens or password fields. No request-body logger is registered in this spec; if a future spec adds one, redaction of `password`, `current_password`, `new_password`, `access_token` is required (Req 8.3 — deferred).
  - Swag annotations on every handler: `@Tags Auth`, `@Accept`/`@Produce`, `@Param`, `@Success`, one `@Failure` per distinct status, `@Security BearerAuth` on `/auth/session` and `/auth/change-password`, `@Router`.
  - **Observable**: handlers compile; each branch is exercised by the integration tests in Phase 5.
  - _Requirements: 1.1, 1.4, 1.5, 3.1, 3.2, 3.3, 4.1, 4.2, 4.3, 4.4, 4.7, 6.5, 8.1, 8.2, 8.3, 8.4_
  - _Boundary: http/controller_

- [x] 3.3 Auth router and `route.Deps` extension
  - Extend `route.Deps` in [internal/http/route/route.go](../../../internal/http/route/route.go) with `Hasher shared.PasswordHasher`, `Signer shared.TokenSigner`, `Validator shared.TokenValidator`, `JWTTTL time.Duration`, and an engine/root-group reference so the unauthenticated `POST /auth/login` can mount outside `/api`.
  - Create `internal/http/route/auth_route.go`: `AuthRouter(d Deps)` builds `repo := userpersist.NewRepository(d.DB)`, `uc := application.NewUserUseCase(repo, d.Hasher, d.Signer, d.Clock, d.Logger)`, `ctrl := controller.NewAuthController(uc)` — the controller does not need the validator at construction time; the middleware uses it. Mounts unauthenticated `POST /auth/login` on the root group; mounts `GET /auth/session` and `POST /auth/change-password` under `Group("/auth", middleware.JWTAuth(d.Validator))`.
  - **Observable**: `AuthRouter(d)` registers exactly three new endpoints; `go vet ./internal/http/route/...` is clean.
  - _Requirements: 6.5_
  - _Boundary: http/route_

## 4. Composition root and migration

- [x] 4.1 (P) SeedUser helper and `cmd/seed users` subcommand
  - Add `SeedUser(ctx context.Context, db *gorm.DB, hasher shared.PasswordHasher, log shared.Logger, email, plain string) error` to `internal/bootstrap/seed.go`. Validate email syntax and password policy. Build the repo via `userpersist.NewRepository(db)`. Hash via `hasher.Hash(plain)`. Call `repo.Save`; on `user.ErrEmailExists` log `"seed.user.skipped"` and return nil. Never log `plain` (Req 8.5).
  - Update `cmd/seed/main.go` to dispatch on `os.Args[1]`: no args → existing `SeedSources`; `users <email> <password>` → calls `SeedUser` constructing `NewBcryptHasher(12)`; `-h` documents both. Weak password / bad email exits non-zero without writing the row.
  - **Observable**: `seed users alice@example.com Password123abc!` creates a row idempotently; re-running prints `seed.user.skipped` and exits 0; `seed users alice@example.com weak` exits non-zero.
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 8.5_
  - _Boundary: bootstrap/seed, cmd/seed_

- [x] 4.2 (P) Wire JWT token service and mount `/auth` router additively
  - In `internal/bootstrap/app.go`, construct a single `JWTTokenService` from `AUTH_JWT_SECRET` + `AUTH_JWT_TTL` and pass it into `route.Deps` as both `Signer` and `Validator`. Also pass `NewBcryptHasher(12)` as `Hasher` and the TTL.
  - Call `route.AuthRouter(d)` on the **root** group (not `/api`) so login is reachable without a token. Authenticated routes live under the auth subgroup created by `AuthRouter`.
  - **Additive only**: the existing `middleware.APIToken(env.APIToken)` mount on `/api` stays in place; existing static-token tests keep passing.
  - **Observable**: `curl -X POST /auth/login` against a seeded user returns 200; existing `/api/*` tests with `X-API-Token` still pass.
  - _Requirements: 1.1, 3.1, 4.1_
  - _Boundary: bootstrap/app_

- [x] 4.3 Extend integration test harness with login-based authentication helpers
  - Update `tests/integration/setup/setup.go`: add `SeedTestUser(t, env)` inserting a deterministic test user via `userpersist.NewRepository(env.DB).Save`. Add `LoginAsTestUser(t, env) string` POSTing to `/auth/login` and returning the access token. Add `AuthorizedRequest(t, env, method, path, body) *http.Request` that lazily calls `LoginAsTestUser` on first use and attaches `Authorization: Bearer <token>`, caching the token on `*TestEnv`.
  - **This task does not modify existing `_test.go` files and does not remove the `TestToken` constant.** Task 4.4 does both atomically.
  - **Observable**: a throwaway test using `setup.AuthorizedRequest(t, env, "GET", "/api/sources", nil)` against the 4.2-wired app returns 200.
  - _Requirements: foundation for 4.4_
  - _Boundary: tests/integration/setup_
  - _Depends: 4.1, 4.2_

- [x] 4.4 Atomic swap: replace static-token guard with JWT and migrate every existing integration test
  - **Explicit integration task crossing boundaries**. Land all changes in one commit so the build and test suite never break.
  - In `internal/bootstrap/app.go`, replace `engine.Group("/api", middleware.APIToken(env.APIToken))` with `engine.Group("/api", middleware.JWTAuth(validator))`.
  - Replace every `req.Header.Set(middleware.APITokenHeader, setup.TestToken)` across all integration tests with `setup.AuthorizedRequest(...)`-based construction. Files: `arxiv_test.go`, `analyzer_test.go`, `extraction_test.go`, `paper_test.go`, `pdf_test.go`, `source_test.go`.
  - Remove the `TestToken` constant from `tests/integration/setup/setup.go`.
  - **Observable**: `task test:integration` passes; `rg APITokenHeader tests/integration/` and `rg TestToken tests/integration/` return no matches.
  - _Requirements: 6.1, 6.2_
  - _Boundary: bootstrap, http/middleware, tests/integration — explicit integration task_
  - _Depends: 4.3_

- [x] 4.5 Delete static-token middleware and `APIToken` env field
  - Delete `internal/http/middleware/api_token.go` and the `APITokenHeader` constant.
  - Remove the `APIToken` field from `internal/bootstrap/env.go` and its required-at-startup check.
  - **Observable**: `rg APITokenHeader internal/` and `rg 'APIToken[^A-Z]' internal/ cmd/` return no matches; `go build ./...` succeeds.
  - _Requirements: 6.2, 6.3_
  - _Boundary: http/middleware, bootstrap/env_
  - _Depends: 4.4_

- [x] 4.6 Replace `@Security APIToken` swag annotations and regenerate docs
  - In every controller under `internal/http/controller/`, replace `@Security APIToken` with `@Security BearerAuth`.
  - Update the `securityDefinitions` block in the swag entrypoint to define `BearerAuth` as `apiKey, in: header, name: Authorization` (or `http, scheme: bearer` depending on the swag version in use).
  - Run `task swag`. Commit regenerated `docs/`.
  - **Observable**: `rg APIToken docs/` returns no matches; the regenerated OpenAPI advertises only `BearerAuth`.
  - _Requirements: 6.2_
  - _Boundary: http/controller, docs_
  - _Depends: 4.5_

- [ ] 4.7 Update product steering to describe the new auth model
  - Replace the "Auth" line in [.kiro/steering/product.md](../../steering/product.md) with a one-line description of the new JWT model — e.g., "Email + password login (`POST /auth/login`); 24h JWT in `Authorization: Bearer`; seeded via `cmd/seed users <email> <password>`."
  - **Observable**: `rg 'X-API-Token' .kiro/steering/` returns no matches.
  - _Requirements: 6.4_
  - _Boundary: steering/product.md_
  - _Depends: 4.4_

## 5. End-to-end integration tests for new auth flows

- [ ] 5.1 (P) Integration tests for `/auth/login`
  - Create `tests/integration/auth_login_test.go`. Use `SetupTestEnv(t)` plus `setup.SeedTestUser`. Subtests:
    - `login with correct credentials returns 200 with access token and user payload` (asserts user.id, user.email; no password hash field anywhere in the body)
    - `login with unknown email and login with wrong password return identical 401 envelopes with reason=invalid_credentials`
    - `login with a syntactically invalid email returns 400 with reason=validation_failed`
    - `login with a password longer than 72 bytes returns 400 with reason=password_too_long`
    - `login with an empty body returns 400 with reason=validation_failed`
  - **Observable**: `task test:integration` passes; each subtest asserts both HTTP status and `error.details.reason`.
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 7.5_
  - _Boundary: tests/integration/auth_login_test.go_
  - _Depends: 4.4_

- [ ] 5.2 (P) Integration tests for protected `/api/*` JWT guard
  - Create `tests/integration/auth_api_guard_test.go`. Use `GET /api/sources` as the representative protected endpoint. Subtests:
    - `request without Authorization header returns 401 with reason=credentials_missing`
    - `request with a non-Bearer scheme returns 401 with reason=credentials_malformed`
    - `request with a token whose signature does not verify returns 401 with reason=invalid_access_token`
    - `request with an expired access token returns 401 with reason=expired_access_token` (short `AUTH_JWT_TTL` or injected clock)
    - `request with a valid access token returns a non-401 status from the downstream handler`
  - **Observable**: `task test:integration` passes; the valid-token subtest hits a real downstream handler.
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_
  - _Boundary: tests/integration/auth_api_guard_test.go_
  - _Depends: 4.4_

- [ ] 5.3 (P) Integration tests for `/auth/session` and `/auth/change-password`
  - Create `tests/integration/auth_session_change_test.go`. Reuse `setup.LoginAsTestUser`.
  - Session subtests:
    - `GET /auth/session with a valid bearer returns id, email, and created_at`
    - `the response body does not contain a password_hash field at any nesting level` (parse JSON, walk all keys)
    - `GET /auth/session without a token returns 401 with the same reason codes as the /api/* guard`
  - Change-password subtests:
    - `change-password with correct current and valid new returns 204 and a subsequent login with the new password succeeds`
    - `change-password with correct current and a new password equal to the current returns 400 with reason=password-unchanged`
    - `change-password with correct current and a weak new password returns 400 with reason=password-policy-violation`
    - `change-password with an incorrect current password returns 400 with reason=current-password-incorrect`
    - `an access token issued before a password change still passes the /api/* guard until its TTL elapses` (covers Req 4.7)
  - **Observable**: `task test:integration` passes; the no-password-hash assertion walks every key.
  - _Requirements: 3.1, 3.2, 3.3, 4.1, 4.3, 4.4, 4.6, 4.7, 8.4_
  - _Boundary: tests/integration/auth_session_change_test.go_
  - _Depends: 4.4_

## Implementation Notes

- **Reason codes are constants in `internal/domain/user/errors.go`** — `ReasonValidationFailed`, `ReasonPasswordTooLong`, `ReasonPasswordPolicyViolation`, `ReasonInvalidCredentials`, `ReasonCurrentPasswordIncorrect`, `ReasonPasswordUnchanged`. Producers (DTO validators, use-case, controller) and tests must reference these — never raw strings. design.md's hyphen/underscore mix is preserved (e.g. `password-policy-violation`, `password_too_long`); the constants are the single source of truth that locks it in.
- **`ChangePasswordRequest.Validate()` checks 72-byte max BEFORE 12-byte min** — a 73-byte new password yields `password_too_long`, not `password-policy-violation`. This honors Req 7.5's universal-cap intent.
- **`LoginRequest.Validate()` does NOT enforce a 12-byte minimum** on the submitted password — only non-empty + valid email + ≤72 bytes. Short legacy passwords reach the use-case and fail with `invalid_credentials`, not `validation_failed`, per Req 1.3.
- **Single-token model**: this spec is **not** the gridbot-style access+refresh design. There is one JWT (24h TTL, single signing key). No refresh endpoint, no cookie, no logout endpoint, no `Origin`/`Referer` check. If those return as needs, they go in a new spec.
- **request_id bridge (3.1 → 3.2)**: task 3.1 added `application.WithRequestID(ctx, id)` (and an internal `requestIDFromContext`) because `middleware.RequestID` writes only to the Gin context, not `context.Context`. The auth controller in task 3.2 must call `ctx = application.WithRequestID(c.Request.Context(), c.GetString(middleware.RequestIDKey))` before invoking any use-case method; otherwise the `request_id` field in `auth.login.*` and `auth.change_password.*` log lines will be empty. The use-case still works correctly with an empty request_id — it just degrades log traceability.
- **Use-case event names (3.1)**: `auth.login.ok` (info), `auth.login.failed` (warn), `auth.change_password.ok` (info), `auth.change_password.failed` (warn). Match these strings in any integration test that grep's log lines.
- **Unused `clock` parameter on `NewUserUseCase`**: the constructor takes `shared.Clock` per design.md but the current 3-method implementation never reads it (the signer carries its own clock for expiry). Kept for design fidelity and as a forward seat for future deterministic-timestamp needs (e.g., lockout). Not a defect — do not "simplify" by removing it without a spec change.
- **Task 4.4's file list was incomplete**: `APITokenHeader`/`TestToken` also appeared in `internal/bootstrap/app_test.go` (route-wiring tests calling `app.Engine.ServeHTTP` directly — fixed via a `mintTestToken` helper that signs with `env.JWTSecret`/`env.JWTTTL`, no login round trip needed) and in `tests/manual/arxiv_live_test.go` + `tests/manual/extraction_mineru_test.go` (gated by `-tags=manual`, invisible to a plain `go build`/`go vet`). Also, `tests/integration/setup/setup.go`'s own harness `/api` group was still `middleware.APIToken(TestToken)` — swapping only `bootstrap/app.go` is not enough; the harness has its own separate wiring. Verify with `go vet -tags=integration ./...`, `-tags=manual`, and `-tags=mineru` (not just a bare build) before declaring an auth-scheme swap done.
- **Concurrency**: `setup.AuthorizedRequest`'s lazy first-login token cache on `*TestEnv` needed a `sync.Mutex` (`TestEnv.authTokenMu`) — `arxiv_pdf_download_concurrency_test.go`'s `fireFetch` is called from multiple goroutines sharing one `env`, which raced on the unguarded cache. Caught via `go test -race`; the default `task test:integration` does not use `-race`, so this class of bug is otherwise silent.
- **Task 4.5 also had a hidden compile break**: `internal/bootstrap/app_test.go` had two `Env{APIToken: "test-token", ...}` struct literals unrelated to HTTP auth (`TestNewApp_ExtractionStartupRecovery`, `TestNewApp_WiresPDFStore` — they never call `/api/*`) that fail to compile the instant the `APIToken` field is removed from `Env`. Any future field deletion on `Env` should `rg '<FieldName>:' internal/` (struct-literal call sites), not just the field declaration and its env-var plumbing.
- **`.env.example` was already stale before this task** (missing `AUTH_JWT_SECRET` — required with no default, so a fresh `cp .env.example .env && task run` was already broken) — fixed as part of removing the dead `API_TOKEN` line. Worth a periodic `diff <(grep -oP '(?<=mapstructure:")[A-Z_]+' internal/bootstrap/env.go) <(grep -oP '^[A-Z_]+(?==)' .env.example)`-style check after specs that add env vars.
