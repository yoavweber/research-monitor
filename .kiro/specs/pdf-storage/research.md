# Gap Analysis: pdf-storage

Date: 2026-05-03
Source spec: `.kiro/specs/pdf-storage/requirements.md`

## 1. Current State

### What already exists in the codebase

| Asset | Location | Status |
|---|---|---|
| `shared.Fetcher` port (generic byte-level GET) | `internal/domain/shared/ports.go:53` | Defined |
| `shared.Fetcher` HTTP impl (`byteFetcher`, 15s timeout, custom UA) | `internal/infrastructure/httpclient/byte_fetcher.go:19-71` | **Reusable as-is** |
| `shared.Logger` port + `slog`-backed impl | `internal/domain/shared/ports.go:8-15`, `internal/infrastructure/observability/logger.go` | **Reusable as-is** |
| `shared.Clock` port | `internal/domain/shared/ports.go:18-20` | **Reusable as-is** |
| Viper-backed env struct with `mapstructure` tags + post-Unmarshal validation | `internal/bootstrap/env.go:12-33`, validation at `82-87`, `99-101`, `142-150` | Pattern to follow for `PDF_STORE_ROOT` |
| Composition root | `internal/bootstrap/app.go:72-76` (fetcher), `:95-113` (extraction) | Pattern to follow for wiring `pdf.Store` |
| Shared mock for `Logger` | `tests/mocks/logger.go` (`RecordingLogger`) | **Reusable** |
| Existing `Fetcher` test pattern (inline `fakeFetcher` + `httptest.Server`) | `internal/infrastructure/arxiv/fetcher_test.go:18-32`, `internal/infrastructure/httpclient/byte_fetcher_test.go` | Pattern to follow |
| Testing rules | `.kiro/steering/testing.md` (`t.Parallel()` everywhere, `t.Run` even single-case, AAA via blank lines, `tests/mocks/` for non-DB collaborators) | Hard rule |

### What does NOT exist

| Missing | Implication |
|---|---|
| Any `pdf` package (domain or infra) | Greenfield namespace — `internal/domain/pdf/` + `internal/infrastructure/pdf/local/` are free |
| Atomic-write helper (tmp+rename utility) | Must use `os.CreateTemp` + `os.Rename` directly inline; no shared util to lean on. The existing `mineru` adapter uses `os.MkdirTemp` + `os.RemoveAll` for *temporary* dirs — different concern, not reusable. |
| Shared `Fetcher` test double in `tests/mocks/` | None present today (callers use inline `fakeFetcher` per testing.md rule "hand-rolled fake when collaborator is an out-of-process system"). The new local-store tests should follow the same inline pattern, not invent a global fake. |
| Any production code that touches `Paper.PDFURL` | Confirms requirement boundary — the store does **not** depend on `domain/paper`; the caller maps `Paper` → `pdf.Key`. |

## 2. Requirement → Asset Map

