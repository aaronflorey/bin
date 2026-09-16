# 12: Inspect artifact decisions without side effects

**What to build:**

Add `bin inspect <source> --json` as a read-only diagnostic surface for supported release and generic-URL artifact sources. Reuse the shared artifact resolver so inspection reports the same selected and rejected product, target, variant, archive-member, transformation, and integrity decisions that installation would make, without creating a separate ranking or validation path.

Define a stable JSON contract suitable for automation. Ambiguous or unresolved selections must report stable typed reasons and never prompt or choose by iteration order. Keep standard output limited to the requested JSON document; route diagnostics and errors through the established non-stdout channels.

Inspection startup and configuration handling must be non-mutating: it must not create or persist configuration, cache, installation, or other durable state. Allow only read-only provider work needed to resolve HTTP artifacts. Reject Docker, Go-install, system-package, completion, and any other effectful source or operation before version discovery, fetching, hooks, process execution, package-manager activity, or completion handling. Redact credentials, authorization material, and signed URL query data from all JSON and diagnostic output. Clean up every temporary bounded download and processing resource on success and failure.

**Blocked by:** 11 / Process generic URLs through the shared resolver

**Status:** resolved

- [x] `inspect <source> --json` uses the shared resolver and reports selected and rejected identity, transformations, and integrity status consistent with installation decisions.
- [x] The JSON output has a stable automation-oriented contract, including stable typed reasons for ambiguity and unresolved selection; inspection never prompts or silently chooses an ambiguous candidate.
- [x] Requested JSON is the only standard-output content; diagnostics and errors do not corrupt the document.
- [x] Inspection startup and configuration access do not create or mutate configuration, cache, install, or other persistent state.
- [x] Read-only release and generic-URL artifact resolution remains within provider transport policy, while Docker, Go-install, system-package, completion, and other effectful operations are rejected before discovery, fetch, hooks, process execution, or package-manager work.
- [x] JSON and diagnostics redact credentials, authorization data, and signed URL query material.
- [x] Temporary downloads, streams, and processing resources are bounded and cleaned on successful, rejected, and failed inspections.
- [x] Existing install, update, ensure, provider transport, resolver, payload-validation, integrity, lifecycle-hook, system-package, and completion behavior remains unchanged.


- [x] Add command-level regressions for selected and rejected JSON decisions, stable ambiguity reasons, no prompting, and clean standard output.
- [x] Add isolated regressions proving inspection does not create or mutate persistent state and does not run hooks, downloaded programs, package managers, or completion work.
- [x] Add provider-boundary regressions covering allowed read-only artifact sources, early effectful-source rejection, transport-policy preservation, redaction, and temporary-resource cleanup.
- [x] Run the focused command, provider, and artifact-processing test suites.
- [x] Run `mise run lint`.
- [x] Run `mise run build`.
- [x] Run `mise run test-race` because inspection performs bounded downloads and cleanup across shared filesystem state.

## Comments

- 2026-09-16: Claimed for implementation. Planned slices: command/read-only startup contract; shared resolver inspection/reporting and safety boundaries; focused verification and ticket closure.
- 2026-09-16: Slice 1 complete. Shared resolver/provider results now expose detached release-candidate, archive-member, transformation, and integrity evidence with typed ambiguity reasons; focused asset/provider regressions pass.
- 2026-09-16: Slice 2 complete. Added the inspect command, read-only startup bypass, early effectful-source rejection, JSON reporting/redaction, cleanup ownership, and command/provider-boundary regressions. Full tests, race tests, lint, verify, and build pass; final review slice remains.
- 2026-09-16: Slice 3 complete. Tightened the requested JSON-only contract, prevented inspect log-file creation, added structured provider integrity decisions and broader redaction/cleanup regressions, and reran full tests, race tests, lint, verify, and build successfully.
- 2026-09-16: Formal two-axis review completed. Standards findings were corrected and re-reviewed; Standards and Spec reviews both approved the final working tree.

## Answer

Implemented `bin inspect <source> --json` as a non-mutating diagnostic command over the shared provider and artifact resolver flow. The stable report includes selected and rejected release/archive evidence, transformations, processing and provider integrity decisions, and typed unresolved reasons. Inspection rejects effectful sources before provider work, preserves transport policy, redacts URL credentials and signed query data, avoids config/log/install state, never prompts, and closes owned temporary resources. Focused and full tests, race tests, lint, verify, and build all pass.
