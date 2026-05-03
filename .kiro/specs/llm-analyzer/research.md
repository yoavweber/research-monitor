# Gap Analysis: llm-analyzer

Date: 2026-05-02. Compares the eight requirements in [requirements.md](./requirements.md) against the current backend codebase to inform `/kiro-spec-design`. Focus: which assets exist, what is missing, which integration patterns the design must honor, and where credible alternatives diverge.

## Path correction up front

The brief uses `interface/http/controller/analyzer` for the HTTP layer. The project's actual layout is `internal/http/controller/<feature>/` — see [internal/http/controller/extraction/controller.go](../../../internal/http/controller/extraction/controller.go) and [internal/http/controller/paper/controller.go](../../../internal/http/controller/paper/controller.go). The design must use `internal/http/controller/analyzer/`. No requirement language depends on the wrong path; this is a brief-level slip to fix in design without re-running requirements.

## 1. Current state — what already exists

### 1.1 Reusable assets (analyzer can consume as-is)

| Asset | Location | What it gives the analyzer |
|---|---|---|
| `extraction.Repository.FindByID` | [domain/extraction/ports.go:18](../../../internal/domain/extraction/ports.go#L18) | Lookup by `extraction_id` returning the row + `body_markdown` |
| `domain.Extraction.Status` + `body_markdown` | [persistence/extraction/model.go:23-39](../../../internal/infrastructure/persistence/extraction/model.go#L23-L39) | Source of truth for "ready to analyze" gating |
| `shared.LLMClient` port | [domain/shared/ports.go:22-39](../../../internal/domain/shared/ports.go#L22-L39) | Port already defined; analyzer is its first consumer |
| `shared.Logger`, `shared.Clock` | [domain/shared/ports.go:9-20](../../../internal/domain/shared/ports.go#L9-L20) | Standard injected ports for use case + repo |
| `middleware.APIToken` | `internal/http/middleware/api_token.go:13-22` | Existing `X-API-Token` auth — analyzer mounts under the same `/api` group |
| `middleware.ErrorEnvelope` + `shared.HTTPError` + `shared.AsHTTPError` | `internal/http/middleware/error_envelope.go:14-27`, `internal/domain/shared/errors.go:8-37` | Standard sentinel-to-HTTP translation; analyzer defines typed sentinels and the middleware does the rest |
| `common.Data` / `common.Err` envelopes | `internal/http/common/envelope.go:1-28` | Standard response shape |
| Bootstrap wiring + AutoMigrate | `internal/bootstrap/app.go:27-169`, `internal/infrastructure/persistence/migrate.go` | Pattern for adding analyzer repo + use case + controller + AutoMigrate of the new `analyses` table |
| `testdb.New(t)` | `tests/testdb/db.go:20-34` | In-memory SQLite + AutoMigrate for repo tests |

### 1.2 Pattern templates (analyzer must mirror, not reuse)

- **Domain ports**: [domain/extraction/ports.go:64-85](../../../internal/domain/extraction/ports.go#L64-L85) shows the canonical `UseCase` shape — context-first, package-qualified callsite (`extraction.UseCase`).
- **Application use case**: [application/extraction/usecase.go:18-43](../../../internal/application/extraction/usecase.go#L18-L43) shows constructor (`NewExtractionUseCase`), unexported struct, dependencies injected.
- **Persistence model + conversion**: [persistence/paper/model.go](../../../internal/infrastructure/persistence/paper/model.go) shows `TableName()`, `FromDomain` / `ToDomain`, sentinel translation (`gorm.ErrRecordNotFound` → `domain.ErrNotFound`, others wrapped as `ErrCatalogueUnavailable`).
- **Race-safe upsert** (most important precedent): [persistence/extraction/repo.go:42-129](../../../internal/infrastructure/persistence/extraction/repo.go#L42-L129) does **not** use `gorm.io/gorm/clause.OnConflict`. It runs a transaction: insert, catch `gorm.ErrDuplicatedKey`, then an explicit `UPDATE ... WHERE id = ?` via `map[string]any` so zero-values land. This is the precedent the analyzer's overwrite-on-rerun should follow (Requirement 3.1-3.5).
- **Controller + route**: `internal/http/controller/extraction/controller.go` and `internal/http/route/extraction_route.go` show the registration pattern via `route.Deps`.

### 1.3 Configuration surface

- Env struct: `internal/bootstrap/env.go:12-162`, viper-backed, holds `APIToken`, `SQLitePath`, `AnthropicAPIKey`, `AnthropicModel`, etc. The analyzer needs a new field — proposed `LLMProvider` (default `"fake"`) — but the *real* provider config (API key, model) already exists for the future Anthropic adapter.

## 2. Requirement-to-asset map (with gaps)

| Req | Capability needed | Existing asset | Gap |
|---|---|---|---|
| 1.1 | Resolve `extraction_id` to body markdown | `extraction.Repository.FindByID` ✓ | None |
| 1.1 | Three LLM calls | `shared.LLMClient` port ✓ | **Missing**: zero implementations exist |
| 1.2 | Persist analysis keyed by `extraction_id` | GORM + AutoMigrate pattern ✓ | **Missing**: no `analyses` table, no `domain/analyzer`, no `infrastructure/persistence/analyzer/` |
| 1.3 | HTTP 200 + JSON body with the documented shape | `common.Data` envelope ✓ | **Missing**: controller + route + DTO |
| 1.4 | Synchronous response | Project default ✓ | None |
| 1.5 | Three independent LLM invocations | n/a | Pure use-case orchestration; no asset gap |
| 2.1-2.3 | `GET /analyses/:extraction_id` read-back | Controller / route pattern ✓ | **Missing**: handler |
| 3.1-3.4 | Overwrite-on-rerun, preserve `created_at` | Extraction's transaction-based upsert pattern (precedent) | **Missing**: analyzer-specific upsert; **Constraint**: must mirror the no-`clause.OnConflict` precedent |
| 3.5 | Concurrent rerun convergence | Same precedent ✓ | **Constraint**: design must specify the transaction strategy explicitly (extraction's pattern is the recommended one) |
| 4.1-4.4 | 404 / 409 / 400 with no LLM call | `extraction.Status` enum ✓; `shared.HTTPError` sentinel translation ✓ | **Missing**: analyzer sentinels (`ErrExtractionNotFound`, `ErrExtractionNotReady`, etc.); they wrap `*shared.HTTPError` with codes 404/409/400 |
| 5.1-5.4 | 502 with distinct transport vs. malformed indicator | `shared.HTTPError` carries code + message ✓ | **Unknown**: confirm the error envelope's body shape carries enough structure to distinguish the two without forcing the researcher to parse the message string. **Research Needed** in design — see §6 |
| 5.5 | 500 on persistence failure after successful LLM run | `ErrCatalogueUnavailable` precedent ✓ | None |
| 6.1-6.4 | Strict thesis-angle JSON envelope, parse + validate | `encoding/json` (stdlib) | **Missing**: parser + validator inside `application/analyzer` |
| 7.1-7.4 | Default fake LLM provider, deterministic per prompt | n/a | **Missing**: `infrastructure/llm/fake/` does not exist; no LLM adapter directory exists at all |
| 8.1-8.3 | `X-API-Token` auth, JSON only | `middleware.APIToken` mounted on `/api` ✓ | None — analyzer mounts on `/api` and inherits |

**Summary of gaps**:
- **Missing (build new)**: `domain/analyzer` package, `application/analyzer` use case, `infrastructure/persistence/analyzer/` repo + GORM model, `infrastructure/llm/fake/` adapter, `internal/http/controller/analyzer/` controller, `internal/http/route/analyzer_route.go`, AutoMigrate registration of the new model, env var `LLMProvider` + bootstrap wiring branch.
- **Constraint**: upsert must follow the extraction transaction pattern, not introduce `clause.OnConflict`.
- **Research Needed (design phase)**: error envelope shape for distinguishing 502-transport vs. 502-malformed (Req 5.4); naming convention for the fake LLM's `model` and `prompt_version` strings so they're stable across reruns and tests.

## 3. Implementation approach options

### Option A: Extend existing components

Not viable as a primary strategy. None of the existing packages are a natural home for analyzer logic:
- `domain/extraction` is single-purpose (extract → markdown); folding "analyze markdown" in would violate single-responsibility and re-introduce the `paper`/`extraction` cross-talk the brief explicitly avoids.
- `domain/paper` is read-only metadata.
- There is no existing LLM adapter directory to extend.

Trade-off summary: ❌ wrong on responsibility, ❌ no real shortcut, ❌ couples specs that should stay independent. Skip.

### Option B: Create new components (recommended)

Build a new vertical slice, mirroring the layout proven by `extraction` and `paper`:

```
internal/domain/analyzer/                    NEW
  model.go         Analysis value type
  ports.go         UseCase, Repository
  errors.go        ErrExtractionNotFound, ErrExtractionNotReady,
                   ErrLLMUpstream, ErrAnalyzerMalformedResponse,
                   ErrAnalysisNotFound, ErrCatalogueUnavailable
internal/application/analyzer/               NEW
  usecase.go       analyzerUseCase + NewAnalyzerUseCase
  prompts.go       short/long/thesis prompt strings + prompt_version
  parse.go         thesis-angle JSON envelope parser + validator
internal/infrastructure/persistence/analyzer/  NEW
  model.go         GORM Analysis row, TableName(), From/ToDomain
  repo.go          repository + race-safe upsert (extraction-style txn)
internal/infrastructure/llm/fake/            NEW (also creates infra/llm/)
  client.go        FakeClient implementing shared.LLMClient,
                   keyed by LLMRequest.PromptVersion
internal/http/controller/analyzer/           NEW
  controller.go    POST /analyses, GET /analyses/:extraction_id
  responses.go     wire DTOs
internal/http/route/analyzer_route.go        NEW
internal/bootstrap/app.go                    EXTEND: wire repo + use case + fake client + AutoMigrate
internal/bootstrap/env.go                    EXTEND: add LLMProvider field (default "fake")
internal/infrastructure/persistence/migrate.go  EXTEND: register analyzer.Analysis
tests/mocks/llm_client.go                    NEW (a bare double for use-case tests, separate from the production fake)
tests/integration/analyzer_test.go           NEW
```

**Why B**:
- Mirrors the established vertical-slice pattern (every other feature lives this way).
- Each new file has one job; existing files barely change (env, app, migrate get one block each).
- The new `infrastructure/llm/` directory is the right home for the eventual real Anthropic adapter; creating it now means the future swap is a peer-package addition, not a layout change.
- Distinguishes cleanly between the **production fake** (`infrastructure/llm/fake/` — wired by bootstrap, used in dev/prod-without-API-key) and a **test double** under `tests/mocks/llm_client.go` for use-case unit tests with controllable per-call behavior. The testing steering allows both.

**Trade-offs**:
- ✅ Clean boundaries; the analyzer doesn't touch `extraction`, `paper`, or any sibling spec.
- ✅ Parallel testability — repo with `testdb.New`, use case with mock LLMClient, controller with httptest.
- ✅ Adding the real Anthropic adapter is a single-package addition (`infrastructure/llm/anthropic/`) with no use-case changes, satisfying Req 7.4.
- ❌ More files than a brownfield extend, but consistent with the rest of the repo.

### Option C: Hybrid — share an LLM facade

Hypothesis: rather than `application/analyzer` calling `shared.LLMClient` directly three times, introduce a thin shared helper (e.g., `application/llm/`) that wraps "run prompt with version, return text-or-envelope". Use case becomes thinner; future LLM consumers (triage, classification) can reuse the helper.

**Trade-offs**:
- ✅ Slightly less code if a second LLM consumer arrives soon.
- ❌ Speculative — only one LLM consumer exists today; this violates the project rule against premature abstractions inside a layer (CLAUDE.md "Boring > clever").
- ❌ Adds a new layer (`application/llm/`) that has no obvious owner.
- ❌ The use case already owns prompts and parsing; pulling those into a shared helper either duplicates them or forces the helper to know about envelope shapes.

**Recommendation**: defer until a second LLM consumer appears. If/when it does, the refactor is local and obvious. Stay with Option B for now.

## 4. Complexity & risk

- **Effort: M (3-7 days)**.
  - New package skeletons (domain + app + persistence + fake LLM + controller + route): ~1.5 days.
  - Race-safe upsert mirroring extraction's pattern + integration tests: ~1 day.
  - Use case orchestration + thesis-envelope parser + use-case tests: ~1 day.
  - Controller + httptest cases for every status code in Req 4 and 5: ~1 day.
  - Bootstrap wiring + AutoMigrate + integration test: ~0.5 day.
  - Steering steering doc updates / Swagger annotations / lint pass: ~0.5 day.
- **Risk: Low**.
  - Every subsystem has a clear template (extraction is the closest analog).
  - No new infrastructure (still SQLite + Gin + GORM).
  - No external network in this slice (fake LLM only); the riskiest integration is deferred to the real-provider follow-up.
  - One genuinely new directory (`infrastructure/llm/`) but the pattern is identical to other infra packages.

## 5. Recommendations for design phase

1. **Adopt Option B** end-to-end. Mirror the `extraction` vertical slice; do not extend or reuse other packages.
2. **Upsert strategy**: explicitly commit to the extraction-style transaction + duplicated-key-catch + UPDATE pattern in `design.md`. Pseudocode the transaction so reviewers can audit `created_at` preservation and concurrency convergence (Req 3.2 / 3.5).
3. **Sentinel-to-HTTP map**: the design should enumerate every analyzer sentinel and the `*shared.HTTPError` it wraps. Mapping target:
   - `ErrAnalysisNotFound` → 404
   - `ErrExtractionNotFound` → 404
   - `ErrExtractionNotReady` → 409
   - `ErrLLMUpstream` → 502, with a stable `code` discriminator (e.g., `"llm_upstream"`)
   - `ErrAnalyzerMalformedResponse` → 502, with a stable `code` discriminator (e.g., `"llm_malformed_response"`)
   - `ErrCatalogueUnavailable` → 500
   - Bad JSON / missing field → 400 (handled by Gin binding; design specifies the shape)
4. **Error envelope discriminator (Req 5.4)** — open question: confirm `shared.HTTPError` already exposes a stable machine-readable `code` field on the wire, distinct from the human-readable `message`. If yes, satisfy Req 5.4 by using two distinct codes for the two 502 modes. If the wire shape only carries `message`, design must add a `code` field to the envelope or to the analyzer's response. Resolve in `kiro-validate-design`.
5. **Fake LLM contract** — pin in design:
   - Keyed by `LLMRequest.PromptVersion`.
   - Distinct canned outputs for `analyzer.short.v1`, `analyzer.long.v1`, `analyzer.thesis.v1` (suggested naming).
   - Thesis fake returns valid `{"flag": false, "rationale": "fake-rationale"}` so the parser path is exercised (Req 7.3).
   - `Model` field returns a fixed string like `"fake"` so the persisted `model` is stable across reruns (matters for Req 3.1 overwrite assertions).
6. **Bootstrap switch** — add `LLM_PROVIDER` env var (default `"fake"`). For now only `"fake"` is implemented; `"anthropic"` slot is reserved and returns a clear startup error if selected. Avoids dead config branches.
7. **Test plan** (per testing.md):
   - Repository: real SQLite via `testdb.New`, exercises insert / overwrite / created_at preservation / concurrent-rerun convergence.
   - Use case: hand-written `tests/mocks/llm_client.go` double with per-call programmable behavior (success, transport error, malformed thesis JSON, missing field, wrong type).
   - Controller: httptest + `gin.TestMode`, asserts every status code in Req 1-8 against an in-memory use-case fake.
   - Integration: `tests/integration/analyzer_test.go` exercises the full HTTP→repo path with the production fake LLM client.
8. **Brief correction**: design doc must use `internal/http/controller/analyzer/` (not the brief's `interface/...` path). Requirements language is path-agnostic, so no requirement edit needed.

## 6. Research items to carry forward to design

- **R-1**: Wire-level shape of `shared.HTTPError` after `ErrorEnvelope` middleware translation. Specifically: does the JSON body carry a stable machine code field independent of the message string? Drives whether Req 5.4 needs a new field or just two distinct codes. Resolution lives in `design.md` after reading `internal/http/middleware/error_envelope.go` and `internal/domain/shared/errors.go` end-to-end.
- **R-2**: Whether the project has a Swagger annotation convention for new controllers. The brief's HTTP surface ships with Swagger UI under `APP_ENV != prod` (per tech.md); design should decide whether analyzer endpoints get annotated now or whether annotation is a follow-up.
- **R-3**: Whether `LLM_PROVIDER` should live as a top-level env var or inside an `LLM` substruct (env.go currently flattens; design should match the existing style consistently).

None of these block design generation; all are local clarifications resolvable inside `/kiro-spec-design`.

---

# Design Synthesis (added during /kiro-spec-design)

Date: 2026-05-02. Captures the synthesis lenses (generalization, build-vs-adopt, simplification) and the resolutions of carry-forward research items R-1, R-2, R-3 prior to writing `design.md`.

## Synthesis lenses

### Generalization

The eight requirements collapse to four underlying behaviors: produce-and-store (R1, R3, R6), retrieve (R2), reject upstream-precondition failures (R4, R5, R8), and provide a substitutable LLM provider (R7). The shared abstraction is **the `shared.LLMClient` port**, which is already declared and which the design adopts as-is. No new internal abstraction is introduced — the use case calls `LLMClient.Complete` three times directly. Generalizing further (e.g., a `PromptRunner` helper) is rejected as premature; only one consumer exists.

### Build vs. adopt

| Concern | Adopt / Build | Rationale |
|---|---|---|
| HTTP framework, middleware (auth, error envelope) | Adopt | `gin`, `middleware.APIToken`, `middleware.ErrorEnvelope`, `common.Envelope` are the project standard |
| Persistence | Adopt (GORM/SQLite) | Matches every other feature; AutoMigrate already wired |
| Race-safe upsert | Adopt extraction's pattern | Transaction + duplicated-key-catch + UPDATE; precedent exists in [persistence/extraction/repo.go:42-129](../../../internal/infrastructure/persistence/extraction/repo.go#L42-L129); no `clause.OnConflict` |
| LLM port | Adopt | `shared.LLMClient` already declared at [domain/shared/ports.go:37](../../../internal/domain/shared/ports.go#L37) |
| Fake LLM adapter | Build | First implementation of the port; trivial; the only build option |
| JSON envelope parsing | Build (stdlib `encoding/json`) | One-shot validator; no schema lib needed for `{flag: bool, rationale: string}` |
| Error discriminator (R-1) | Extend `shared.HTTPError` | Smaller and more reusable than building an analyzer-local discriminator path; see R-1 resolution below |

### Simplification

- **No `application/llm/` facade**. Use case calls `LLMClient.Complete` three times directly; the prompts are constants in the analyzer package.
- **No retry / repair logic**. Both transport errors and JSON-parse errors fail the request immediately (per Req 5.3); no retry budget is introduced.
- **No analyzer-specific HTTP error path**. Sentinels wrap `*shared.HTTPError`; the existing `ErrorEnvelope` middleware does the translation. The only addition is an optional `Reason` field on `HTTPError` (R-1 resolution).
- **No use-case interface for the use case's internal helpers**. The thesis-envelope parser and prompt assembly are unexported functions inside `application/analyzer`, not ports.
- **No analyzer-specific test infra**. Reuse `tests/testdb.New(t)` for repository tests and the existing `tests/mocks/` convention for hand-written doubles.

## Carry-forward research resolutions

### R-1 resolved: Error envelope discriminator

**Finding**: The wire shape produced by `middleware.ErrorEnvelope` (see [internal/http/middleware/error_envelope.go:14-27](../../../internal/http/middleware/error_envelope.go#L14-L27) and [internal/http/common/envelope.go:13-21](../../../internal/http/common/envelope.go#L13-L21)) is `{"error": {"code": <http_status>, "message": <string>, "details": <map>}}`. The `code` is the HTTP status, not a machine discriminator. Two distinct 502 sentinels would today produce envelopes that are byte-for-byte indistinguishable except in `message`, which is human-readable.

**Resolution**: Extend `shared.HTTPError` with an **optional** `Reason string` field. Update `middleware.ErrorEnvelope` to populate `error.details.reason` when `Reason != ""`. This is a small additive cross-cutting change to project-shared infrastructure. Backward compatibility: every existing call site that uses `shared.NewHTTPError(code, message, err)` continues to work unchanged because the new field defaults to empty and is omitted from the envelope.

**Analyzer-specific use**:
- `ErrLLMUpstream` → `*shared.HTTPError{Code: 502, Reason: "llm_upstream", Message: "..."}` → wire envelope contains `details.reason = "llm_upstream"`.
- `ErrAnalyzerMalformedResponse` → `*shared.HTTPError{Code: 502, Reason: "llm_malformed_response", Message: "..."}` → wire envelope contains `details.reason = "llm_malformed_response"`.

This satisfies Requirement 5.4 with one additive infrastructure change shared across the project. Modified files: `internal/domain/shared/errors.go`, `internal/http/middleware/error_envelope.go`. The middleware change is mechanical (one branch); the shared type change adds one optional field with a no-op zero value.

### R-2 resolved: Swagger annotations

**Finding**: `tech.md` states `swaggo/gin-swagger` is the project standard; `task swag` regenerates `docs/`. Existing controllers carry annotations.

**Resolution**: Annotate the two new endpoints (`POST /analyses`, `GET /analyses/:extraction_id`) following the existing pattern. Run `task swag` post-implementation. Annotation is in scope for this slice; not a follow-up.

### R-3 resolved: Env-var nesting style

**Finding**: [internal/bootstrap/env.go:12-33](../../../internal/bootstrap/env.go#L12-L33) is flat — every field is a top-level `mapstructure` tag. There is no substruct convention for related groups.

**Resolution**: Add a single top-level `LLMProvider string` field with `mapstructure:"LLM_PROVIDER"`, default `"fake"`. Bootstrap validates that the value is one of the supported providers (`"fake"` for now; `"anthropic"` reserved and returns a clear startup error until that adapter ships). Existing `AnthropicAPIKey` and `AnthropicModel` fields stay where they are.

## Decisions captured

| Decision | Rationale |
|---|---|
| Vertical-slice layout mirroring `extraction` | Consistency with the rest of the repo; clear boundaries; parallel-safe testability |
| Race-safe upsert via transaction + duplicated-key-catch + UPDATE | Mirrors [extraction repo.go:42-129](../../../internal/infrastructure/persistence/extraction/repo.go#L42-L129); avoids introducing `clause.OnConflict` to the codebase |
| Extend `shared.HTTPError` with optional `Reason` | Smallest project-wide change that satisfies Req 5.4; backward-compatible additive |
| Production fake LLM lives at `infrastructure/llm/fake/`; test double at `tests/mocks/llm_client.go` | Production fake exercises the real provider seam end-to-end; the test double supports per-call programmable behavior in use-case unit tests |
| Prompt versions: `analyzer.short.v1`, `analyzer.long.v1`, `analyzer.thesis.v1` | Stable strings drive the fake's deterministic output and are persisted on every analysis row |
| Single `LLM_PROVIDER` env var, default `"fake"` | Matches flat env-var convention; no dead config branches at startup |
| Swagger annotations included in this slice | `tech.md` standard; cheap to do alongside controller |

