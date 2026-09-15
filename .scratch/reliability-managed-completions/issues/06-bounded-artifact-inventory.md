# 06: Process artifacts through a bounded owned inventory

**What to build:**

Introduce a small owned artifact-processing result that carries executable metadata, transformation and integrity evidence, and a bounded inventory shared by release assets and later generic-URL processing. Classify every observed entry as executable, completion, or ignored. Keep ignored material out of staging while retaining enough evidence to make later resolution deterministic, and give one owner responsibility for cleanup and cancellation across all success and failure paths.

Establish an outer artifact-kind classification boundary before compatibility, name, packaging, or depth ranking. Metadata and data sidecars, including notarization JSON, must never become executable candidates. Define classification and decoding from one explicit supported-format set: support the intended plain, TAR, and ZIP forms, keep opaque executable payloads subject to final payload validation, and recognize Zstandard as explicitly unsupported until a decoder is intentionally added rather than silently treating it as another payload type.

Stream all artifact layers through finite configurable limits for download bytes, entry count, per-entry size, total expanded bytes, and archive nesting. Enforce limits even for inaccurate headers, nested archives, and ignored entries. Validate archive member identities as safe relative paths; reject traversal and duplicate normalized identities while preserving valid case-sensitive identities on platforms that support them. Use random staging locations and prevent duplicate or aliased inventory entries from selecting or overwriting material by iteration order.

Preserve established provider transport policy, checksum/integrity evidence, executable validation, system-package behavior, release lanes, aliases, pinning, interactive ambiguity handling, and non-interactive safety. Record and prevent the observed metadata-sidecar regression without weakening the final runnable-payload gate.

**Blocked by:** 05 / Commit executable and config state recoverably

**Status:** resolved

## Implementation progress

- [x] Slice 1: define the owned inventory, supported-format boundary, finite budgets, safe member identities, and focused unit regressions.
- [x] Slice 2: route release download/plain/TAR/ZIP processing through the owned bounded inventory, including classification, staging, nesting, and cleanup.
- [x] Slice 3: preserve release selection and integrity behavior while adding sidecar, opaque payload, limit, cancellation, and deterministic staging regressions.
- [x] Slice 4: run focused and repository-wide verification and resolve review findings.

### Slice 1 result

Added the package-local owned result and executable/completion/ignored inventory model, a shared plain/TAR/ZIP format boundary with explicit unsupported Zstandard recognition, finite budget primitives, safe normalized relative identities, deterministic alias/duplicate rejection, and idempotent cleanup ownership. Verification: `go test ./pkg/assets`, focused artifact tests, `mise run lint`, and `git diff --check` passed.

### Slice 2 result

Release downloads now enforce a streaming byte ceiling and pass through a shared owned processor that inventories and randomly stages plain/TAR/ZIP and existing compressed forms, accounts for ignored bytes, rejects duplicate identities and unsupported Zstandard, carries integrity/transformation evidence, and cleans the complete result through the returned reader. Provider integrity failures now close that owner. Verification: focused asset/provider tests, `mise run lint`, `mise run test-race`, and `git diff --check` passed.

### Slice 3 result

Completed the release regressions and edge handling: all observed archive identities count and validate (including directories/non-regular entries), nested identities are scoped deterministically, completion entries remain bounded/staged but never executable candidates, ignored entries are bounded without staging, opaque payloads use the runnable gate, metadata JSON is excluded before ranking, interrupted downloads and provider integrity failures clean ownership, and system-package handling remains separate. Verification: focused command/asset/provider/config tests, asset/provider race tests, `mise run lint`, and `git diff --check` passed.

### Slice 4 result

Reviewed every acceptance boundary, made compressed formats explicit in the shared definition, tightened member identity handling and byte-first format detection, and completed repository-wide checks. Verification: `mise run test`, `mise run verify`, `mise run build`, `mise run test-race`, and `git diff --check` passed. Live providers, privileged installs, and unavailable macOS/Windows native acceptance were not run.

The first review found two blocking budget/format gaps. Plain staged bytes now count against the expanded-byte budget, and opaque bytes with misleading TAR/ZIP names follow byte detection and the final runnable gate; focused pass/fail regressions were added. Post-correction verification: focused asset/provider tests, `mise run test-race`, `mise run lint`, and `git diff --check` passed.

The final two-axis review then identified cleanup-error reporting, single-payload per-entry accounting, and compressed metadata-sidecar gaps. Cleanup now reports both owned removals, plain/compressed payload layers enforce per-entry and expanded budgets without double charging decoded archive members, and compressed metadata remains non-runnable before ranking and processing. Focused command/asset/provider/config tests, race tests, lint, and diff checks passed after correction.

