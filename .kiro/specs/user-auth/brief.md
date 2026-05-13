# Brief: user-auth

## Problem

The thesis backend is currently protected by a single static `X-API-Token` header (see [internal/http/middleware/api_token.go](../../../internal/http/middleware/api_token.go) and the "Auth" line in [product.md](../../steering/product.md)). That mechanism is sufficient for a strictly local single-user prototype but is unsafe for any use beyond that: the token cannot be rotated without redeploy, cannot be revoked, cannot identify *who* is calling, and offers no story for adding a second human operator. The product description allows for the possibility of a second (equal-role) user, and "prioritize safety" was an explicit constraint when planning this change.

## Current State

- Single shared static token compared via `subtle.ConstantTimeCompare` in [api_token.go](../../../internal/http/middleware/api_token.go).
- No `User` entity anywhere in the codebase. No password handling, no JWT support, no cookie handling.
- No user-facing identity surfaces at all (no login, no session, no current-user endpoint).
- Backend stack: Go 1.25, Gin, GORM/SQLite, Viper, slog. Clean/hexagonal layering per [structure.md](../../steering/structure.md) — domain ports in `domain/<entity>/ports.go`, implementations in `infrastructure/`, route wiring in `bootstrap/`.
- `cmd/seed` already exists as a seeding entry point for other domain data; it is the natural home for creating initial users.

## Desired Outcome

- Operator authenticates with email + password, receives a short-lived access JWT (in `Authorization: Bearer`) and a longer-lived refresh JWT (in an `HttpOnly`+`Secure`+`SameSite=Strict` cookie scoped to the refresh endpoint).
- Protected API routes accept the access token via a new JWT middleware. The static `X-API-Token` middleware is removed from the codebase, not toggled — there is exactly one auth scheme.
- Access tokens expire quickly; refresh tokens can be exchanged for a new access token without re-entering a password.
- Password can be rotated in-app (old + new), not by editing the database.
- A second user can be added by re-running the seed command. All seeded users have identical privileges (no roles, no RBAC).

## Approach

Stateless JWT authentication with separate access and refresh tokens, modelled on the gridbot reference [api/auth](../../../../../gridbot/backend_rest/api/auth) but adapted to the thesis backend's clean-hex layering and stripped of multi-role concerns.

**Key choices:**
- **Stateless refresh** (no DB-backed token table, no rotation, no reuse detection). Tradeoff explicitly accepted: a leaked refresh token is valid until expiry. Mitigation is a short refresh TTL.
- **Single auth scheme**: the static-token middleware is deleted, not coexistent.
- **Seeded users only**: no public `/auth/register` endpoint; user creation lives in `cmd/seed` where the operator already has shell access.
- **No roles**: a future second user is identical to the first. If RBAC ever becomes necessary, it is a separate spec.
- **Library hygiene** (defaults baked in, not novel choices): current `golang-jwt/jwt` major version, `golang.org/x/crypto/bcrypt` at a 2026-current cost factor, `token.Valid` checked alongside parse error per the documented CVE footgun, refresh cookie scoped with `Path` set and no `Domain` attribute, `Origin`/`Referer` check on refresh endpoint as defense-in-depth.

**Why this shape:**
- Replaces the existing single-secret model with the minimum upgrade that gives identity, in-app password rotation, and a path to additional users — without dragging in registration flows, email infrastructure, RBAC, or session storage that the single-user product does not need.
- Mirrors gridbot's working pattern (login + refresh + JWT middleware + bcrypt) so the operator already has a mental model, but drops the parts that exist in gridbot only because gridbot is multi-tenant (roles, role-aware authz middlewares, encrypted credential vault).

## Scope

