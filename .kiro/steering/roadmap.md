# Roadmap

## Overview

Personal DeFi research monitor backend. The system ingests papers (and later, governance forum posts and RSS feeds) and surfaces thesis-angle candidates to the sole-user researcher. Specs are added incrementally, each one a thin vertical slice along the ingestion pipeline.

## Approach Decision

- **Chosen**: File-backed search defaults + extend `arxiv-fetcher` with optional date and keyword query parameters. The fetcher reads stored defaults when the request omits them and accepts per-request overrides.
- **Why**: User wants stored keywords so they don't have to retype, plus the ability to narrow by date and keyword. File-on-disk is the smallest mechanism that solves the pain — no schema, no migration, no API surface, hand-edited by the operator.
- **Rejected alternatives**:
  - **GORM/SQLite for keyword storage** — overkill for a single editable list; adds schema and migration cost with no payoff for one user.
  - **Env-only config** — fixed at startup; user explicitly wants stored, mutable defaults that persist across restarts but can be edited without redeploy.
  - **HTTP CRUD for stored keywords** — extra surface area for a single-user tool that is happy hand-editing JSON.
  - **Single combined spec for filters + storage** — muddies the `arxiv-fetcher` boundary (currently "fetch given parameters") with a separate concern (where defaults come from).

## Scope

- **In**:
  - `arxiv-fetcher` accepts optional `from`, `to`, and `keywords` query parameters on its fetch endpoint.
  - When the request omits any of those parameters, the fetcher reads them from a file-backed defaults source via a `SearchDefaults` port.
  - A new spec owns the file-backed defaults: a JSON/YAML file on disk with a single global keyword list and a default date window, hand-edited by the operator, exposed via a port that `arxiv-fetcher` consumes.
- **Out**:
  - HTTP endpoints to read or mutate the stored defaults (hand-edit only).
  - Named keyword profiles or per-user defaults (single global list).
  - Reuse of the same defaults mechanism by other ingestion adapters (RSS, governance) — defer until a second consumer actually exists.
  - Any change to `paper-persistence` or `document-extraction`.

## Constraints

- Stack is Go 1.25 + Gin + GORM/SQLite per [tech.md](./tech.md). New file storage must respect the dependency rule in [structure.md](./structure.md): port in `domain/`, implementation in `infrastructure/`.
- `arxiv-fetcher` is implementation-complete (`tasks-approved`, all tasks done). Extending it requires re-running `/kiro-spec-requirements arxiv-fetcher` to amend its requirements + tasks.
- `document-extraction` is implementation-complete; no interleaving concern remains.
- `arxiv-pdf-download` is implementation-complete (merged, PR #6) and already modified `application/arxiv`'s use case and `arxivctrl.FetchResponse` for the same seam this roadmap entry touches (`internal/http/controller/arxiv/responses.go`, `internal/application/arxiv/usecase.go`). Read those files before amending `arxiv-fetcher` — this entry's changes land on top of that PR, not on the pre-PR shape.

## Boundary Strategy

- **Why this split**: `arxiv-fetcher` currently owns "fetch from arXiv given inputs". Adding stored defaults is a different responsibility — it owns "where do default search inputs come from when the caller omits them". Keeping these separate means `arxiv-fetcher` does not need to know about files, and the defaults port can later be reused by other fetchers without dragging arXiv concerns along.
- **Shared seams to watch**:
  - `SearchDefaults` port shape — must be agnostic to arXiv-specific concepts (categories stay env-config; the port returns keywords + date window only).
  - File schema versioning — a missing or malformed file should fail closed at startup, not silently revert to "no keywords, no date filter".
  - Behavior when the file is edited at runtime: read-once-at-startup vs. read-on-each-fetch. Treat as a design decision in the new spec, not a roadmap-level call.

## Existing Spec Updates

- [ ] arxiv-fetcher — add optional `from`, `to`, `keywords` query parameters to the fetch endpoint; consume `SearchDefaults` port to fall back to stored values when parameters are omitted; document that categories remain env-only. Dependencies: arxiv-search-defaults

## Specs (dependency order)

- [ ] arxiv-search-defaults — file-backed `SearchDefaults` port providing a default keyword list and default date window, hand-edited via a JSON/YAML file on disk, no HTTP surface. Dependencies: none. Touches `application/arxiv`'s use case, which `arxiv-pdf-download` also modified — read that spec's boundary section before amending.

## Completed / In-flight (not part of this roadmap entry)

- arxiv-fetcher — tasks-approved, all tasks complete (manual fetch, env-only categories, no filters; this roadmap entry extends it)
- paper-persistence — tasks-approved, all tasks complete (catalogue + auto-persist on fetch)
- document-extraction — implementation complete
- llm-analyzer — implementation complete, but wired to `infrastructure/llm/stub` only. Real provider integration (Anthropic) was explicitly deferred as a follow-up spec and has no spec yet — by design, deferred until the rest of the pipeline is done.
- pdf-storage — tasks-approved, all tasks complete (local filesystem `pdf.Store`, consumed by `document-extraction` and `arxiv-pdf-download`)
- arxiv-pdf-download — implementation complete, merged PR #6 (2026-05-11). Downloads each fetched entry's PDF into `pdf.Store` fire-and-forget after `/api/arxiv/fetch` persists it, with SSE (`GET /api/arxiv/downloads/:job_id/stream`) and REST (`GET /api/arxiv/downloads/:job_id`) status. Reused `pdf.Store` unchanged; no persistence of `download_status` to the DB (in-memory job registry).
- user-auth — implementation complete, merged directly to `main` 2026-08-11 (no PR — pushed straight from a local worktree, unlike every other spec above). Replaces the static `X-API-Token` with email+password login (`POST /auth/login`) issuing a stateless 24h JWT (`Authorization: Bearer`); `GET /auth/session`, `POST /auth/change-password`; users provisioned via `task seed:user -- <email> <password>`. No roles, no refresh tokens, no logout endpoint. Local dev token caching via `task dev:login -- <email> <password>` (writes `.dev-token`).

## Known Gaps

- The product pipeline (fetch → dedupe → triage → extract → LLM → persist, per [product.md](./product.md)) is still not connected end-to-end even after `arxiv-pdf-download`. Fetch now auto-downloads PDFs, but `document-extraction` still requires the operator to hand-supply the resulting `pdf_path`, and `POST /analyses` still requires a hand-carried `extraction_id`. No spec exists yet for either handoff, or for a triage stage.
- RSS ingestion is called out as the v1 concrete ingestion source in `product.md` but has no spec; only `domain/source` + `feedutil` scaffolding exists.
- Real LLM provider integration is deliberately last in sequence, not forgotten — see `llm-analyzer` note above.
- **Process note**: local `main` was 36 commits behind `origin/main` as of 2026-08-10 (missing the entire `arxiv-pdf-download` merge) until this session force-synced it. Verify `git status` vs `origin/main` before trusting this file's "Completed" section in future sessions — this roadmap is hand-maintained and both this repo's remote and local copies have gone stale before.
