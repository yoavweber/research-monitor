# Requirements Document

## Project Description (Input)
The sole-user researcher runs `GET /api/arxiv/fetch`, which persists new papers and auto-downloads their PDFs (`arxiv-pdf-download`) — but the pipeline stops there today. To get an extraction, the operator must manually call `POST /api/extractions` with a hand-supplied `pdf_path`, even though the PDF's on-disk location is already known to the system the moment the download succeeds. This is exactly the kind of manual ID/path hand-carrying the pipeline was supposed to eliminate.

What should change: the moment a PDF download succeeds, extraction should be submitted automatically with the correct `pdf_path` (the same canonical path `pdf.Store` wrote to) and `source_type`/`source_id` derived from the paper's identity — no operator action, no hand-carried path. This applies unconditionally to every successful download, regardless of whether the download job was triggered by the automated arxiv-fetch flow or any other caller of the pdfdownload registry. Failed downloads never trigger extraction (silent stop, visible via existing status/SSE surfaces and structured logs).

See `.kiro/specs/pdf-to-extraction-trigger/brief.md` for full discovery context (approach, scope, boundary candidates, constraints).

## Introduction

`pdf-to-extraction-trigger` closes the gap between PDF download and text extraction in the research pipeline. Today, the download feature fetches and stores each paper's PDF automatically, but nothing observes that success and hands it to the extraction feature — the operator must still find the stored file and submit it by hand. This feature adds that missing handoff: extraction starts automatically the instant a download succeeds, using the identity and file location the download already established. It owns only the trigger — not the extraction feature's request/poll contract, and not the download feature's own fetch/store behavior.

## Boundary Context (Optional)

- **In scope**: Automatically submitting an extraction request the moment a PDF download completes successfully, for every successful download regardless of trigger source (the automated arXiv fetch flow or any other caller of the download feature), including re-downloads of a paper downloaded before.
- **Out of scope**: Any change to how extraction requests are processed, polled, or reported once submitted (the existing extraction feature's own contract). Any change to how PDFs are fetched, stored, or reported as downloaded (the existing download feature's own contract). Retrying a failed download or a failed extraction submission.
- **Adjacent expectations**: A manual operator can still submit an extraction request directly at any time; this feature does not replace or restrict that path. This feature expects the extraction feature to accept a request built from a file location, a source type, and a source identifier, and to behave the same whether the request arrives from an operator or from this automatic trigger.

## Requirements

### Requirement 1: Automatic Extraction Submission on Download Success
**Objective:** As the sole-user researcher, I want extraction to start automatically once a PDF download succeeds, so that I don't have to find the downloaded file and submit it by hand.

#### Acceptance Criteria
1. When a PDF download for a paper completes successfully, the PDF Download feature shall submit an extraction request for that paper without operator action.
2. The PDF Download feature shall submit the extraction request using the exact file location the successful download produced.
3. The PDF Download feature shall submit the extraction request using the source type and source identifier that already identify the paper in the system, without requiring the operator to supply them.
4. When a download job is started by the automated arXiv fetch flow, the PDF Download feature shall trigger extraction the same way as when a download job is started by any other caller of the download feature.
5. The PDF Download feature shall not provide a way to opt out of automatic extraction submission on a per-download basis.

### Requirement 2: No Extraction Triggered on Download Failure
**Objective:** As the sole-user researcher, I want a failed download to never produce a half-formed extraction, so that extraction results stay trustworthy.

#### Acceptance Criteria
1. If a PDF download for a paper fails, then the PDF Download feature shall not submit an extraction request for that paper.
2. If a PDF download for a paper fails, then the PDF Download feature shall rely on the existing download status and log surfaces to communicate the failure, without introducing a new failure-reporting surface for this behavior.

### Requirement 3: Re-download Re-triggers Extraction
**Objective:** As the sole-user researcher, I want a successful re-download of a paper to produce a fresh extraction, so that extraction output stays current with the latest downloaded file.

#### Acceptance Criteria
1. When a PDF for a paper that was already downloaded and extracted before is downloaded again successfully, the PDF Download feature shall submit a new extraction request the same way as for a first-time success.

### Requirement 4: Extraction Submission Failure Visibility
**Objective:** As the sole-user researcher, I want to know when the automatic hand-off itself fails, so that a silent gap in the pipeline doesn't go unnoticed.

#### Acceptance Criteria
1. If the extraction feature does not accept the automatically submitted request, then the PDF Download feature shall record the failure in the existing structured logs.
2. The PDF Download feature shall not automatically retry a failed automatic extraction submission.
