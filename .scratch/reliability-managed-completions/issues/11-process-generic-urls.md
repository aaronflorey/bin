# 11: Process generic URLs through the shared resolver

**What to build:**

Route generic URL downloads through the same bounded artifact-processing and resolution flow used for release assets. Hand the response to common processing after exactly one download so plain executable payloads and TAR/ZIP archives receive consistent inventory limits, archive-member resolution, integrity handling, and final runnable-payload validation.

Keep generic-provider transport behavior intact before that handoff: retain redirect and authentication policy, response-derived filename precedence, and explicit-version precedence. Carry the relevant fetch options, selection intent, and resolved version into the shared flow. Preserve the raw response filename as provenance and maintain checksum association with the downloaded artifact; do not add speculative checksum-sidecar probing for generic URLs.

**Blocked by:** 09 / Persist stable selection intent

**Status:** resolved

- [x] Generic URL responses are downloaded once and then processed by the shared bounded artifact inventory and resolver flow for both plain files and TAR/ZIP archives.
- [x] Generic URL processing applies the established archive-member selection, compatibility, explicit-selection, integrity, and final runnable-payload validation gates.
- [x] Existing redirect and authentication restrictions remain enforced, and response filename precedence remains unchanged.
- [x] Relevant fetch options, persisted or requested selection intent, and explicit version behavior are retained through the generic URL handoff.
- [x] The raw response filename remains available as source provenance, and checksum evidence stays associated with the exact downloaded artifact.
- [x] Generic URL handling does not probe guessed or speculative checksum sidecars.
- [x] Existing release-asset, direct-binary, archive, lifecycle, non-interactive safety, and system-package behavior remains unchanged.


- [x] Add local HTTP fixture regressions for plain executable and TAR/ZIP generic URL responses that exercise the shared resolver, member selection, and runnable-payload validation.
- [x] Add local HTTP regressions covering redirects, authentication, response filename precedence, explicit version precedence, and relevant fetch-option propagation.
- [x] Add regressions proving raw response filename provenance and checksum association survive common processing without speculative checksum requests.
- [x] Run the focused provider, asset-processing, and command test suites.
- [x] Run `mise run lint`.
- [x] Run `mise run test-race` because download processing crosses shared filesystem and lifecycle state.

## Comments

- Claimed for implementation on 2026-09-15.
- Slice 1: expose the existing bounded artifact download/processing path for an already-open response body, while preserving release-provider behavior.
- Slice 2: hand generic URL responses and fetch options into that shared path, preserving transport, filename, version, provenance, integrity, and selection semantics.
- Slice 3: close meaningful regression gaps, then run focused and repository-wide verification.
- Slice 1 completed: `assets.Filter.ProcessReader` now owns shared bounded stream processing, and `ProcessURL` delegates after its single release-asset GET. Focused asset tests and lint passed.
- Slice 2 completed: generic fetch now derives metadata from its download response, applies fetch/selection options, processes plain and archived payloads through the shared resolver, and preserves raw filename provenance without checksum-sidecar probing. Focused provider/asset tests and lint passed.
- Slice 3 completed: added filename-precedence coverage, reviewed acceptance gaps, and passed provider/asset/command tests, lint, race tests, and diff checks.
- Final verification correction: cleanup assertions now satisfy repository-wide errcheck; `mise run test` and `mise run verify` pass.
- Code review: Standards and Spec axes approved. The initial Spec review requested an authentication redirect regression; it was added and the re-review approved it.

## Answer

Generic URL fetches now derive metadata from their single download response and pass the response body into the same bounded artifact inventory and resolver used by release assets. Plain files and TAR/ZIP archives share compatibility, selection, integrity-scope, cleanup, and runnable-payload gates while retaining generic redirect/auth policy, filename and explicit-version precedence, source provenance, and no speculative checksum requests.
