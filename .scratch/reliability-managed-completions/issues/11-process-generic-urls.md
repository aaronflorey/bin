# 11: Process generic URLs through the shared resolver

**What to build:**

Route generic URL downloads through the same bounded artifact-processing and resolution flow used for release assets. Hand the response to common processing after exactly one download so plain executable payloads and TAR/ZIP archives receive consistent inventory limits, archive-member resolution, integrity handling, and final runnable-payload validation.

Keep generic-provider transport behavior intact before that handoff: retain redirect and authentication policy, response-derived filename precedence, and explicit-version precedence. Carry the relevant fetch options, selection intent, and resolved version into the shared flow. Preserve the raw response filename as provenance and maintain checksum association with the downloaded artifact; do not add speculative checksum-sidecar probing for generic URLs.

**Blocked by:** 09 / Persist stable selection intent

**Status:** ready-for-agent

- [ ] Generic URL responses are downloaded once and then processed by the shared bounded artifact inventory and resolver flow for both plain files and TAR/ZIP archives.
- [ ] Generic URL processing applies the established archive-member selection, compatibility, explicit-selection, integrity, and final runnable-payload validation gates.
- [ ] Existing redirect and authentication restrictions remain enforced, and response filename precedence remains unchanged.
- [ ] Relevant fetch options, persisted or requested selection intent, and explicit version behavior are retained through the generic URL handoff.
- [ ] The raw response filename remains available as source provenance, and checksum evidence stays associated with the exact downloaded artifact.
- [ ] Generic URL handling does not probe guessed or speculative checksum sidecars.
- [ ] Existing release-asset, direct-binary, archive, lifecycle, non-interactive safety, and system-package behavior remains unchanged.


- [ ] Add local HTTP fixture regressions for plain executable and TAR/ZIP generic URL responses that exercise the shared resolver, member selection, and runnable-payload validation.
- [ ] Add local HTTP regressions covering redirects, authentication, response filename precedence, explicit version precedence, and relevant fetch-option propagation.
- [ ] Add regressions proving raw response filename provenance and checksum association survive common processing without speculative checksum requests.
- [ ] Run the focused provider, asset-processing, and command test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because download processing crosses shared filesystem and lifecycle state.
