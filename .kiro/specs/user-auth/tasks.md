# Implementation Plan

> Note on test doubles: per [testing.md](../../steering/testing.md), this plan uses **real implementations** (in-memory SQLite, real bcrypt at cost 4 in tests, real JWT signer with test keys) instead of the hand-written fakes in `tests/mocks/` that design.md hypothetically suggested. No mock files are introduced for `PasswordHasher`, `TokenSigner`, or `TokenValidator`.
>
> Note on package naming: the existing codebase uses `<entity>persist` as the import alias for `infrastructure/persistence/<entity>` packages (e.g., `sourcepersist` in [internal/bootstrap/seed.go](../../../internal/bootstrap/seed.go)). The same convention applies here — bare `user` for `internal/domain/user`, `userpersist` for `internal/infrastructure/persistence/user` — so consumers can import both without name collision.

## 1. Foundation: dependencies, ports, env, schema

- [x] 1.1 Add JWT v5 and bcrypt module dependencies
  - Add `github.com/golang-jwt/jwt/v5` pinned at `v5.2.2` or later (the CVE-2025-30204 fix).
  - Add `golang.org/x/crypto/bcrypt`.
  - Run `go mod tidy`; commit `go.mod` and `go.sum`.
  - **Observable**: `go build ./...` succeeds and both modules appear in the `require` block of `go.mod`.
  - _Requirements: 9.1, 9.2_
  - _Boundary: go.mod_

- [x] 1.2 (P) Add cross-cutting auth ports to the shared domain
  - Append `PasswordHasher`, `TokenSigner`, `TokenValidator` interfaces with the signatures from design.md to [internal/domain/shared/ports.go](../../../internal/domain/shared/ports.go).
  - Append crypto error sentinels `ErrHashMismatch`, `ErrTokenMalformed`, `ErrTokenSignatureInvalid`, `ErrTokenExpired` to the same file.
  - **Observable**: the three interfaces and four sentinels compile under `go build ./internal/domain/shared/...` with no concrete implementations yet.
  - _Requirements: 9.1, 9.3, 2.4, 2.5, 3.3, 3.4_
  - _Boundary: domain/shared_

- [x] 1.3 (P) Add auth env-configuration fields and startup validation
  - Add fields to the env struct in [internal/bootstrap/env.go](../../../internal/bootstrap/env.go): `AccessSecret` (env `AUTH_ACCESS_SECRET`, required), `RefreshSecret` (env `AUTH_REFRESH_SECRET`, required), `AccessTTL` (env `AUTH_ACCESS_TTL`, default `15m`), `RefreshTTL` (env `AUTH_REFRESH_TTL`, default `24h`), `CookieInsecure` (env `AUTH_COOKIE_INSECURE`, default `false`), `RefreshOrigin` (env `AUTH_REFRESH_ORIGIN`, required).
  - Use `v.SetDefault` for the two TTLs and `CookieInsecure`. Bootstrap fails fast at startup when any required secret is empty, when the two secrets are byte-equal, or when either secret is shorter than 32 bytes.
  - `APIToken` field and its existing required-at-startup check remain in place until task 4.5 deletes them — keep the app bootable in the interim.
  - **Observable**: starting the app with any required `AUTH_*` var unset yields a clear startup error message naming the missing variable; setting all of them to valid values starts the app cleanly.
  - _Requirements: 1.2, 1.3, 1.7, 1.8, 3.5, 3.8, 3.9_
  - _Boundary: bootstrap/env_

