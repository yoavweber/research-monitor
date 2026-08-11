# Research & Design Decisions

## Summary
- **Feature**: `pdf-to-extraction-trigger`
- **Discovery Scope**: Extension (existing system) — light discovery
- **Key Findings**:
  - `extraction.UseCase.Submit(ctx, RequestPayload)` already has exactly the signature this feature needs to call; no adapter/wrapper type is required, only a narrow consumer-defined interface that `extraction.UseCase` satisfies structurally.
  - `pdfdownload.Registry.runEntry` already computes the on-disk path via `pdf.Store.Ensure` but discards it after the byte-size stat; the path only needs to be returned one level up, not persisted anywhere new.
  - `document-extraction`'s own design.md fixes `source_type = "paper"` as a literal content-type constant unrelated to `paper.ID.Source` (which holds the provider, e.g. `"arxiv"`); `source_id` mirrors `paper-persistence`'s catalogue identity, which is `(Source, SourceID)` only — `Version` is excluded from the unique index (`idx_papers_source_source_id`), so re-downloading a newer version of the same paper correctly overwrites the same extraction row rather than creating a second one.

## Research Log

### Extension point analysis: where does the trigger call belong?
- **Context**: Brief proposed "a new injected port on `application/pdfdownload` (or a package it depends on)". Needed to confirm whether a new port type is actually necessary or whether an existing interface already fits.
- **Sources Consulted**: `internal/domain/extraction/ports.go`, `internal/application/pdfdownload/worker.go`, `internal/application/pdfdownload/service.go`, `internal/application/pdfdownload/types.go`.
- **Findings**:
  - `extraction.UseCase.Submit(ctx context.Context, payload RequestPayload) (SubmitResult, error)` is already the exact shape a caller needs to enqueue an extraction. It is a real DB write plus a non-blocking channel notify — fast, safe to call inline.
  - `pdfdownload.Registry.runJob` drives one entry at a time via `runEntry`, then `appendAndFanOut` (commit + SSE fan-out) and `emitEntryCompleted` (structured log). `runEntry` currently discards the `*pdf.Locator` from `store.Ensure` after using it for a best-effort byte-size stat.
  - `EntryResult` (the wire type serialized to SSE/REST) has no path field today. Adding one would widen an existing public payload shape for no requirement-driven reason.
- **Implications**: The design defines a single-method interface in `application/pdfdownload` (not a new domain package, not `domain/paper`) whose signature intentionally matches `extraction.UseCase.Submit`. Go's structural typing means `*extraction.UseCase`'s concrete implementation (built in bootstrap) satisfies it with zero glue code — no adapter struct. `runEntry`'s signature grows a second return value (the resolved path, empty on failure) that stays internal to the package; `EntryResult`'s wire shape is untouched, keeping the "no new SSE event" boundary intact by construction rather than by convention.

### Precedent check: how did `arxiv-pdf-download` wire its own trigger?
- **Context**: `arxiv-pdf-download` already added one cross-feature trigger (`arxivUseCase.Fetch` → `Registry.Schedule`) via `paper.PDFScheduler`, a narrow port defined in `domain/paper/ports.go`. Needed to decide whether to follow that exact placement or diverge.
- **Sources Consulted**: `.kiro/specs/arxiv-pdf-download/design.md` (Components, File Structure Plan, bootstrap wiring sections).
- **Findings**: `PDFScheduler` was placed in `domain/paper` because `paper.ID` is the entity both the caller (`application/arxiv`) and the implementer (`application/pdfdownload`) already shared — neutral ground both sides already imported.
- **Implications**: For this feature there is no equivalent shared domain entity between `application/pdfdownload` (caller) and `application/extraction` (implementer of the real behavior) — the only shared vocabulary is `extraction.RequestPayload`/`extraction.SubmitResult`, which already live in `domain/extraction`. `application/pdfdownload` already imports sibling domain packages directly (`domain/paper`, `domain/pdf`), so importing `domain/extraction` types into a locally-declared interface is consistent with existing precedent — no new domain subpackage is warranted for a single method.