| Req | Need | Existing asset | Gap |
|---|---|---|---|
| 1. Materialize on demand (fetch-on-miss / return-on-hit) | HTTP GET, file existence check, read file | `shared.Fetcher` impl, `os.Stat` (stdlib) | **Missing**: orchestrator that combines them; the `pdf.Store` itself |
| 1.3 Handle exposes both `Path()` and `Open()` | New abstraction | None | **Missing**: `pdf.Locator` interface + local impl |
| 1.4 Validation | Existing `Validate() error` pattern in `domain/<entity>/requests.go` | e.g. `domain/source/requests.go` | Pattern available; need new `pdf.Key.Validate()` |
| 1.5 No partial reads | `os.CreateTemp` + `os.Rename` (atomic on same FS) | Stdlib only | **Constraint**: must implement tmp+rename ourselves |
| 2.1 Idempotent / fetch at most once | File-existence gate before fetch | Stdlib `os.Stat` | **Missing**: gate logic in store |
| 2.3 Recover from prior partial | `*.tmp` will be overwritten on next `os.CreateTemp(dir, "<id>.*.tmp")` because we never rename a half file. Stale `*.tmp` siblings are harmless; document this. | Stdlib | **Constraint**: pattern only — no leftover-tmp cleanup needed for correctness, but worth a one-line `Glob` sweep for hygiene |
| 2.4 Empty-file = not stored | `FileInfo.Size() == 0` check | Stdlib | **Missing**: gate logic |
| 3.1 Fetch failure surfaces typed cause | `shared.Fetcher` already wraps `shared.ErrBadStatus` and surfaces stdlib transport errors | `shared.ErrBadStatus`, `*url.Error`, `context.DeadlineExceeded` | **Constraint**: store must NOT swallow these; just wrap with `pdf`-package context |
| 3.2 Storage failure distinguishable from fetch failure | Sentinels: `pdf.ErrFetch`, `pdf.ErrStore` (or single `ErrStorage` + `errors.Is`) | None | **Missing**: typed error sentinels |
| 3.3 Cancellation cleanup | `os.Remove` on `*.tmp` in defer | Stdlib | **Missing**: cleanup logic |
| 4. Handle abstraction | Interface in `domain/pdf/ports.go` | None | **Missing**: `pdf.Locator` |
| 5.1 Configurable root via env | `mapstructure:"PDF_STORE_ROOT"` field | `env.go` pattern at line 12-33 | **Missing**: new env field + default value |
| 5.2 Create root if missing | `os.MkdirAll` at startup | Stdlib | **Missing**: bootstrap-side `Init` call |
| 5.3 Fail-fast if root unwritable | Validate in `env.go` post-Unmarshal block (lines 82-101 pattern) **or** in store constructor | Stdlib | **Missing**: validation; design must pick where it lives |
| 5.4 Layout `<root>/<source_type>/<source_id>.pdf` | `filepath.Join`, `os.MkdirAll` per `source_type` | Stdlib | **Missing**: layout function |
| 6. Keep-forever | No deletion code | n/a | **Trivially satisfied**: just don't write deletion |
| 7.1-7.3 Structured logs | `shared.Logger.InfoContext` / `WarnContext` | Existing logger | **Missing**: log call sites |
| 7.4 No body in logs | Discipline | n/a | **Constraint**: code review item |

**Net gap**: one new domain package (`pdf`), one new infra package (`pdf/local`), one env field, one bootstrap wiring block. All composed from existing ports.

## 3. Implementation Approach Options

### Option A — Extend an existing package
Place `Store` / `Locator` / `Key` inside `domain/shared/` and the local impl beside `infrastructure/httpclient/`.
- ✅ Zero new directories.
- ❌ Violates structure.md §3 ("Every aggregate lives at `internal/domain/<entity>/`"). PDF storage is a distinct aggregate, not a cross-cutting port like `Logger`/`Clock`/`LLMClient`.
- ❌ Conflates "generic byte fetch" with "PDF-on-disk lifecycle". Damages the boundary the brief explicitly asked for.

**Verdict**: rejected.

### Option B — New domain + infrastructure packages (recommended)
- New `internal/domain/pdf/` with `ports.go` (`Store`, `Locator`, `Key`), `errors.go` (sentinels), `requests.go` (`Key.Validate()`).
- New `internal/infrastructure/pdf/local/` with `store.go` (the `localStore` type implementing `pdf.Store`) + `store_test.go` + `locator.go` (the `localLocator` type implementing `pdf.Locator`).
- Env field `PDFStoreRoot string \`mapstructure:"PDF_STORE_ROOT"\`` in `env.go`, default `data/pdfs`, validated post-Unmarshal.
- Bootstrap wiring in `app.go`: `pdfStore := pdflocal.NewStore(env.PDFStoreRoot, byteFetcher, logger)` next to the existing `byteFetcher` block.

- ✅ Matches structure.md §2-§4 exactly.
- ✅ Mirrors the `arxiv` adapter shape (small package, fetcher consumed via DI).
- ✅ Independent test surface; no risk to existing `extraction` / `arxiv` tests.
- ✅ Future S3 impl drops into `internal/infrastructure/pdf/s3/` with no caller changes.
- ❌ Two new directories — trivial cost.