- [x] 1.4 (P) Create the User domain aggregate
  - Create [internal/domain/user/model.go](../../../internal/domain/user/model.go): `User` entity with `ID uuid.UUID`, `Email string`, `PasswordHash string`, `CreatedAt`, `UpdatedAt time.Time`.
  - Create [internal/domain/user/ports.go](../../../internal/domain/user/ports.go): `user.UseCase` interface (`Login`, `Refresh`, `Session`, `ChangePassword` — no `Logout` method; logout is HTTP-only per design line 426) and `user.Repository` interface (`FindByEmail`, `FindByID`, `Save`, `UpdatePasswordHash`).
  - Create [internal/domain/user/requests.go](../../../internal/domain/user/requests.go): `LoginRequest`, `ChangePasswordRequest` with `Validate()` enforcing email syntax via `net/mail.ParseAddress`, 12-char min and 72-byte max on passwords, returning `*shared.HTTPError` with `WithReason("validation_failed" | "password_too_long")`.
  - Create [internal/domain/user/responses.go](../../../internal/domain/user/responses.go): `SessionResponse{ID, Email, CreatedAt}`, `LoginResponse{AccessToken, ExpiresAt, User SessionResponse}`, `RefreshResponse{AccessToken, ExpiresAt}`, plus the use-case result types `LoginResult{AccessToken, AccessExpiresAt, RefreshToken, RefreshExpiresAt, User *User}` and `RefreshResult{AccessToken, ExpiresAt}` referenced by the `user.UseCase` interface. No type in this file may carry a password hash.
  - Create [internal/domain/user/errors.go](../../../internal/domain/user/errors.go): `ErrNotFound`, `ErrEmailExists`, `ErrInvalidCredentials`, `ErrCurrentPasswordIncorrect`, `ErrPasswordPolicyViolation`, `ErrPasswordUnchanged`.
  - **Observable**: `go vet ./internal/domain/user/...` is clean; package builds; types compile.
  - _Requirements: 1.6, 5.1, 5.3, 6.3, 6.5, 6.6, 7.5, 9.5_
  - _Boundary: domain/user_

- [x] 1.5 Add User persistence model and register with AutoMigrate
  - Create [internal/infrastructure/persistence/user/model.go](../../../internal/infrastructure/persistence/user/model.go): GORM `Model` struct with `ID string` (text PK), `Email` (not null, unique index), `PasswordHash` (not null), `CreatedAt`, `UpdatedAt`; `TableName() = "users"`; `ToDomain()` and `FromDomain()` converting `uuid.UUID` ↔ `string`. Package declaration is `package user` (consumers will import as `userpersist`).
  - Append `&userpersist.Model{}` to the `AutoMigrate` call in [internal/infrastructure/persistence/migrate.go](../../../internal/infrastructure/persistence/migrate.go), importing the persistence package under the alias `userpersist` to avoid collision with the domain `user` package.
  - **Observable**: starting the app (or running `task seed`) creates a `users` table in the SQLite DB with the expected columns and a unique index on `email`. Inspect with `sqlite3 <db> '.schema users'`.
  - _Requirements: 7.5, 5.1_
  - _Boundary: infrastructure/persistence/user_

## 2. Core adapters

- [ ] 2.1 (P) Bcrypt password hasher with colocated unit test
  - Create [internal/infrastructure/auth/bcrypt_hasher.go](../../../internal/infrastructure/auth/bcrypt_hasher.go): `NewBcryptHasher(cost int) shared.PasswordHasher`. `Hash` uses `bcrypt.GenerateFromPassword(pw, cost)`. `Verify` uses `bcrypt.CompareHashAndPassword`, returning `shared.ErrHashMismatch` on `bcrypt.ErrMismatchedHashAndPassword` and the underlying error otherwise.
  - Bootstrap will construct with cost 12; tests will construct with cost 4 for speed.
  - Add colocated `bcrypt_hasher_test.go`: subtests `round-trip hash and verify returns nil`, `verify returns ErrHashMismatch on wrong password`, `hash of empty string returns error`.
  - **Observable**: `go test ./internal/infrastructure/auth/...` passes; the test file uses cost 4.
  - _Requirements: 9.1, 9.2, 9.3_
  - _Boundary: infrastructure/auth_

