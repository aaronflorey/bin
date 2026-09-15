# 12: Inspect artifact decisions without side effects

**What to build:**

Add `bin inspect <source> --json` as a read-only diagnostic surface for supported release and generic-URL artifact sources. Reuse the shared artifact resolver so inspection reports the same selected and rejected product, target, variant, archive-member, transformation, and integrity decisions that installation would make, without creating a separate ranking or validation path.

Define a stable JSON contract suitable for automation. Ambiguous or unresolved selections must report stable typed reasons and never prompt or choose by iteration order. Keep standard output limited to the requested JSON document; route diagnostics and errors through the established non-stdout channels.

Inspection startup and configuration handling must be non-mutating: it must not create or persist configuration, cache, installation, or other durable state. Allow only read-only provider work needed to resolve HTTP artifacts. Reject Docker, Go-install, system-package, completion, and any other effectful source or operation before version discovery, fetching, hooks, process execution, package-manager activity, or completion handling. Redact credentials, authorization material, and signed URL query data from all JSON and diagnostic output. Clean up every temporary bounded download and processing resource on success and failure.

**Blocked by:** 11 / Process generic URLs through the shared resolver

**Status:** ready-for-agent

- [ ] `inspect <source> --json` uses the shared resolver and reports selected and rejected identity, transformations, and integrity status consistent with installation decisions.
- [ ] The JSON output has a stable automation-oriented contract, including stable typed reasons for ambiguity and unresolved selection; inspection never prompts or silently chooses an ambiguous candidate.
- [ ] Requested JSON is the only standard-output content; diagnostics and errors do not corrupt the document.
- [ ] Inspection startup and configuration access do not create or mutate configuration, cache, install, or other persistent state.
- [ ] Read-only release and generic-URL artifact resolution remains within provider transport policy, while Docker, Go-install, system-package, completion, and other effectful operations are rejected before discovery, fetch, hooks, process execution, or package-manager work.
- [ ] JSON and diagnostics redact credentials, authorization data, and signed URL query material.
- [ ] Temporary downloads, streams, and processing resources are bounded and cleaned on successful, rejected, and failed inspections.
- [ ] Existing install, update, ensure, provider transport, resolver, payload-validation, integrity, lifecycle-hook, system-package, and completion behavior remains unchanged.


- [ ] Add command-level regressions for selected and rejected JSON decisions, stable ambiguity reasons, no prompting, and clean standard output.
- [ ] Add isolated regressions proving inspection does not create or mutate persistent state and does not run hooks, downloaded programs, package managers, or completion work.
- [ ] Add provider-boundary regressions covering allowed read-only artifact sources, early effectful-source rejection, transport-policy preservation, redaction, and temporary-resource cleanup.
- [ ] Run the focused command, provider, and artifact-processing test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run build`.
- [ ] Run `mise run test-race` because inspection performs bounded downloads and cleanup across shared filesystem state.
