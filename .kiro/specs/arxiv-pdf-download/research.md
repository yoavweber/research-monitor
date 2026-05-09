# Gap Analysis: arxiv-pdf-download

## 1. Current State

### Existing assets the feature can reuse

| Concern | Location | Notes |
|---|---|---|
| PDF byte fetch + atomic publish + cache gate | `internal/infrastructure/pdf/local/store.go` | `Ensure(ctx, Key)` is idempotent, concurrent-safe, returns wrapped errors via `pdf.ErrFetch` / `pdf.ErrStore` / `pdf.ErrInvalidKey`. **Reuse unchanged.** |
| PDF domain port | `internal/domain/pdf/ports.go` | `Store` interface already abstracts the byte source. |
| Failure categories | `internal/domain/pdf/events.go` | `CategoryInvalidKey`, `CategoryFetch`, `CategoryStore` map 1:1 to the requirement's "stable failure category". **Reuse for SSE event payloads and status responses.** |
| arXiv use case + per-entry `IsNew` | `internal/application/arxiv/usecase.go` | Already returns `[]Result{Entry, IsNew}`. R1 trigger condition is computable here without changes to the persistence path. |
| `paper.Entry` carries `PDFURL`, `SourceID`, `Version` | `internal/domain/paper/model.go` | Sufficient input to construct `pdf.Key{SourceType:"arxiv", SourceID: …, URL: PDFURL}`. |
| HTTP layer composition | `internal/http/route/route.go` + per-resource router files | New `DownloadConfig` sub-bundle slots in alongside `PDFConfig`. New `DownloadRouter` follows the existing per-resource pattern. |
| Bootstrap composition root | `internal/bootstrap/app.go` | Already constructs `pdfStore` and `arxivFetcher`. Single place to construct the new scheduler/registry and wire it into both arxiv and download routers. |
| Logging port | `internal/domain/shared.Logger` | Use for job lifecycle (R6 acceptance criterion 5). |
| Clock port | `internal/domain/shared.Clock` | Use for retention-window eviction (R6.2/R6.3) so tests stay deterministic. |
| Test mocks layout | `tests/mocks/` (hand-written, no codegen) | New port (`pdfdownload.Scheduler`, registry reader) gets a hand-written fake here. |
| Integration harness | `tests/integration/setup/SetupTestEnv(t)` | SSE end-to-end test plugs in here; integration build tag `integration` already exists. |

### Conventions that constrain the new code

- Inward-only dependency rule (`structure.md` §2). New port must live under
  `internal/domain/` (or `internal/application/` if it owns a registry that
  is application-scoped); arxiv use case may only depend on the port.
- Constructor returns interface (`structure.md` §7). Implementing struct
  unexported (`scheduler`, `jobRegistry`).
- `context.Context` is the first parameter on every method.
- All HTTP handlers must carry the swag annotation block (`structure.md`
  §6.1) including a `@Failure` per status. Run `task swag` before commit.
- Tests call `t.Parallel()`. Real-over-fake DB; doubles in `tests/mocks/`.
- `log/slog` only via `shared.Logger`.
- No production changes for test observability (steering memory).

### Discrepancy with the requirements

- Requirements assume `POST /api/arxiv/fetch`; the route as wired today is
  **`GET /api/arxiv/fetch`** (`internal/http/route/arxiv_route.go:19`).
  This is cosmetic — both verbs trigger the same handler — but the spec
  language must align with the actual surface or the surface must change.
  **Decision needed in design phase**: keep `GET` (no requirement breaks)
  or migrate to `POST` (more honest about side effects: persistence +
  scheduling). Flagged, not blocking.

## 2. Requirement-to-Asset Map