### Identity mapping: what exactly goes into `RequestPayload.SourceType` / `SourceID`?
- **Context**: `paper.ID` carries `(Source, SourceID, Version)` and its own doc comment calls out that arXiv treats each version as "a distinct artifact." Extraction's `Submit` validates `SourceType == "paper"` exactly. A naive mapping (`SourceType = req.PaperID.Source`, i.e. `"arxiv"`) would always fail validation.
- **Sources Consulted**: `internal/domain/paper/id.go`, `internal/infrastructure/persistence/paper/model.go` (`uniqueIndex:idx_papers_source_source_id`, excludes `Version`), `.kiro/specs/document-extraction/design.md` (example payload `{"source_type": "paper", "source_id": "2404.12345", ...}`, and its own stated adjacent expectation that the pair "is the same identifier used by `paper-persistence`").
- **Findings**: `source_type` is a fixed content-type literal (`"paper"`) — a v1 constant of the extraction feature itself, independent of `paper.ID.Source`. `source_id` reuses `paper.ID.SourceID` alone (no version suffix), matching `paper-persistence`'s own dedupe identity, which already collapses versions of the same paper into one catalogue row.
- **Implications**: A re-download of a newer version of an already-extracted paper maps to the *same* `(source_type, source_id)` pair, so the extraction feature's existing overwrite-in-place `Upsert` behavior naturally satisfies Requirement 3 (re-download re-triggers extraction) without this feature needing any dedupe logic of its own.

## Architecture Pattern Evaluation

| Option | Description | Strengths | Risks / Limitations | Notes |
|--------|-------------|-----------|---------------------|-------|
| Narrow consumer-defined interface, structurally satisfied by `extraction.UseCase` | `application/pdfdownload` declares a one-method interface shaped exactly like `Submit`; bootstrap passes the concrete use case directly | Zero adapter code, minimal surface, interface-segregated (worker only sees `Submit`, not `Get`/`Process`) | None identified for this scope | Selected |
| Wrapper adapter struct implementing a bespoke `ExtractionTrigger` port | A new concrete type in `infrastructure/` wraps `extraction.UseCase` and translates to/from a pdfdownload-specific request shape | Would allow the trigger request shape to diverge from `extraction.RequestPayload` if it ever needed to | Pure indirection today — `RequestPayload` is already the right shape; the wrapper would forward every field unchanged | Rejected — no divergence exists to justify the extra layer (Simplification) |
| Async dispatch (goroutine / queue) for the trigger call | Mirrors the sibling `extraction-to-analysis-trigger` spec's async approach | Would fully decouple the download worker from extraction's write latency | `Submit` is a fast DB write + non-blocking notify, not a slow round-trip (no LLM call involved); async here would add goroutine-lifecycle and error-visibility complexity with no latency problem to solve | Rejected — this leg's downstream call is fast, unlike the sibling spec's `Analyze` call |

## Design Decisions

### Decision: Single-method consumer interface instead of a wrapper adapter
- **Context**: Brief suggested a "new injected port... invoked synchronously" without settling whether it needs its own adapter type.
- **Alternatives Considered**:
  1. Wrapper struct in `infrastructure/` implementing a bespoke port, translating to `extraction.RequestPayload` internally.
  2. A one-method interface in `application/pdfdownload` shaped identically to `extraction.UseCase.Submit`, satisfied structurally by the concrete use case with no wrapper.
- **Selected Approach**: Option 2.
- **Rationale**: `extraction.RequestPayload` is already the correct, stable shape for the request; there is nothing to translate. Go's structural typing makes the wrapper pure ceremony. Bootstrap wiring becomes a one-line pass of the already-constructed `extraction.UseCase` value.
- **Trade-offs**: If a future caller ever needed a materially different request shape, this interface would need to grow a translation step then — acceptable, since YAGNI: no such caller exists today.
- **Follow-up**: None.

