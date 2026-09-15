# 06: Process artifacts through a bounded owned inventory

**What to build:**

Introduce a small owned artifact-processing result that carries executable metadata, transformation and integrity evidence, and a bounded inventory shared by release assets and later generic-URL processing. Classify every observed entry as executable, completion, or ignored. Keep ignored material out of staging while retaining enough evidence to make later resolution deterministic, and give one owner responsibility for cleanup and cancellation across all success and failure paths.

Establish an outer artifact-kind classification boundary before compatibility, name, packaging, or depth ranking. Metadata and data sidecars, including notarization JSON, must never become executable candidates. Define classification and decoding from one explicit supported-format set: support the intended plain, TAR, and ZIP forms, keep opaque executable payloads subject to final payload validation, and recognize Zstandard as explicitly unsupported until a decoder is intentionally added rather than silently treating it as another payload type.

Stream all artifact layers through finite configurable limits for download bytes, entry count, per-entry size, total expanded bytes, and archive nesting. Enforce limits even for inaccurate headers, nested archives, and ignored entries. Validate archive member identities as safe relative paths; reject traversal and duplicate normalized identities while preserving valid case-sensitive identities on platforms that support them. Use random staging locations and prevent duplicate or aliased inventory entries from selecting or overwriting material by iteration order.

Preserve established provider transport policy, checksum/integrity evidence, executable validation, system-package behavior, release lanes, aliases, pinning, interactive ambiguity handling, and non-interactive safety. Record and prevent the observed metadata-sidecar regression without weakening the final runnable-payload gate.

**Blocked by:** 05 / Commit executable and config state recoverably

**Status:** ready-for-agent

- [ ] Artifact processing returns an owned result containing executable metadata, transformation and integrity evidence, and an inventory with explicit executable, completion, and ignored entry classes.
- [ ] Release artifact processing uses the bounded owned inventory, with an integration seam suitable for generic URLs to use the same processing rules later.
- [ ] One clear owner closes streams, removes temporary material, and handles cancellation on every success and failure path; ignored entries are not staged and no early error leaks owned resources.
- [ ] Outer artifact-kind classification excludes metadata/data sidecars before compatibility, name, package-format, or archive-depth ranking; the logged notarization/metadata-sidecar regression cannot select a sidecar as an executable candidate.
- [ ] Classification and decoding share one explicit supported-format definition for plain payloads, TAR, and ZIP; recognized Zstandard input fails with a clear unsupported-format outcome until intentionally supported.
- [ ] Opaque executable downloads remain eligible only when final payload validation succeeds.
- [ ] Configurable finite streaming limits bound download bytes, entry count, per-entry bytes, total expanded bytes, and nesting depth, including when headers are missing, inaccurate, or malicious and when entries are ignored or nested.
- [ ] Unsafe relative member identities and duplicate normalized identities are rejected deterministically; valid distinct case-sensitive identities remain supported where the platform permits them.
- [ ] Random staging and inventory handling prevent duplicate, aliased, or iteration-order-dependent entries from overwriting, leaking, or silently changing the selected material.
- [ ] Existing transport security, checksum and installed-byte integrity semantics, final executable validation, system-package behavior, release lanes, aliases, pinning, and interactive/non-interactive selection safety remain intact.


- [ ] Add artifact-processing regressions for entry classification, ignored-entry non-staging, cleanup and cancellation ownership, and the logged metadata-sidecar selection failure.
- [ ] Add streaming-limit regressions for oversized downloads and entries, excessive entry counts, expanded bytes, nesting, inaccurate headers, ignored entries, and nested archives.
- [ ] Add format regressions for supported plain, TAR, and ZIP inputs, explicit unsupported Zstandard input, and opaque payloads that pass or fail final validation.
- [ ] Add deterministic member-identity regressions for traversal, duplicate normalized paths, case-sensitive distinct paths where supported, and duplicate/aliased staging protection.
- [ ] Run the focused command, asset-processing, provider, and configuration test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because this work changes streaming resource ownership, filesystem staging, cancellation, and shared artifact state.