| Req | Capability needed | Existing asset | Gap |
|---|---|---|---|
| R1.1 | Trigger after persistence for `IsNew` entries | `arxiv.UseCase.Fetch` returns `[]Result` with `IsNew` | **Missing**: a `Scheduler` port and a call site in `Fetch` |
| R1.2 | No job when no new entries | Same call site | **Missing**: condition check |
| R1.3 | No job on fetch/persistence error | Same call site | **Missing**: only schedule on the success path |
| R1.4 | Visible deferred decision (future user-confirm flow) | n/a | **Missing**: a single comment in the use case at the trigger site |
| R2.1 | Non-blocking response | n/a | **Missing**: fire-and-forget worker; controller must not await job |
| R2.2 | `job_id` + `scheduled_for_download` in response | `controller.Fetch` builds `FetchEnvelope` via `ToFetchResponse` | **Constraint**: response shape extension; swag `FetchEnvelope` must be regenerated |
| R2.3 | Globally unique `job_id` within retention window | `github.com/google/uuid` already in stack (`tech.md`) | Use `uuid.NewString()` |
| R2.4 | Independent concurrent fetches | n/a | **Missing**: registry mutex discipline |
| R3.1 | Per-entry attempts | `pdf.Store.Ensure` | **Missing**: worker iterating entries |
| R3.2 / R3.3 | Per-entry outcome with bytes / category + description | `pdf.Category*` + `pdf.ErrFetch`/`ErrStore`/`ErrInvalidKey` | **Missing**: classifier (mirror `localStore.failFetch`/`failStore` taxonomy) and result struct |
| R3.4 | No duplicate artifacts under overlapping calls | `Store.Ensure` is already idempotent on `Key` | **Constraint satisfied** by underlying store |
| R3.5 | Worker survives request cancellation | `arxiv.UseCase.Fetch` receives request `ctx` | **Missing**: derive a background context for the job; do NOT pass the request `ctx` to the worker |
| R4.1 | Replay buffered events on subscribe | n/a | **Missing**: in-registry per-job event log |
| R4.2 | `download.progress` event payload | `pdf.Category*` | **Missing**: event encoder (controller layer) |
| R4.3 | Terminal `download.summary` then close | n/a | **Missing**: completion fan-out + stream close semantics |
| R4.4 | Client disconnect doesn't affect job/peers | Gin `c.Request.Context().Done()` + `c.Stream` already provide the hook | **Missing**: subscriber goroutine + per-subscriber buffered channel, drop-on-slow policy |
| R4.5 | Unknown/expired job → client error | n/a | **Missing**: 404 on missing `job_id` |
| R4.6 | Server-push only (no client data needed) | SSE protocol gives this for free | **Constraint satisfied** by transport choice |
| R5.1 / R5.2 / R5.3 | Per-job aggregate + per-entry status | n/a | **Missing**: registry reader interface + JSON encoder |
| R5.4 | Stream/status consistency | n/a | **Constraint**: events must be appended **before** subscribers are notified — single mutex-guarded write path in the registry |
| R6.1 | Retain in-progress jobs | n/a | **Missing**: registry storage |
| R6.2 / R6.3 | Retention window after completion | `shared.Clock` | **Missing**: TTL eviction loop or lazy-eviction-on-read |
| R6.4 | Lost on restart | In-memory registry | **Constraint satisfied** by storage choice |
| R6.5 | Lifecycle logging | `shared.Logger` | **Missing**: log lines at scheduled / completed / evicted with `job_id` and originating `source_id`s |

### Research Needed (carry to design)

- **R1**: keep `GET` or move to `POST` for the fetch endpoint; whichever, the spec language must match.
- **R3**: backoff/retry — explicitly out of scope, but the user marked the exclusion as "open to a short comment suggesting it." Design phase should propose a single `// TODO: retry policy` comment (or a Scheduler option struct seam) without implementing it.
- **R4**: SSE write semantics in Gin — `c.Stream` vs. manual `c.Writer.Flush()`; preferred pattern in this codebase (no SSE precedent yet — first use).
- **R4.4**: slow-subscriber policy. Drop oldest event? Disconnect the subscriber? Block the worker? Recommendation needed in design.
- **R6**: eviction trigger — background ticker (one goroutine per registry, respects `shutdown`) vs. lazy-on-read. Lazy is simpler; ticker is more predictable for tests.

## 3. Implementation Approach Options

### Option A — Extend existing `pdf` infrastructure with a thin scheduler

Place the scheduler under `internal/infrastructure/pdf/local/` as a sibling
of `localStore`. The scheduler owns a registry + worker goroutines and
calls into `localStore.Ensure`.

- ✅ Co-located with the byte-fetch implementation; one package to read.
- ❌ Violates layering: `infrastructure/pdf/local` would now own
  application-scoped state (jobs, subscribers). That makes the package
  reachable from controllers, blurring the inward dependency rule.
- ❌ Domain port `pdf.Store` would either grow (Ensure + Schedule) or stay
  narrow while the scheduler exposes a separate concrete type. Either
  way the arxiv use case ends up depending on a concrete or on a port
  that lives in infrastructure — both are wrong per `structure.md` §4.

**Not recommended.**

### Option B — New `pdfdownload` application aggregate (recommended)

Create a new application-layer aggregate:

- `internal/domain/pdfdownload/ports.go` — `Scheduler` (write side), `Reader` (read side for the status endpoint), `JobID` value type, `JobResult` / `EntryResult` value objects, `ErrJobUnknown` sentinel.
- `internal/application/pdfdownload/usecase.go` — the actual scheduler + registry + worker. Depends on `pdf.Store`, `shared.Logger`, `shared.Clock`. Returns the `Scheduler`/`Reader` ports.
- `internal/http/controller/pdfdownload/` — SSE handler, status handler, swag annotations, response DTOs.
- `internal/http/route/pdfdownload_route.go` — registers `GET /api/arxiv/downloads/{job_id}` and `GET /api/arxiv/downloads/{job_id}/stream` under the existing `/api` group.
- `internal/http/route/route.go` — new `DownloadConfig{Scheduler, Reader}` sub-bundle on `Deps`.
- `internal/bootstrap/app.go` — construct one registry; pass `Scheduler` into `route.ArxivConfig` and the same registry's `Reader` into `route.DownloadConfig`.