### Decision: `SourceID` excludes version; `SourceType` is a fixed literal
- **Context**: See Research Log entry above.
- **Alternatives Considered**:
  1. `SourceID = paper.SourceID + paper.Version` (mirrors `pdfdownload.PDFArtifactKey`, used for the PDF cache key).
  2. `SourceID = paper.SourceID` alone (mirrors `paper-persistence`'s catalogue identity).
- **Selected Approach**: Option 2.
- **Rationale**: Matches `document-extraction`'s own stated adjacent expectation and its worked example; keeps one extraction row per catalogue paper, consistent with how `paper-persistence` already treats versions (latest overwrites, not appends).
- **Trade-offs**: If a v1 and v2 PDF are downloaded in quick succession, the second download's auto-triggered extraction overwrites the first's in-flight or completed row — this is the same behavior Requirement 3 explicitly asks for, not a side effect to guard against.
- **Follow-up**: None — this is existing `Upsert` behavior, unmodified by this feature.

### Decision: Integration test harness gets a real-or-fake `ExtractionSubmitter`, not a hard co-requirement between `WirePDFDownload` and `Extractor`
- **Context**: The task-graph sanity review (run before finalizing `tasks.md`) found that `tests/integration/setup/setup.go` constructs its own `pdfdownload.Registry` (line ~315) entirely independently of `bootstrap.NewApp`, and every existing `WirePDFDownload: true` test leaves `Extractor` unset. The original design draft only accounted for `bootstrap/app.go`'s construction site and stated (incorrectly) that bootstrap was the only place `extraction.UseCase` reaches `NewRegistry`. This was a real gap, not a task-plan omission — it required a design decision on how the harness should behave, not just an extra task.
- **Alternatives Considered**:
  1. Force `WirePDFDownload` and `Extractor` to be co-required (fail the test setup if one is set without the other) — would require editing all three existing pdfdownload integration test files even though their trigger behavior isn't under test.
  2. Supply a real `extractionUseCase` when `Extractor` is set, else a hand-written fake — mirrors the exact pattern already used one branch above in the same file for the arxiv scheduler port when `WirePDFDownload` is false.
- **Selected Approach**: Option 2.
- **Rationale**: Zero changes to any existing test file; the fallback fake is `mocks.ExtractionUseCaseFake{}`'s zero value, which already structurally satisfies `ExtractionSubmitter` — no new mock type. Consistent with this repo's established harness convention of substituting a recording fake for an opted-out dependency rather than forcing every test to opt into every feature.
- **Trade-offs**: A `WirePDFDownload`-only test now silently records (and discards) an extraction-submission attempt it never asserts on — acceptable, since that's exactly the "not testing this concern" behavior the equivalent existing fake (`mocks.NewPDFDownloadScheduler()`) already exhibits for the sibling port.
- **Follow-up**: None — captured as an explicit task in `tasks.md`; no further design work deferred.

## Risks & Mitigations
- **Bootstrap construction order**: `internal/bootstrap/app.go` currently constructs `pdfDownloadRegistry` (line ~101) before `extractionUseCase` (line ~131). Mitigation: reorder so extraction composition (`extractionRepo` → `mineruExtractor`/`extractionNotifier` → `extractionUseCase`) happens before `pdfdownload.NewRegistry` is called, since none of those extraction components depend on the registry.
- **Constructor signature break**: Adding a required parameter to `pdfdownload.NewRegistry` breaks every existing call site. Mitigation: this is a mechanical, compile-time-caught change — but "every call site" turned out to include a second composition root (`tests/integration/setup/setup.go`, not just `bootstrap/app.go` and `service_test.go`), found only by explicitly grepping for `NewRegistry(` during task-graph sanity review rather than assuming the design's own File Structure Plan was exhaustive. **Lesson for the next spec's design/task-sanity pass**: grep every construction call site of a modified constructor before finalizing the File Structure Plan, not just the ones the brief/design draft already had in mind (the `user-auth` spec hit this same class of gap with `app_test.go` and `tests/manual/*.go`).
- **Trigger failure silently drops a paper from the pipeline**: If `Submit` errors (e.g., a future non-`"paper"` `source_type`, or a DB error), Requirement 4 requires only a log line, no retry — this is an intentional accepted risk already scoped by the brief and requirements, not a gap in this design.

## References
- [document-extraction design.md](../document-extraction/design.md) — `source_type`/`source_id` example payload and adjacent-expectation statement.
- [arxiv-pdf-download design.md](../arxiv-pdf-download/design.md) — precedent for a consumer-defined trigger port (`paper.PDFScheduler`) and its bootstrap wiring pattern.
