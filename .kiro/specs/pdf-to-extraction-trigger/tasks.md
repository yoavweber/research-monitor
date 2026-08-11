# Implementation Plan

## 1. Foundation: extraction-submission contract

- [ ] 1. Define the extraction-submission contract used by the download worker
  - Add a narrow interface to the pdfdownload package describing exactly the ability to submit an extraction request, shaped so the existing extraction use case already satisfies it with no adapter
  - Add a small helper that turns a completed download's paper identity and resolved file path into an extraction request payload: the content-type discriminator is always the fixed "paper" value, the identifier reuses the paper's catalogue identifier with no version suffix, and the path is passed through unchanged
  - Observable completion: the package compiles with the new interface exported; a unit test confirms the payload helper maps a paper identity (including one carrying a version suffix) and a path into a payload whose identifier excludes the version and whose content type is always "paper"
  - _Requirements: 1.3, 3.1_
  - _Boundary: application/pdfdownload_

## 2. Core: worker trigger call and dependency wiring

- [ ] 2. Wire the download worker to submit extraction automatically on success

- [ ] 2.1 Accept the extraction-submission dependency in the download registry's construction
  - The registry's construction gains the new extraction-submission dependency as a required input, stored for later use by the per-entry worker
  - Every existing construction call site inside the pdfdownload package's own tests is updated to supply it, so the package and its existing tests keep compiling and passing unchanged otherwise
  - Observable completion: the package and its existing test suite build and pass with the new required dependency threaded through every construction call site
  - _Requirements: 1.1_
  - _Depends: 1_
  - _Boundary: application/pdfdownload_

- [ ] 2.2 Surface the downloaded file's resolved location from the per-entry download step
  - The per-entry download step additionally reports the exact on-disk location the download produced when it succeeds, and reports no location when it fails
  - No existing per-entry result field or wire shape changes — the additional location is available only to the caller orchestrating the download loop, not exposed on the download job's public status/event surface
  - Observable completion: a unit test asserts a successful download reports a non-empty resolved location and a failed download reports none
  - _Requirements: 1.2_
  - _Boundary: application/pdfdownload_

- [ ] 2.3 Trigger automatic extraction submission after a successful download
  - Immediately after a download entry succeeds (and after its existing status recording and logging), the worker submits an extraction request built from that entry's paper identity and resolved file location, using the same background execution context the rest of the download job already runs on
  - A failed download never submits an extraction request, and there is no way to opt a download out of the automatic submission
  - A successful submission is recorded in the structured logs; a rejected or failed submission is also recorded in the structured logs and is never retried, and neither outcome changes the download job's own recorded status or events
  - This behavior applies the same way regardless of what originally started the download job — no branching on job origin
  - Observable completion: a unit test using a stand-in for the extraction-submission dependency confirms it is called exactly once per successful entry with the expected payload, is never called for a failed entry, and that a simulated submission failure produces a log entry without altering the download job's recorded outcome
  - _Requirements: 1.1, 1.4, 1.5, 2.1, 2.2, 3.1, 4.1, 4.2_
  - _Depends: 2.1, 2.2_
  - _Boundary: application/pdfdownload_

## 3. Integration: production composition wiring

- [ ] 3. Wire the real extraction capability into application startup
  - Reorder application startup so the extraction capability is fully constructed before the download registry that now depends on it, then pass it through at construction time
  - No new configuration is introduced — the dependency is always supplied, matching the unconditional behavior established in task 2.3
  - Observable completion: the application starts successfully end-to-end, and a manual run of a download against a running instance produces an extraction record reachable through the existing extraction status lookup, without any manual extraction submission call
  - _Requirements: 1.1_
  - _Depends: 2.3_
  - _Boundary: bootstrap_

## 4. Integration: test harness composition wiring

- [ ] 4. Wire extraction submission into the integration test harness's own download registry
  - The integration test harness builds its own download registry independently of application startup, and today can build one without any extraction capability wired at all — this needs to keep working for every test that only exercises download behavior, while also being available for a test that exercises both together
  - Reorder the harness's own composition so the extraction capability, when the test opts into it, is available before the download registry that may now need it
  - When a test opts into download behavior but not extraction behavior, the harness supplies a recording stand-in instead, matching the harness's existing convention for other optional dependencies, so no currently-passing test changes behavior
  - Observable completion: every existing test that opts into download behavior continues to build and pass unchanged; a test that opts into both download and extraction behavior together compiles and runs for the first time
  - _Requirements: 1.1_
  - _Depends: 2.3_
  - _Boundary: tests/integration/setup_

## 5. Validation: end-to-end coverage

- [ ] 5. Verify the automatic hand-off across real package boundaries

- [ ] 5.1 Integration test: successful download produces an extraction without manual submission
  - Exercise the real download registry wired to the real extraction capability (opting into both in the same test, as enabled by task 4), schedule a download, and confirm an extraction record for that paper exists once the download completes — without the test itself ever submitting an extraction request
  - Observable completion: the integration test suite passes this new case under the project's integration test tag
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.1_
  - _Depends: 4_
  - _Boundary: tests/integration_

- [ ] 5.2 Integration test: re-downloading a paper re-triggers extraction on the same record
  - Schedule two successful downloads for the same paper identity (simulating a later version replacing an earlier one) and confirm the second automatic submission overwrites the same extraction record rather than producing a second one
  - Observable completion: the integration test suite passes this new case, asserting exactly one extraction record exists for the paper afterward
  - _Requirements: 3.1_
  - _Depends: 4_
  - _Boundary: tests/integration_