- **In**:
  - `User` aggregate: `domain/user/{model.go, ports.go, errors.go, requests.go, responses.go}`. Fields at minimum: `ID`, `Email` (unique), `PasswordHash`, `CreatedAt`, `UpdatedAt`.
  - Application use-cases: `Login`, `Refresh`, `Logout`, `Session` (current user), `ChangePassword`.
  - Infrastructure: GORM persistence for users (append to `AutoMigrate` in [internal/infrastructure/persistence/migrate.go](../../../internal/infrastructure/persistence/migrate.go)), bcrypt password hasher behind a port, JWT signer/verifier behind a port, clock already exists in `domain/shared`.
  - HTTP layer: `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `GET /auth/session`, `POST /auth/change-password`. New `JWTAuth` middleware in `internal/http/middleware/` that replaces the current `APIToken` middleware on the `/api` group.
  - Deletion of [api_token.go](../../../internal/http/middleware/api_token.go) and its wiring in [bootstrap/app.go](../../../internal/bootstrap/app.go).
  - Config additions to [bootstrap/env.go](../../../internal/bootstrap/env.go): access-token secret, refresh-token secret, access TTL, refresh TTL, refresh cookie name, app environment flag for cookie `Secure` toggling in local dev.
  - Seed support: extend `cmd/seed` so it can create users (email + plaintext password → bcrypt hash → DB row). Idempotent on re-run.
  - Update [product.md](../../steering/product.md) "Auth" line to reflect the new model.
  - Integration tests under `tests/integration/` covering login success, login failure, refresh, session, change-password, and rejection of unauthenticated requests to `/api/*`. Real SQLite via the existing test harness, not mocks for the repository.

- **Out**:
  - Registration endpoint (`POST /auth/register`). Users are seeded.
  - Roles, permissions, RBAC, any authz beyond "authenticated == has access".
  - Password reset by email, magic links, email verification, 2FA, OAuth, SSO, social login.
  - Refresh-token rotation, reuse detection, server-side revocation, `refresh_token_jti` table.
  - Session listing, "log out everywhere", device tracking.
  - Rate limiting / brute-force lockout (separate cross-cutting concern; out of this spec's boundary).
  - CSRF token plumbing — `SameSite=Strict` + `Origin` check is the chosen mitigation surface.
  - Postgres migration. Thesis backend is on SQLite per [tech.md](../../steering/tech.md); persistence stays on SQLite via GORM.
  - Frontend work of any kind.

## Boundary Candidates

- **Identity domain** (`domain/user/`) — what a user *is* and what can be asked of it. Ports for `Repository`, `PasswordHasher`, and `TokenSigner` live here.
- **Authentication use-cases** (`application/user/`) — login, refresh, logout, session, change-password orchestrators. Pure functions over the ports.
- **JWT/cookie infrastructure** (`infrastructure/auth/`) — the signer/verifier implementation, refresh cookie packing/unpacking. Single home for token format decisions.
- **HTTP surface** (`interface/http/`) — handlers, request/response DTOs, JWT middleware. Owns transport concerns only; no business logic.
- **Seed extension** (`cmd/seed/`) — operator-facing user creation. Isolated from production code paths.

## Out of Boundary

- Anything that implies multi-tenant identity: tenants, organizations, workspaces.
- Authorization beyond authentication: roles, scopes, permissions, attribute-based access.
- Email or notification infrastructure (would be needed for password reset, verification, magic links).
- Auditing / login-attempt logging beyond standard request logs.
- API-key issuance for machine clients (the static `X-API-Token` going away is intentional; if machine clients return as a need, they get their own spec).

## Upstream / Downstream

- **Upstream**:
  - [internal/bootstrap/](../../../internal/bootstrap/) — DI wiring, env loading.
  - [internal/http/route/](../../../internal/http/route/) — per-feature router pattern this spec must follow.
  - [internal/infrastructure/persistence/migrate.go](../../../internal/infrastructure/persistence/migrate.go) — GORM AutoMigrate entry list.
  - [domain/shared/](../../../internal/domain/shared/) — `Logger`, `Clock`, `HTTPError` already used by every feature.
  - [tests/integration/setup/setup.go](../../../tests/integration/setup/setup.go) — test harness this spec's integration tests reuse.
- **Downstream**:
  - Every existing controller currently mounted under `/api` (arxiv, paper, extraction, analyzer) — they keep working unchanged once JWT middleware replaces the static-token middleware on that group.
  - Any future frontend will call `POST /auth/login` then carry the bearer token; no further coupling.

## Existing Spec Touchpoints

- **Extends**: none. This is a new boundary.
- **Adjacent**:
  - [arxiv-fetcher](../arxiv-fetcher), [paper-persistence](../paper-persistence), [document-extraction](../document-extraction), [llm-analyzer](../llm-analyzer), [pdf-storage](../pdf-storage), [arxiv-pdf-download](../arxiv-pdf-download) — all consume the `/api` group; none owns auth. This spec is the sole owner of route protection.
  - [product.md](../../steering/product.md) "Auth" section is currently "single static API token" — must be updated as part of this spec, not left stale.

## Constraints

- Stack is fixed per [tech.md](../../steering/tech.md): Go 1.25, Gin, GORM/SQLite, Viper, slog. No new web frameworks, no ORM swap, no Postgres in this spec.
- Layering rules in [structure.md](../../steering/structure.md) are non-negotiable: domain code may not import infrastructure; ports live in `domain/<entity>/ports.go`; constructors return interfaces, structs unexported.
- Test standards in [testing.md](../../steering/testing.md): real DB over fakes, AAA via blank lines, sentence-style subtests, doubles in `tests/mocks/`. Integration tests must use the existing harness.
- Library version specifics and exact cost factors are left to the requirements/design phase under "follow current best practices" — no novel cryptographic choices.
- Single-user-today is the operational reality; multi-user-tomorrow is a possible expansion. The data model must allow more than one user row from day one (unique constraint on email, no implicit "the user").
- Cookie `Secure` must be controllable via env so local-dev over `http://localhost` does not silently break login.