- [ ] 2.2 (P) JWT token service with colocated unit test
  - Create [internal/infrastructure/auth/jwt_token_service.go](../../../internal/infrastructure/auth/jwt_token_service.go): `JWTConfig{AccessSecret, RefreshSecret []byte; AccessTTL, RefreshTTL time.Duration; Clock shared.Clock}`. `NewJWTTokenService(cfg JWTConfig)` implements both `shared.TokenSigner` and `shared.TokenValidator`. Claims: `sub` (string), `iat`, `exp`. HS256.
  - Always check `token.Valid` alongside the parse error in both `VerifyAccess` and `VerifyRefresh` (CVE-2024-51744 guard).
  - Map `jwt.ErrTokenExpired` → `shared.ErrTokenExpired`; signature errors → `shared.ErrTokenSignatureInvalid`; parse/malformed errors → `shared.ErrTokenMalformed`.
  - Add colocated `jwt_token_service_test.go` driving a deterministic clock: `round-trip access token verifies`, `round-trip refresh token verifies`, `access token presented to VerifyRefresh fails with ErrTokenSignatureInvalid` (key-separation guard for Req 3.9), `verifier rejects token whose exp is past the clock`, `verifier rejects a token that parses but has Valid=false`.
  - **Observable**: `go test ./internal/infrastructure/auth/...` passes; the key-separation subtest fails loudly if anyone collapses the two keys to one.
  - _Requirements: 1.7, 1.8, 2.4, 2.5, 2.7, 3.3, 3.4, 3.7, 3.8, 3.9_
  - _Boundary: infrastructure/auth_

- [ ] 2.3 (P) User repository (GORM) with colocated unit test
  - Create [internal/infrastructure/persistence/user/repo.go](../../../internal/infrastructure/persistence/user/repo.go): `NewRepository(db *gorm.DB) user.Repository`. Maps `gorm.ErrRecordNotFound` → `user.ErrNotFound`; SQLite unique-constraint violation → `user.ErrEmailExists`. `UpdatePasswordHash` updates only the `password_hash` and `updated_at` columns explicitly (no full-record replace).
  - Add colocated `repo_test.go` using the in-memory SQLite helper pattern from [tests/integration/setup/setup.go](../../../tests/integration/setup/setup.go) (per [testing.md](../../steering/testing.md)): subtests `save persists a new user`, `save returns ErrEmailExists on duplicate email`, `find by email returns ErrNotFound when missing`, `find by id returns ErrNotFound when missing`, `update password hash replaces hash and bumps updated_at`.
  - **Observable**: `go test ./internal/infrastructure/persistence/user/...` passes against a temp SQLite DB.
  - _Requirements: 7.5, 5.1, 6.1, 1.1_
  - _Boundary: infrastructure/persistence/user_

- [ ] 2.4 (P) JWTAuth middleware with colocated unit test
  - Create [internal/http/middleware/jwt_auth.go](../../../internal/http/middleware/jwt_auth.go): `JWTAuth(validator shared.TokenValidator) gin.HandlerFunc`. Reads `Authorization: Bearer <token>` header. Maps errors to `*shared.HTTPError` via `c.Error` with reason codes `credentials_missing`, `credentials_malformed`, `invalid_access_token`, `expired_access_token`, `malformed_access_token` and then `c.AbortWithStatus(401)`. On success, sets `user_id` on the Gin context as `uuid.UUID` and calls `c.Next()`. Never consults a DB or external service.
  - Add colocated `jwt_auth_test.go` using a real `JWTTokenService` from task 2.2 to mint tokens with controlled expiry against a fake `gin.Context`: subtests `accepts a valid bearer and sets user_id on context`, `rejects missing Authorization header with credentials_missing`, `rejects non-Bearer scheme with credentials_malformed`, `rejects tampered signature with invalid_access_token`, `rejects expired token with expired_access_token`.
  - **Observable**: `go test ./internal/http/middleware/...` passes; assertions check both status and the `details.reason` envelope field.
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_
  - _Boundary: http/middleware_
  - _Depends: 2.2_

## 3. Application + HTTP surface

