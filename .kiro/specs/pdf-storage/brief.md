# Brief: pdf-storage

## Problem

The system fetches paper metadata (including `Paper.PDFURL`) via `arxiv-fetcher` and persists it via `paper-persistence`, but no component owns "given a paper, materialize its PDF bytes somewhere a downstream consumer can read". `document-extraction` requires a `PDFPath` on local disk for `mineru -p <path>`, and that path is currently supplied directly by the HTTP caller — there is no production path from `PDFURL` to `PDFPath`.

## Current State

- `domain/paper.Paper.PDFURL` carries the upstream URL.
- `internal/domain/shared.Fetcher` exists as a generic byte-level GET port, but no caller invokes it for PDFs.
- `domain/extraction.ExtractInput.PDFPath` and `infrastructure/extraction/mineru/adapter.go` both assume the file already exists locally.
- No code writes PDF bytes to disk; no directory under `data/` is reserved for PDFs.

## Desired Outcome

- A single port owns "ensure the PDF for a given paper exists locally and hand back a handle to it".
- The port is idempotent: if the PDF was already materialized, it returns the existing handle without re-downloading.
- v1 stores PDFs on the local filesystem under `data/pdfs/`.
- The port shape is forgiving to a future S3/NFS implementation — consumers do not assume "always a real local path".
- `mineru` (and any future on-disk PDF tool) can keep operating on a real path today.

## Approach

Define `domain/pdf.Store` with a single primary method:

```go
type Store interface {
    Ensure(ctx context.Context, key Key) (Locator, error)
}

type Key struct { SourceType, SourceID, URL string } // SourceType+SourceID is the stable identity; URL is the fetch source
type Locator interface {
    Path() string                  // local filesystem path to the PDF; materialized lazily for non-local impls
    Open(ctx context.Context) (io.ReadCloser, error)
}
```

v1 implementation `infrastructure/pdf/local`:
- Uses `shared.Fetcher` to GET the URL (no direct `net/http` in the implementation; injection-friendly per CLAUDE.md).
- Path layout: `<root>/<source_type>/<source_id>.pdf` (e.g. `data/pdfs/paper/2404.12345v1.pdf`).
- `Ensure` is content-agnostic: if the file exists and is non-empty, return its `Locator` immediately; otherwise fetch via `Fetcher`, write to a `*.tmp` sibling, `rename` into place (atomic on the same filesystem), return the `Locator`.
- `Locator.Path()` returns the on-disk path; `Locator.Open()` opens that path. A future S3 impl returns a `Locator` whose `Path()` materializes a temp file on first call — same call sites, no rewrite.
- Retention: keep forever in v1. No background sweeper. Re-extraction reuses the on-disk file.

## Scope

- **In**:
  - New domain package `internal/domain/pdf/` with `ports.go` (`Store`, `Key`, `Locator`) and a small typed-error file.
  - New infrastructure package `internal/infrastructure/pdf/local/` implementing `Store` over the local filesystem, depending on `shared.Fetcher` and `shared.Logger`.
  - Wiring in `bootstrap` to construct the store with a configured root directory (`PDF_STORE_ROOT`, default `data/pdfs`).
  - Unit tests for the local implementation: cache hit (file already present), cache miss (fetch + atomic rename), fetcher error surfaces, partial-write recovery (tmp file left behind from a prior crash is overwritten cleanly).
- **Out**:
  - Any change to `document-extraction` request shape, controllers, or worker. Integration of `pdf-storage` into the extraction flow is a follow-on update to that spec.
  - Any change to `arxiv-fetcher` or `paper-persistence`.
  - HTTP surface for the PDF store (no `GET /pdfs/...`). It is a domain port, not a public endpoint.
  - Background cleanup, TTL, retention, deduplication by content hash.
  - S3 / object-storage / NFS implementations.
  - Streaming extraction (we do not need `io.Reader`-only consumers in v1; the port allows them later via `Open`).

## Boundary Candidates

- "Where do PDF bytes live, and how do we get them on demand" — owned by `pdf.Store`.
- "How does HTTP fetching work" — already owned by `shared.Fetcher`. `pdf.Store` consumes it; it does not re-implement it.
- "What do tools that need a real on-disk PDF receive" — the `Locator` boundary. Consumers ask for a `Path()` only when they truly need one.

## Out of Boundary

- Mapping `Paper` → `pdf.Key`. That is the caller's job (likely the extraction worker or a small orchestration use case in a future spec). The store does not import `domain/paper`.
- Deciding when to evict / re-download. v1 is "if it exists on disk, it is good".
- Authorization / quota / rate limiting.

## Upstream / Downstream

- **Upstream**: `shared.Fetcher`, `shared.Logger`, `shared.Clock` (only if needed for log timing — likely not).
- **Downstream**: a future update to `document-extraction` will replace caller-supplied `pdf_path` with `pdf_url` (or `source_type`+`source_id`) and call `pdf.Store.Ensure` from the worker before invoking the `Extractor`. That work is explicitly NOT part of this spec.

## Existing Spec Touchpoints

- **Extends**: none.
- **Adjacent**:
  - `document-extraction` (in implementation) — must not be modified by this spec; the integration is a follow-on.
  - `arxiv-fetcher` — supplies `PDFURL` on `Paper`; no changes here.

## Constraints

- Go 1.25, dependency rule per `.kiro/steering/structure.md`: port in `domain/pdf/`, implementation in `infrastructure/pdf/local/`, wiring in `bootstrap/`.
- No direct `net/http` or filesystem calls from domain code; all I/O goes through ports.
- Tests follow `.kiro/steering/testing.md`: real filesystem (temp dir via `t.TempDir()`) for the local implementation, `Fetcher` faked via a test double in `tests/mocks/`.
- `log/slog` only via `shared.Logger`; `context.Context` is the first argument of every method.
- Atomic write semantics: write to `*.tmp` then `rename`; never expose a partially-written file under the canonical path.