Arxiv use case acquires a `Scheduler` dependency (constructor signature grows by one) and calls `Scheduler.Schedule(ctx, jobInputs)` after the persistence loop. The returned `JobID` and scheduled `source_id`s are surfaced through the `arxiv.Result` (or a sibling field on the response). Trigger site carries the R1.4 comment.

- ✅ Clean layering: arxiv depends on a port in `domain/`. Worker lives in `application/`, where orchestration belongs. Controller depends on `Reader`.
- ✅ Single registry instance owns all in-memory state; mutex discipline is local to one package.
- ✅ Tests: `pdfdownload` is testable in isolation with a `pdf.Store` fake; arxiv use case test gets a `pdfdownload.Scheduler` fake under `tests/mocks/`.
- ✅ Future "user-confirm" flow swaps the trigger location without touching the scheduler — only the call site in arxiv (or a new use case) changes.
- ❌ Most new files of any option (~5 new packages/dirs).
- ❌ Two-port surface (`Scheduler`, `Reader`) needs careful naming to avoid
  the `Job{Reader,Scheduler}` stutter — Go convention is to scope by package.

**Recommended.** Matches `structure.md` §3 ("every aggregate lives at `internal/domain/<entity>/`") and §4 (port/impl split).

### Option C — Hybrid: scheduler in `application/pdfdownload`, no new domain package

Put the port directly under `internal/application/pdfdownload/ports.go`
and have the arxiv use case import it. Skip the `domain/pdfdownload`
package.

- ✅ Fewer files than Option B; arguably appropriate because the registry is
  pure orchestration (no domain entities of its own).
- ❌ Breaks `structure.md` §4: ports live in `domain/`. The arxiv
  application package would then import from another application
  package, which is allowed by the table but inconsistent with how
  every other aggregate in the repo is shaped (paper, source,
  extraction, analyzer all expose ports from `domain/<entity>`).
- ❌ Asymmetry costs more in confusion than it saves in files.

**Possible fallback if the design phase decides "no domain entities, no
domain package", but Option B is consistent with the rest of the repo.**

## 4. Effort & Risk

| Option | Effort | Risk | Justification |
|---|---|---|---|
| A | S | Medium | Few files but layering rot raises long-term cost; risk of bleeding registry state into infrastructure |
| **B** | **M** | **Low** | 5 small packages following established patterns; SSE is the only first-time-in-repo concern; pdf.Store and shared ports already exist |
| C | S | Medium | Saves one directory but breaks symmetry with the rest of the codebase, increases reviewer surprise |

First-time-in-repo concerns for Option B (carry to design):
- SSE: no prior usage. Need a sensible Gin pattern + integration test.
- Concurrency review: the registry is the only mutable shared state — must be exercised with `-race` and ideally a parallel-fetch integration test.

## 5. Recommendations for Design Phase

1. **Adopt Option B**. New aggregate `pdfdownload` under `domain/` + `application/` + `http/controller/` + `http/route/` + `tests/mocks/`.
2. **Resolve the GET vs. POST question** for `/api/arxiv/fetch` early in design; pick one and align requirements language. Recommendation: keep `GET` to avoid surface churn, document the side effect in the swag description.
3. **Define the slow-subscriber policy** in design (recommendation: per-subscriber buffered channel of size N; drop the subscriber on overflow rather than blocking the worker). Add a corresponding acceptance test.
4. **Pick eviction strategy**: lazy-on-read for simplicity, or background ticker for predictability. Recommendation: lazy + a `Sweep(ctx)` method exposed for tests so behavior is deterministic.
5. **Construct the worker context** explicitly (`context.Background()` derived from a registry-owned cancel) so request cancellation cannot abort downloads (R3.5). Hook the cancel into bootstrap shutdown.
6. **Reuse `pdf.CategoryInvalidKey/Fetch/Store`** as the on-the-wire `category` values for SSE payloads and the status response. Do not invent new strings.
7. **Add the R1.4 deferred-decision comment** at the call site only — single short comment, not a TODO scattered through multiple files.
8. **Honor the user's accepted retry comment**: design phase should propose where a future retry/backoff policy would attach (likely a `Scheduler` option struct or a wrapper around `pdf.Store.Ensure`) without implementing it.
9. **Swag**: extend `arxivctrl.FetchEnvelope` with the job fields; add new envelopes for the download status and SSE event payloads. Run `task swag` before commit.

## 6. Out-of-Scope for this Analysis (carry as research items)

- SSE writer pattern in Gin (research during design): `c.Stream(func(w io.Writer) bool {...})` vs. manual flushing.
- Whether reverse proxy / middleware in this stack already buffers SSE; verify in design with a smoke test against the existing Gin engine.
- Whether `internal/http/route/route.go::Deps` should split `PDFConfig` and `DownloadConfig`, or whether `DownloadConfig` should absorb the existing `PDFConfig.Store` (likely the latter, since the scheduler depends on the store anyway).
