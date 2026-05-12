# Research & Design Decisions — user-auth

## Summary

- **Feature**: `user-auth`
- **Discovery Scope**: Complex Integration — new auth subsystem replacing existing static-token middleware. Slots into established clean-hex patterns, no new architectural style; security-sensitive decisions concentrated in token format, cookie attributes, and password handling.
- **Key Findings**:
  - The codebase enforces a strict clean-hex layout with ports in `internal/domain/<entity>/ports.go` and cross-cutting ports in `internal/domain/shared/ports.go`. The new feature must add `PasswordHasher`, `TokenSigner`, and `TokenValidator` as **cross-cutting ports** (not user-specific) because they encapsulate crypto primitives that other future aggregates could reuse.
  - The HTTP-layer error envelope ([internal/http/common/envelope.go](../../../internal/http/common/envelope.go)) already supports a `details.reason` machine-readable code via `HTTPError.WithReason(...)`. This is exactly the surface the requirements need for distinguishing missing/malformed/invalid/expired tokens — no new envelope plumbing required.
  - The existing `cmd/seed` binary (`internal/bootstrap/seed.go`) currently seeds hard-coded sources. User seeding needs operator-supplied credentials, so the seed binary will grow a subcommand pattern: `seed users <email> <password>` (separate from the default sources seed).
  - The integration test harness ([tests/integration/setup/setup.go](../../../tests/integration/setup/setup.go)) currently injects `X-API-Token: test-token` on every authenticated request. After this feature, the harness must (a) provision a test user via the user repo directly, then (b) acquire an access token via `POST /auth/login`, and (c) attach `Authorization: Bearer <token>` to authenticated calls. Every existing integration test will need its auth header updated.

## Research Log

### Existing layered architecture and aggregate template

- **Context**: Need a structural template to model the new `user-auth` feature after.
- **Sources Consulted**: [structure.md](../../steering/structure.md), `internal/domain/source/*`, `internal/application/source_usecase.go`, `internal/infrastructure/persistence/source/*`, `internal/http/controller/source_controller.go`, `internal/http/route/source_route.go`.
- **Findings**:
  - Aggregate pattern: `internal/domain/<entity>/{model.go, ports.go, requests.go, responses.go, errors.go}`.
  - Application: flat file `internal/application/<entity>_usecase.go` (the simpler aggregates follow this; complex ones get their own subdir like `application/analyzer/`).
  - Persistence: `internal/infrastructure/persistence/<entity>/{model.go, repo.go}` with `ToDomain()` / `FromDomain()` on the persistence side.
  - HTTP: `internal/http/controller/<entity>_controller.go` (+ `<entity>_responses.go` for shared envelope types), `internal/http/route/<entity>_route.go` with signature `func XxxRouter(d Deps)` that wires its own `repo → usecase → controller` chain locally.
  - Interfaces named `UseCase` and `Repository` (package-scoped: `user.UseCase`, `user.Repository`), implementing structs unexported, constructors return the interface.
- **Implications**: `user-auth` follows the same shape, with one deviation — token signing and password hashing are cross-cutting concerns that go in `internal/domain/shared/ports.go` (joining the existing `Logger`, `Clock`, `LLMClient`, `Extractor`, `Fetcher`) rather than `domain/user/ports.go`.

### Steering doc layer name vs reality

- **Context**: [structure.md](../../steering/structure.md) lists `internal/interface/` as the inbound-adapter layer, but the live codebase uses `internal/http/` directly.
- **Findings**: Live paths are `internal/http/{common,controller,middleware,route}/`. Steering text is aspirational or stale.
- **Implications**: Design follows live paths (`internal/http/...`). Updating the steering doc is out of scope for this spec.

### Error envelope and machine-readable reason codes

- **Context**: Requirements 2, 3, 5, 6, 7, 9 all reference distinct reason codes (e.g., `expired_access_token`, `current-password-incorrect`, `password-policy-violation`). Need to confirm the envelope supports this.
- **Sources Consulted**: [internal/domain/shared/errors.go](../../../internal/domain/shared/errors.go), [internal/http/common/envelope.go](../../../internal/http/common/envelope.go), [internal/http/middleware/error_envelope.go](../../../internal/http/middleware/error_envelope.go).
- **Findings**: `shared.HTTPError` has a `WithReason(string)` chain method that populates `error.details.reason` in the JSON envelope. The `ErrorEnvelope` middleware reads the first error from `c.Errors`, type-asserts to `*shared.HTTPError`, and serializes accordingly. Handlers signal errors via `c.Error(err)`; the middleware does the rest.
- **Implications**: All auth-specific reason codes are wired via `shared.NewHTTPError(code, msg, cause).WithReason("...")`. No new envelope surface is needed. Reason-code strings are part of the public contract and become a Revalidation Trigger.

### JWT library selection — adopt over build