**Verdict**: recommended.

### Option C — Hybrid
Define `Store`/`Locator` as cross-cutting ports in `domain/shared/ports.go` (alongside `Fetcher`, `LLMClient`), implementation in `infrastructure/pdf/local/`.
- ✅ Acknowledges that `Store` is consumed by multiple potential aggregates (extraction today, others later).
- ❌ structure.md treats `domain/shared` as the home of *cross-cutting* primitives (logging, clock, generic byte fetch, LLM). PDF storage carries domain-shaped concepts (`Key`, `Locator`, layout) that don't belong in `shared`.
- ❌ Future complexity: when retention policy or content-hash dedup arrives, they need a home — `shared` is the wrong place.

**Verdict**: rejected. The shape of PDF storage (typed key, typed errors, value-object handle) is aggregate-shaped, not cross-cutting.

## 4. Effort & Risk

- **Effort: S (1–3 days)**.
  - Domain package (~80 LOC): ports, errors, key validation.
  - Local impl (~150 LOC): existence gate, fetcher call, tmp+rename, locator type, log call sites.
  - Tests (~250 LOC): cache hit, cache miss, fetch error propagation, write error propagation, ctx cancellation cleanup, empty-file replacement, partial-tmp recovery, atomic visibility (read-during-write smoke test).
  - Env + bootstrap wiring (~30 LOC).
- **Risk: Low**.
  - All ports already exist and are in use elsewhere.
  - `os.CreateTemp` + `os.Rename` is well-trodden stdlib territory.
  - No schema, no migration, no HTTP surface, no concurrency primitives beyond standard FS atomicity.

## 5. Recommendations for Design Phase

### Preferred approach
**Option B**: new `domain/pdf/` + `infrastructure/pdf/local/` packages.

### Key decisions to lock in design
1. **Atomic-write recipe**: `os.CreateTemp(<root>/<source_type>, "<source_id>.*.pdf.tmp")` → write fetched bytes → `os.Rename(tmp, canonical)`. Document that `*.tmp` siblings are harmless leftover from crashed prior runs and never observed by readers.
2. **Where to validate the storage root**: bootstrap (during env load) **and** at store construction. Bootstrap-side validation ensures fail-fast per Req 5.3; constructor double-check protects unit-test wirings.
3. **`Locator` shape**: small interface (`Path() string`, `Open(ctx) (io.ReadCloser, error)`) rather than a struct. Concrete `localLocator` is private; constructor returns the interface. Future `s3Locator` materializes a temp file on first `Path()` call.
4. **Error sentinels in `domain/pdf/errors.go`**:
   - `ErrInvalidKey` (Req 1.4)
   - `ErrFetch` (Req 3.1) — wraps the underlying `shared.Fetcher` error so callers can `errors.Is(err, shared.ErrBadStatus)` / `errors.Is(err, context.Canceled)`.
   - `ErrStore` (Req 3.2) — for filesystem failures.
   - All exported sentinels documented as "wraps the underlying cause; use `errors.Is` to inspect".
5. **Logging contract**: three log lines — `pdf.store.fetched` (info, with `source_type`/`source_id`/byte count/duration), `pdf.store.cache_hit` (info), `pdf.store.failed` (warn, with category). No body bytes.
6. **Concurrency note**: v1 has no in-process lock. Two concurrent `Ensure` calls for the same key will both fetch and both rename — the rename loser silently overwrites, the consumer still sees a valid complete file, byte-for-byte deterministic for the same URL. Document this as acceptable for the single-user system; if it ever matters, add a per-key `singleflight` later.
7. **Test doubles strategy**: follow existing repo precedent — inline `fakeFetcher` in `store_test.go` (per `infrastructure/arxiv/fetcher_test.go:18-32`), `tests/mocks/logger.go` for the logger. Do **not** introduce a new `tests/mocks/fetcher.go` just for this spec.

### Research carried forward (for design phase)
None required. All open questions are design choices listed above, not external unknowns.