The approval recheck found nil-option and ignored-compressed-layer bypasses. Package-path matching is now nil-safe, and ignored compressed/nested members are decoded and budgeted without staging or selection; regressions cover direct nil-option processing plus ignored expansion and nesting. Focused suites, race tests, lint, and diff checks passed.

A subsequent spec recheck caught lost standalone gzip member naming. Valid embedded gzip names are again preserved through recursive processing, unsafe embedded names are rejected, and focused provider/asset plus race/lint/diff checks passed.

The last classification review found that completion containers could expose executable descendants and gzip metadata could be staged before embedded-name classification. Completion containers now remain completion-only, gzip embedded identities are validated/classified before staging, and ignored nested gzip identities are validated while streaming without staging. Focused, race, lint, and diff checks passed.

The final format review identified transparent concatenated gzip members as an identity/accounting bypass. Gzip multistream merging is now disabled; subsequent members are bounded and identity-validated, with concatenated or aliased members rejected rather than merged into one selected payload. Top-level and ignored-nested regressions pass with focused, race, lint, and diff checks.

- [x] Artifact processing returns an owned result containing executable metadata, transformation and integrity evidence, and an inventory with explicit executable, completion, and ignored entry classes.
- [x] Release artifact processing uses the bounded owned inventory, with an integration seam suitable for generic URLs to use the same processing rules later.
- [x] One clear owner closes streams, removes temporary material, and handles cancellation on every success and failure path; ignored entries are not staged and no early error leaks owned resources.
- [x] Outer artifact-kind classification excludes metadata/data sidecars before compatibility, name, package-format, or archive-depth ranking; the logged notarization/metadata-sidecar regression cannot select a sidecar as an executable candidate.
- [x] Classification and decoding share one explicit supported-format definition for plain payloads, TAR, and ZIP; recognized Zstandard input fails with a clear unsupported-format outcome until intentionally supported.
- [x] Opaque executable downloads remain eligible only when final payload validation succeeds.
- [x] Configurable finite streaming limits bound download bytes, entry count, per-entry bytes, total expanded bytes, and nesting depth, including when headers are missing, inaccurate, or malicious and when entries are ignored or nested.
- [x] Unsafe relative member identities and duplicate normalized identities are rejected deterministically; valid distinct case-sensitive identities remain supported where the platform permits them.
- [x] Random staging and inventory handling prevent duplicate, aliased, or iteration-order-dependent entries from overwriting, leaking, or silently changing the selected material.
- [x] Existing transport security, checksum and installed-byte integrity semantics, final executable validation, system-package behavior, release lanes, aliases, pinning, and interactive/non-interactive selection safety remain intact.


- [x] Add artifact-processing regressions for entry classification, ignored-entry non-staging, cleanup and cancellation ownership, and the logged metadata-sidecar selection failure.
- [x] Add streaming-limit regressions for oversized downloads and entries, excessive entry counts, expanded bytes, nesting, inaccurate headers, ignored entries, and nested archives.
- [x] Add format regressions for supported plain, TAR, and ZIP inputs, explicit unsupported Zstandard input, and opaque payloads that pass or fail final validation.
- [x] Add deterministic member-identity regressions for traversal, duplicate normalized paths, case-sensitive distinct paths where supported, and duplicate/aliased staging protection.
- [x] Run the focused command, asset-processing, provider, and configuration test suites.
- [x] Run `mise run lint`.
- [x] Run `mise run test-race` because this work changes streaming resource ownership, filesystem staging, cancellation, and shared artifact state.

## Answer

Release artifacts now flow through one owned bounded inventory that retains executable and completion evidence, records ignored identities without staging their content, carries transformation and scoped integrity metadata, and owns all stream and temporary-file cleanup. Format detection is byte-first for plain/TAR/ZIP and existing compressed forms, Zstandard is explicitly unsupported, opaque bytes still require runnable-payload validation, and all layers enforce finite download, entry, expansion, and nesting limits. Safe normalized identities, random staging, scoped nested members, and deterministic duplicate rejection prevent traversal and overwrite-by-order behavior.

Repository verification passed with `mise run test`, `mise run verify`, `mise run build`, and `mise run test-race`. Final `code-review` Standards and Spec axes both approved the staged diff after corrections. Live providers, privileged package installs, and unavailable macOS/Windows native acceptance were not run.