- [ ] 3.1 User use-case implementation with colocated unit test
  - Create [internal/application/user_usecase.go](../../../internal/application/user_usecase.go): `NewUserUseCase(repo user.Repository, hasher shared.PasswordHasher, signer shared.TokenSigner, clock shared.Clock, log shared.Logger) user.UseCase`. Implements `Login`, `Refresh`, `Session`, `ChangePassword`. (No `Logout` — that's HTTP-only.)
  - Login flow: lookup by email, verify password; on either failure return `user.ErrInvalidCredentials` (same error for unknown email and wrong password, Req 1.4, 1.5). On match, issue access + refresh tokens and return `user.LoginResult`. Emit info log `"event":"auth.login.ok"` with `user_id` and `request_id`; on any failure emit warn log `"event":"auth.login.fail"` with `email` and `reason`. Never log password material or token strings.
  - Refresh flow: `signer` is not called; the validator is used at the HTTP layer. Use-case `Refresh` accepts `subject string` (already verified) and issues a new access token only — no new refresh.
  - ChangePassword flow: load user by id, verify current password (wrong → `ErrCurrentPasswordIncorrect`), enforce new-password policy (`Validate()` on the DTO already does this; map a returned policy error to `ErrPasswordPolicyViolation`), reject byte-equal new password (`ErrPasswordUnchanged`), hash new password, call `UpdatePasswordHash`. Document in a comment: pre-change access and refresh tokens remain valid until expiry (Req 6.7).
  - Treat any `Verify` error other than `shared.ErrHashMismatch` as `user.ErrInvalidCredentials` and emit a warn log with `reason="hash_unreadable"` — covers a malformed stored hash without surfacing a 500 (Req 9.4).
  - Add colocated `user_usecase_test.go` using **real** in-memory SQLite repo + real bcrypt at cost 4 + real `JWTTokenService` with test keys + a deterministic `Clock` mock-by-injection (per [testing.md](../../steering/testing.md) "real over fake"): subtests `login with unknown email returns invalid credentials`, `login with wrong password returns invalid credentials with the same error as unknown email`, `change password rotates the hash and subsequent login with the new password succeeds`, `change password with wrong current returns ErrCurrentPasswordIncorrect`, `change password with new equals current returns ErrPasswordUnchanged`, `change password with weak new returns ErrPasswordPolicyViolation`, `access tokens issued before a password change still validate until their expiry` (covers Req 6.7), `login emits info log on success and warn log on each failure mode`.
  - **Observable**: `go test ./internal/application/...` passes; the `pre-change tokens still valid` subtest fails loudly if anyone later adds revocation behind the scenes.
  - _Requirements: 1.4, 1.5, 1.9, 1.10, 3.1, 3.6, 5.1, 6.1, 6.3, 6.4, 6.5, 6.6, 6.7, 9.4_
  - _Boundary: application/user_
  - _Depends: 2.1, 2.2, 2.3_

- [ ] 3.2 Auth controller, response wrappers, and swag annotations
  - Create [internal/http/controller/auth_controller.go](../../../internal/http/controller/auth_controller.go) and [internal/http/controller/auth_responses.go](../../../internal/http/controller/auth_responses.go). Constructor: `NewAuthController(uc user.UseCase, validator shared.TokenValidator, cookieInsecure bool, refreshOrigin string) *AuthController` — config flags are constructor parameters; the controller does not read from `route.Deps` directly.
  - Login: bind body, call `req.Validate()`, call `uc.Login`. On success: serialize `LoginResponse` (no refresh token in body); `c.SetCookie("refresh_token", refreshToken, int(refreshTTL.Seconds()), "/auth/refresh", "", !cookieInsecure, true)` (cookie params: name, value, max-age, path, domain="" for host-only, secure, httpOnly). Apply `SameSite=Strict` via `c.SetSameSite(http.SameSiteStrictMode)` before the call.
  - Refresh: read `Origin` and `Referer` headers; if both are absent or the present one does not match `refreshOrigin`, abort with 403 `origin_mismatch` and stop before reading the cookie. Otherwise read the cookie, call `validator.VerifyRefresh(token)`, then `uc.Refresh(ctx, subject)`. Return `RefreshResponse` (no Set-Cookie — refresh tokens are not rotated, Req 3.6).
  - Logout: always 204 and `c.SetCookie("refresh_token", "", -1, "/auth/refresh", "", !cookieInsecure, true)` with the same `SameSite=Strict`. Accept calls without an `Authorization` header (Req 4.2) and without an existing cookie (Req 4.3). Never write any server-side record of logout (Req 4.4 — satisfied by absence).
  - Session: read `user_id` from the Gin context (set by `JWTAuth`), call `uc.Session`, return `SessionResponse` (id, email, created_at — no password hash, no token, Req 5.3 + 10.4).
  - ChangePassword: read `user_id` from context, bind body, call `req.Validate()`, call `uc.ChangePassword`. 204 on success.
  - Error path everywhere: `c.Error(shared.NewHTTPError(status, message, cause).WithReason(reason))` with the reason codes in the design's API-contract table.
  - Never include any access token, refresh token, or password field in any log statement at the controller layer (Req 10.1, 10.2). No request-body logger is registered in this spec; if a future spec adds one, redaction of `password`, `current_password`, `new_password`, `access_token`, `refresh_token` is required (Req 10.3 — deferred satisfaction).
  - Add swag annotations on every new handler: `@Tags Auth`, `@Accept`/`@Produce`, `@Param`, `@Success`, one `@Failure` per distinct status, `@Security BearerAuth` only on `/auth/session` and `/auth/change-password`, `@Router`.
  - **Observable**: handlers compile and each branch of the API-contract table is exercised by integration tests in Phase 5.
  - _Requirements: 1.1, 1.2, 1.3, 1.7, 1.8, 3.1, 3.2, 3.5, 3.6, 4.1, 4.2, 4.3, 4.4, 5.1, 5.2, 5.3, 6.1, 6.2, 6.3, 6.4, 6.7, 8.5, 10.1, 10.2, 10.3, 10.4_
  - _Boundary: http/controller_

- [ ] 3.3 Auth router and `route.Deps` extension
  - Extend the `route.Deps` struct in [internal/http/route/route.go](../../../internal/http/route/route.go) with `Hasher shared.PasswordHasher`, `Signer shared.TokenSigner`, `Validator shared.TokenValidator`, `AccessTTL time.Duration`, `RefreshTTL time.Duration`, `CookieInsecure bool`, `RefreshOrigin string`, and either `Engine *gin.Engine` or `RootGroup *gin.RouterGroup` so the unauthenticated `/auth/*` routes can be mounted outside `/api`.
  - Create [internal/http/route/auth_route.go](../../../internal/http/route/auth_route.go): `AuthRouter(d Deps)` imports the persistence package as `userpersist`, builds `repo := userpersist.NewRepository(d.DB)`, `uc := application.NewUserUseCase(repo, d.Hasher, d.Signer, d.Clock, d.Logger)`, `ctrl := controller.NewAuthController(uc, d.Validator, d.CookieInsecure, d.RefreshOrigin)`. Mounts unauthenticated `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout` on the root group; mounts authenticated `GET /auth/session` and `POST /auth/change-password` under `Group("/auth", middleware.JWTAuth(d.Validator))`.
  - **Observable**: `route.AuthRouter(d)` registers exactly five new endpoints; `go vet ./internal/http/route/...` is clean.
  - _Requirements: 8.5_
  - _Boundary: http/route_

## 4. Composition root and migration

- [ ] 4.1 (P) SeedUser helper and `cmd/seed users` subcommand
  - Add `SeedUser(ctx context.Context, db *gorm.DB, hasher shared.PasswordHasher, log shared.Logger, email, plain string) error` to [internal/bootstrap/seed.go](../../../internal/bootstrap/seed.go). Validate email syntax (`net/mail.ParseAddress`) and password policy (12-char min / 72-byte max). Build the repo via `userpersist.NewRepository(db)`. Hash via `hasher.Hash(plain)`. Call `repo.Save`; on `user.ErrEmailExists` log `"seed.user.skipped"` at info level with the email and return `nil` (idempotent re-runs). Never log `plain` (Req 10.5).
  - Update [cmd/seed/main.go](../../../cmd/seed/main.go) to dispatch on `os.Args[1]`: no args → existing `SeedSources` path (unchanged); `users <email> <password>` → calls `SeedUser` constructing `NewBcryptHasher(12)`; `-h` / `--help` documents both modes. Bad arity exits non-zero with usage. Weak password / bad email surfaces the validator error and exits non-zero without writing the row.
  - **Observable**: `seed users alice@example.com Password123abc!` creates one row; re-running prints `seed.user.skipped` and exits 0; `seed users alice@example.com weak` exits non-zero and writes no row; the password never appears in stdout/stderr/log files.
  - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 10.5_
  - _Boundary: bootstrap/seed, cmd/seed_

- [ ] 4.2 (P) Wire JWT token service and mount `/auth` router additively
  - In [internal/bootstrap/app.go](../../../internal/bootstrap/app.go), construct a single `JWTTokenService` from the new env fields and pass it into `route.Deps` as both `Signer` and `Validator`. Also pass `NewBcryptHasher(12)` as `Hasher`, the env TTLs/origin/cookie-insecure flag.
  - Call `route.AuthRouter(d)` on the **root** group (not `/api`) so login/refresh/logout are reachable without a token.
  - **This step is additive only.** The existing `middleware.APIToken(env.APIToken)` mount on `/api` stays in place; the existing static-token tests keep passing.
  - **Observable**: `curl -X POST /auth/login` against a seeded user returns 200 + Set-Cookie; existing integration tests that hit `/api/*` with `X-API-Token` still pass.
  - _Requirements: 1.1, 3.1, 4.1, 5.1, 6.1_
  - _Boundary: bootstrap/app_

- [ ] 4.3 Extend integration test harness with login-based authentication helpers
  - Update [tests/integration/setup/setup.go](../../../tests/integration/setup/setup.go): add `SeedTestUser(t *testing.T, env *TestEnv)` that inserts a deterministic test user (`testuser@example.com` / a constant test password) via `userpersist.NewRepository(env.DB).Save`. Add `LoginAsTestUser(t, env) (accessToken string, refreshCookie *http.Cookie)` that POSTs to `/auth/login` and returns the token + cookie, caching them on `*TestEnv`. Add `AuthorizedRequest(t, env *TestEnv, method, path string, body io.Reader) *http.Request` that calls `LoginAsTestUser` lazily on first use and attaches `Authorization: Bearer <token>`.
  - **This task does not modify any existing `_test.go` files and does not remove the `TestToken` constant.** The atomic swap in 4.4 does both.
  - **Observable**: calling `setup.AuthorizedRequest(t, env, "GET", "/api/sources", nil)` from a new throwaway test compiles, attaches a Bearer header, and returns 200 from the existing sources handler when run against the `4.2`-wired app.
  - _Requirements: foundation for 4.4_
  - _Boundary: tests/integration/setup_
  - _Depends: 4.1, 4.2_

- [ ] 4.4 Atomic swap: replace static-token guard with JWT and migrate every existing integration test
  - **Explicit integration task crossing boundaries: bootstrap + middleware + every existing `tests/integration/*_test.go`.** Land all changes in one commit so the build and the test suite never break.
  - In [internal/bootstrap/app.go](../../../internal/bootstrap/app.go), replace `engine.Group("/api", middleware.APIToken(env.APIToken))` with `engine.Group("/api", middleware.JWTAuth(validator))` using the validator from 4.2.
  - Replace every `req.Header.Set(middleware.APITokenHeader, setup.TestToken)` (and any equivalent) across all integration tests with `setup.AuthorizedRequest(...)`-based construction. Files affected (from the codebase survey): `arxiv_test.go`, `analyzer_test.go`, `extraction_test.go`, `paper_test.go`, `pdf_test.go`, `source_test.go`. Verify with `rg -l 'APITokenHeader|TestToken' tests/integration/` returning empty after the edit.
  - Remove the `TestToken` constant from `tests/integration/setup/setup.go`.
  - **Observable**: `task test:integration` passes; `rg APITokenHeader tests/integration/` returns no matches; `rg TestToken tests/integration/` returns no matches.
  - _Requirements: 8.1, 8.2_
  - _Boundary: bootstrap, http/middleware, tests/integration — explicit integration task_
  - _Depends: 4.3_

- [ ] 4.5 Delete static-token middleware and `APIToken` env field
  - Delete [internal/http/middleware/api_token.go](../../../internal/http/middleware/api_token.go) and the `APITokenHeader` constant entirely.
  - Remove the `APIToken` field from the env struct in [internal/bootstrap/env.go](../../../internal/bootstrap/env.go) and its required-at-startup check.
  - **Observable**: `rg APITokenHeader internal/` returns no matches; `rg 'APIToken[^A-Z]' internal/ cmd/` returns no matches; `go build ./...` succeeds.
  - _Requirements: 8.2, 8.3_
  - _Boundary: http/middleware, bootstrap/env_
  - _Depends: 4.4_

- [ ] 4.6 Replace `@Security APIToken` swag annotations and regenerate docs
  - In every controller under [internal/http/controller/](../../../internal/http/controller/), replace `@Security APIToken` with `@Security BearerAuth`.
  - Update the `securityDefinitions` block (or equivalent) in the swag entrypoint to define a `BearerAuth` scheme of type `apiKey, in: header, name: Authorization` (or `http, scheme: bearer` depending on the swag version in use).
  - Run `task swag`. Commit the regenerated `docs/` directory.
  - **Observable**: `rg APIToken docs/` returns no matches; the regenerated OpenAPI document advertises only `BearerAuth` as the security scheme.
  - _Requirements: 8.2_
  - _Boundary: http/controller, docs_
  - _Depends: 4.5_

- [ ] 4.7 Update product steering to describe the new auth model
  - Replace the "Auth" line in [.kiro/steering/product.md](../../steering/product.md) (currently "Single static API token, header `X-API-Token`.") with a one-line description of the new JWT model — e.g., "Email + password login (`POST /auth/login`); access token in `Authorization: Bearer`, refresh token in `HttpOnly` cookie scoped to `/auth/refresh`; seeded via `cmd/seed users <email> <password>`."
  - **Observable**: `rg 'X-API-Token' .kiro/steering/` returns no matches; the new line is the sole content of the Auth section.
  - _Requirements: 8.4_
  - _Boundary: steering/product.md_
  - _Depends: 4.4_

## 5. End-to-end integration tests for new auth flows

- [ ] 5.1 (P) Integration tests for `/auth/login`
  - Create `tests/integration/auth_login_test.go`. Use the `SetupTestEnv(t)` harness; build a `setup.SeedTestUser(t, env)` row before each subtest.
  - Subtests: `login with correct credentials returns 200 with access token and a refresh cookie carrying HttpOnly + SameSite=Strict + Path=/auth/refresh + no Domain attribute` (asserts every cookie attribute by parsing the `Set-Cookie` header); `login with unknown email and login with wrong password return identical 401 envelopes with reason=invalid_credentials`; `login with a syntactically invalid email returns 400 with reason=validation_failed`; `login with a password longer than 72 bytes returns 400 with reason=password_too_long`; `login with an empty body returns 400 with reason=validation_failed`.
  - **Observable**: `task test:integration` passes; each subtest asserts both HTTP status and the `error.details.reason` field of the envelope.
  - _Requirements: 1.1, 1.2, 1.4, 1.5, 1.6, 9.5_
  - _Boundary: tests/integration/auth_login_test.go_
  - _Depends: 4.4_

- [ ] 5.2 (P) Integration tests for `/auth/refresh`
  - Create `tests/integration/auth_refresh_test.go`. Each subtest acquires a valid refresh cookie via `setup.LoginAsTestUser` and manipulates it as needed.
  - Subtests: `refresh with valid cookie and Origin matching AUTH_REFRESH_ORIGIN returns 200 with a new access token`; `refresh without the refresh cookie returns 401 with reason=refresh_token_missing`; `refresh with a tampered refresh token returns 401 with reason=invalid_refresh_token`; `refresh with an expired refresh token returns 401 with reason=expired_refresh_token` (the test sets `AUTH_REFRESH_TTL=1s` and waits, or injects a clock); `refresh with no Origin or Referer header returns 403 with reason=origin_mismatch`; `refresh with a mismatched Origin returns 403 with reason=origin_mismatch`; `the refresh response does not contain a Set-Cookie header for refresh_token` (verifies Req 3.6).
  - **Observable**: `task test:integration` passes; the no-rotation subtest fails loudly if anyone later wires rotation in.
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6_
  - _Boundary: tests/integration/auth_refresh_test.go_
  - _Depends: 4.4_

- [ ] 5.3 (P) Integration tests for protected `/api/*` JWT guard
  - Create `tests/integration/auth_api_guard_test.go`. Use `GET /api/sources` as the representative protected endpoint (it exists today via the source aggregate and returns 200 for a list).
  - Subtests: `request without Authorization header returns 401 with reason=credentials_missing`; `request with a non-Bearer scheme returns 401 with reason=credentials_malformed`; `request with a token whose signature does not verify returns 401 with reason=invalid_access_token`; `request with an expired access token returns 401 with reason=expired_access_token` (short `AUTH_ACCESS_TTL` or injected clock); `request with a valid access token returns a non-401 status from the downstream `/api/sources` handler` (asserts the handler is actually reached, not just authenticated).
  - **Observable**: `task test:integration` passes; the valid-token subtest hits and exercises a real downstream handler.
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_
  - _Boundary: tests/integration/auth_api_guard_test.go_
  - _Depends: 4.4_

- [ ] 5.4 (P) Integration tests for `/auth/session`, `/auth/change-password`, `/auth/logout`
  - Create `tests/integration/auth_session_change_logout_test.go`. Reuse `setup.LoginAsTestUser`.
  - Session subtests: `GET /auth/session with a valid bearer returns id, email, and created_at`; `the response body does not contain a password_hash field at any nesting level` (parse JSON, assert key absence — covers Req 5.3 + 10.4); `GET /auth/session without a token returns 401 with the same reason codes as the /api/* guard`.
  - Change-password subtests: `change-password with correct current and valid new returns 204 and a subsequent login with the new password succeeds`; `change-password with correct current and a new password equal to the current returns 400 with reason=password-unchanged`; `change-password with correct current and a weak new password returns 400 with reason=password-policy-violation`; `change-password with an incorrect current password returns 400 with reason=current-password-incorrect`; `an access token issued before a password change still passes the /api/* guard until its access TTL elapses` (covers Req 6.7).
  - Logout subtests: `POST /auth/logout returns 204 and the Set-Cookie response header clears the refresh_token (Max-Age=0, same Path)`; `POST /auth/logout with no Authorization header still returns 204`; `POST /auth/logout with no refresh cookie still returns 204`.
  - **Observable**: `task test:integration` passes; the no-password-hash assertion walks every key in the JSON response.
  - _Requirements: 4.1, 4.2, 4.3, 4.4, 5.1, 5.2, 5.3, 6.1, 6.3, 6.4, 6.6, 6.7, 10.4_
  - _Boundary: tests/integration/auth_session_change_logout_test.go_
  - _Depends: 4.4_

## Implementation Notes

- **Reason-code casing (post-1.4)**: requirements.md Req 9.5 spells the over-72-byte reason as `password-too-long` (hyphen), while design.md uses `password_too_long` (underscore) consistently across login and change-password. Implementation in `internal/domain/user/requests.go` uses **`password_too_long`** (underscore) — match this in tasks 3.2 (controller error envelopes) and 5.1 (integration tests for "password > 72 bytes returns 400 ..."). Same applies if you ever see `password-policy-violation` vs `password_policy_violation`: implementation uses **`password-policy-violation`** (hyphen) — that one matches design.md's casing.
- **`ChangePasswordRequest.Validate()` checks 72-byte max BEFORE 12-byte min** — a 73-byte new password yields `password_too_long`, not `password-policy-violation`. This honors Req 9.5's universal-cap intent.
- **`LoginRequest.Validate()` does NOT enforce a 12-byte minimum** on the submitted password — only non-empty + valid email + ≤72 bytes. Short legacy passwords reach the use-case and fail with `invalid_credentials`, not `validation_failed`, per Req 1.5 / 1.6.
