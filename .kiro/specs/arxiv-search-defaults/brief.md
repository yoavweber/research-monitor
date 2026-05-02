# Brief: arxiv-search-defaults

## Problem

The researcher running the personal DeFi research monitor — the sole user of the backend — wants to filter arXiv fetches by keywords and a date window without retyping the same keyword list on every request. Today the `arxiv-fetcher` endpoint accepts no filters and reads only categories from env config, so each fetch returns the full configured-category window with no keyword narrowing.

## Current State

- `arxiv-fetcher` (tasks-approved): manual HTTP trigger, categories from env, no per-request filters, no notion of "default keywords" or "default date window".
- No on-disk store for search-input defaults; the only configuration mechanism is env vars, which require an app restart to change and are not a natural place for a frequently edited keyword list.
- Stack uses GORM + SQLite for `paper-persistence`, but a SQLite-backed keyword store is rejected (see Approach) — this brief is for a file-backed store only.

## Desired Outcome

- A single JSON or YAML file on disk holds a default keyword list and a default date window.
- A `SearchDefaults` port in `domain/` exposes those values to consumers.
- A file-reader implementation in `infrastructure/` loads and validates the file.
- `arxiv-fetcher` consumes the port (separately scoped — see "Existing Spec Touchpoints"). When a request omits keywords or dates, the fetcher uses the stored defaults; when present, the request overrides.
- The operator changes defaults by editing the file in their editor — no API surface.

## Approach

- Define a `SearchDefaults` port in `domain/` returning the default keyword list (`[]string`) and default date window (e.g. `from`, `to` or `lookback duration`).
- Implement the port in `infrastructure/` as a file reader. File path comes from env config; format is JSON or YAML (decide in design phase).
- Validate the file at startup: malformed or missing file fails the app boot rather than silently degrading to "no defaults".
- File reads behavior — read-once-at-startup vs. read-on-each-call — is a design decision; default expectation is read-once for simplicity, but defer to design.
- No HTTP endpoints. No CRUD. No schema migration. Hand-edit only.

## Scope

- **In**:
  - `SearchDefaults` port definition.
  - File-reader implementation that loads keywords + default date window from a JSON/YAML file on disk.
  - Startup validation of the file (existence, parseability, sane shape).
  - Env config for the file path.
  - A documented example file committed under a non-secret path (e.g. `data/search-defaults.example.json`).
- **Out**:
  - HTTP endpoints for read or write of defaults.
  - Named profiles, per-user defaults, or multiple keyword sets in one file.
  - Hot-reload semantics beyond a documented behavior choice.
  - Persisting categories (categories remain env-only and are owned by `arxiv-fetcher`).
  - Any direct consumption by adapters other than `arxiv-fetcher`.

## Boundary Candidates

- Port lives in `domain/<package>/ports.go`; implementation in `infrastructure/`. Wiring in `bootstrap/`.
- File format and validation rules are owned by the file-reader implementation; the port stays format-agnostic.
- The shape of "date window" (absolute `from`/`to` vs relative `lookback`) is a design-phase decision and lives entirely inside this spec; consumers see only the resolved values.

## Out of Boundary

- Mutation of defaults via the API surface — explicitly hand-edit only.
- Storage of fetcher-side state (last-fetched timestamps, dedupe markers) — that belongs to `paper-persistence` or future specs.
- Category list — owned by `arxiv-fetcher` env config and not duplicated here.
- Reuse by RSS/governance ingestion — left for a future spec when a second consumer exists.

## Upstream / Downstream

- **Upstream**: env config (file path), the file on disk.
- **Downstream**: `arxiv-fetcher` consumes the port to fill in missing request parameters. No other current consumers.

## Existing Spec Touchpoints

- **Extends**: none (new spec).
- **Adjacent**: `arxiv-fetcher` — its requirements must be amended in a separate pass (`/kiro-spec-requirements arxiv-fetcher`) to add `from`, `to`, `keywords` query parameters and to depend on the `SearchDefaults` port. That update is tracked in `roadmap.md` under "Existing Spec Updates" and is not in scope here.

## Constraints

- Go 1.25, Gin, GORM/SQLite stack per [`.kiro/steering/tech.md`](../../steering/tech.md).
- Dependency rule per [`.kiro/steering/structure.md`](../../steering/structure.md): port in `domain/`, file I/O in `infrastructure/`, never the other way.
- `log/slog` only via the `domain/shared.Logger` port.
- Single user, single backend instance — no multi-tenant or concurrency-around-edits requirements.