- **Context**: Requirements 1, 3 commit to stateless JWTs with separate access and refresh signing keys.
- **Sources Consulted**: Viability subagent results (golang-jwt/jwt v5.3.1, Jan 2026). CVE-2025-30204 fixed in v5.2.2. CVE-2024-51744 documents the `token.Valid` check requirement.
- **Findings**: `golang-jwt/jwt/v5` is the actively maintained, MIT-licensed, current major. Gridbot's reference uses v4 — we use v5 for new code.
- **Implications**: Adopt `github.com/golang-jwt/jwt/v5`. HS256 with separate access/refresh keys is appropriate for a single-issuer/single-verifier deployment. The `jwt_signer` implementation must check `token.Valid` alongside the parse error (CVE-2024-51744 footgun).

### Bcrypt cost and password length

- **Context**: Requirements 6.5 and 9.2 specify the password policy and bcrypt cost expectations.
- **Sources Consulted**: Viability subagent results — OWASP 2026 password storage cheat sheet recommends cost 12 minimum, 13–14 preferred. Bcrypt has a hard 72-byte input limit; longer inputs are silently truncated.
- **Findings**: Cost 12 is the 2026 baseline. Argon2id is the gold standard but adds a dependency the project does not currently carry. Bcrypt at cost 12 meets the safety bar with smaller scope.
- **Implications**: Implementation uses `bcrypt.GenerateFromPassword(pw, 12)` with the cost set in code, not relying on `bcrypt.DefaultCost`. The validation layer rejects any password longer than 72 bytes with a `password-too-long` reason code (Requirement 9.5), so truncation cannot silently weaken security.

### Refresh-cookie security attributes

- **Context**: Requirement 1.2 nails the cookie attributes; the viability check surfaced subtler footguns.
- **Sources Consulted**: Viability subagent results; OWASP CSRF cheat sheet.
- **Findings**:
  - `Path=/auth/refresh` prevents the cookie from being attached to every API call.
  - Omitting `Domain` makes the cookie host-only — narrower, safer.
  - `SameSite=Strict` plus `Origin`/`Referer` check on the refresh endpoint (Requirement 3.5) is sufficient CSRF mitigation without adding a CSRF token.
  - Local-development `http://` requires `Secure` to be disabled. Config-driven, not hard-coded.
- **Implications**: Implementation reads `AUTH_COOKIE_INSECURE` from env (default `false`); when `true`, the `Secure` flag is omitted. The flag is loud and clearly named to avoid accidental production use.

### Why no `type` claim on tokens despite shared library

- **Context**: A common JWT footgun is using a refresh token where an access token is expected (or vice versa). Many implementations add a `typ: "access" | "refresh"` claim as defense-in-depth.
- **Findings**: We use **separate signing keys** for access and refresh (Requirement 3.9). A refresh token presented as an access token will fail signature verification immediately because the access-token verifier uses the access key. The signing-key separation provides the same guarantee a `typ` claim would, without adding a claim consumers need to validate.
- **Implications**: Claims stay minimal: `sub` (user id), `iat`, `exp`. No `typ`, no `email`, no roles. This keeps the token small and the contract narrow.

### Token TTL configurability

- **Context**: Requirements 1.7, 1.8, 3.8 specify 15-minute access and 24-hour refresh expiries.
- **Findings**: Hardcoding TTLs would make local testing painful and any future tuning a code change. Env-driven TTLs with the documented defaults are operator-observable and testable.
- **Implications**: `AUTH_ACCESS_TTL` and `AUTH_REFRESH_TTL` env fields with the documented defaults (`15m`, `24h`). Tests can shrink them to validate expiry paths quickly.

## Architecture Pattern Evaluation

| Option | Description | Strengths | Risks / Limitations | Notes |
|--------|-------------|-----------|---------------------|-------|
| Clean-hex with per-aggregate ports (chosen) | New `user` aggregate under `domain/user/`, application use-case, GORM repo, HTTP controller — mirrors `source` template exactly | Slots into the existing convention with zero deviation; reviewer can validate by analogy | None — this is the established pattern | Selected |
| Single fat "AuthService" stitching DB + crypto + HTTP | Skip the aggregate split, put everything in one service struct | Less file count | Violates [structure.md](../../steering/structure.md) dependency rule; can't test domain in isolation; opaque to reviewers | Rejected |
| Domain-local crypto ports (PasswordHasher in `domain/user/ports.go`) | Put password and token interfaces inside the user aggregate | Tight cohesion within one package | Future aggregates can't reuse these primitives without dragging in `user` package; muddies the user aggregate's responsibility | Rejected — chose cross-cutting placement in `domain/shared/ports.go` |

## Design Decisions

### Decision: Cross-cutting placement of `PasswordHasher`, `TokenSigner`, `TokenValidator`

- **Context**: Requirements 1, 2, 3, 6, 9 imply crypto primitives. They could live with the user aggregate or in `domain/shared`.
- **Alternatives Considered**:
  1. `domain/user/ports.go` — colocate with the consumer.
  2. `domain/shared/ports.go` — alongside `Logger`, `Clock`, `LLMClient`.
- **Selected Approach**: Option 2. New cross-cutting ports added to `internal/domain/shared/ports.go`.
- **Rationale**: These are crypto primitives, not user-specific. They have no concept of "user" in their signatures (`Hash(plain) -> string`, `IssueAccess(subject) -> token`). Placing them in `shared` makes them reusable for any future aggregate that needs signed tokens or hashed credentials, and matches the convention used for every other cross-cutting capability in this codebase.
- **Trade-offs**: Slightly more imports at the use-case (one extra package), in exchange for a cleaner aggregate and reusable primitives.
- **Follow-up**: When implementing, confirm no circular import emerges between `domain/shared` and any concrete adapter package.

### Decision: Split `TokenSigner` and `TokenValidator` into two interfaces

- **Context**: Use-cases need to issue tokens (login, refresh); the JWT middleware needs only to validate them. A single combined interface would force the middleware to depend on issuance methods it never calls.
- **Alternatives Considered**:
  1. Single `TokenService` with both issuance and verification methods.
  2. Split into `TokenSigner` (issue) and `TokenValidator` (verify) — two interfaces, one concrete adapter implements both.
- **Selected Approach**: Option 2.
- **Rationale**: Interface segregation — the middleware's blast radius narrows to verification. A test fake for the middleware doesn't need to implement issuance, and vice versa for the use-case.
- **Trade-offs**: One extra named interface in `domain/shared/ports.go` (~10 lines).
- **Follow-up**: The concrete `JWTTokenService` implements both interfaces; the bootstrap wires the same instance to both consumers.

### Decision: Stateless tokens accept "no revocation" — explicitly

- **Context**: Requirement 6.7 acknowledges that access tokens issued before a password change remain valid until expiry. This is the predictable consequence of fully stateless tokens with no rotation or jti table.
- **Alternatives Considered**:
  1. Refresh-token rotation + reuse-detection table (introduced in viability check) — gives real revocation.
  2. Pure stateless, no rotation — accept the window.
- **Selected Approach**: Option 2, per explicit user decision documented in [brief.md](brief.md).
- **Rationale**: Operator chose minimal surface over revocation. Short refresh TTL (24h) bounds the blast radius.
- **Trade-offs**: A compromised refresh token is valid for up to 24h. A password change does not invalidate outstanding access tokens (max 15min window).
- **Follow-up**: If a future spec needs revocation, it introduces a `refresh_token_jti` table and rotation logic; the current `TokenSigner`/`TokenValidator` shape remains compatible.

### Decision: Subcommand for user seeding in `cmd/seed`

- **Context**: Requirement 7 says users are created via the existing seed binary. Existing `cmd/seed/main.go` runs a fixed batch (`SeedSources`). User seeding needs operator-supplied email + password — not hardcodable.
- **Alternatives Considered**:
  1. Separate `cmd/seed-user/` binary.
  2. Add CLI subcommand: `seed users <email> <password>` (default `seed` continues to run sources).
  3. Mutate `internal/bootstrap/seed.go` to read users from env vars.
- **Selected Approach**: Option 2.
- **Rationale**: Keeps the operator surface single ("the seed binary"). Preserves the default no-op behavior for non-user seed runs. Env-driven approach was rejected because it tempts secrets into long-lived env vars.
- **Trade-offs**: `cmd/seed/main.go` grows a tiny arg-router using stdlib flag/os.Args. No CLI framework needed.
- **Follow-up**: Document the subcommand in the seed binary's `-h` output.

## Risks & Mitigations

- **Cookie attributes set incorrectly** — silently weakens auth security. *Mitigation*: integration tests inspect the `Set-Cookie` header on login responses and assert `HttpOnly`, `SameSite=Strict`, `Path=/auth/refresh`, and `Secure` flag presence per env.
- **`token.Valid` not checked alongside parse error** — known CVE-2024-51744 footgun lets some malformed tokens pass. *Mitigation*: implementation explicitly checks `token.Valid`; integration test forges a token with structural issues and asserts 401.
- **Test harness updates touching every existing integration test** — large mechanical change risks merge churn if other branches add tests in parallel. *Mitigation*: harness exposes a `setup.AuthorizedRequest(t, method, path, body)` helper; test files swap one line each.
- **Stateless tokens make incident response harder** — if a refresh token leaks, the operator cannot revoke it. *Mitigation*: ops play is "rotate the refresh signing key" — invalidates all outstanding refresh tokens immediately. Document this in the design's Operational Notes.
- **Bcrypt cost set too high** — login latency on slow machines becomes user-visible. *Mitigation*: cost 12 is the OWASP 2026 baseline; tests measure login p95 < 250ms on the CI machine. If we ever drop below, the cost is configurable to allow tuning.

## References

- [golang-jwt/jwt v5 — README and migration guide](https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md) — adopted library.
- [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) — bcrypt cost 12 baseline.
- [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html) — SameSite + Origin defense in depth.
- [CVE-2024-51744 — golang-jwt token.Valid footgun](https://github.com/golang-jwt/jwt/security/advisories/GHSA-29wx-vh33-7x7r) — implementation guard.
- [structure.md](../../steering/structure.md), [tech.md](../../steering/tech.md), [testing.md](../../steering/testing.md), [product.md](../../steering/product.md) — internal steering documents the design aligns with.
