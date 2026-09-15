# 09: Persist stable selection intent

**What to build:**

Add an optional, portable selection descriptor that records deliberate logical-product, applicable target, ABI/CPU-variant, and archive-member intent independently of versioned source filenames. Carry it through the complete managed-binary lifecycle: resolution and fetch, installation, update, ensure, config cloning and mutation, and portable export/import. Use it to preserve a prior deliberate selection without turning transient ranking details into an update constraint.

Keep source provenance separate from selector intent. Preserve remote logical name, source asset, archive-member/package identity, release lane, pinning, aliases, and existing system-package metadata according to their established meanings: source metadata describes where bytes came from, while the descriptor constrains the intended product and compatible build. Do not overload the descriptor with macOS application-bundle identity or local-only state.

Derive intent for old records only where the existing persisted facts unambiguously support it. Records without supported derived intent must retain compatible legacy behavior, while a stored target or variant that is no longer available fails explicitly rather than silently selecting a different build. Centralize ensure-time selection through the same resolver used by install and update so aliases, release lanes, pinned versions, archive members, and deliberate variant choices agree across lifecycle operations.

**Blocked by:** 08 / Resolve release products before packaging

**Status:** ready-for-agent

- [ ] An optional portable selection descriptor represents logical product, applicable target, ABI/CPU-variant constraints, and archive-member identity without depending on versioned source filenames.
- [ ] New selection intent is retained through fetch, install, update, ensure, config mutation/cloning, and portable export/import round trips.
- [ ] Provenance remains distinct from selector intent; existing remote name, source asset, package/member identity, release lane, pinning, aliases, and system-package metadata preserve their established semantics.
- [ ] Old records derive descriptor fields only from supported unambiguous facts and remain backward compatible when no derivation is possible.
- [ ] Missing stored variants or targets fail with an explicit stable selection outcome rather than falling back to a different compatible-looking build.
- [ ] Install, update, and ensure use a common persisted-intent resolver so lifecycle operations produce consistent product, target, variant, and member decisions.
- [ ] Existing interactive and non-interactive safety, compatibility and runnable-payload validation, integrity behavior, release lanes, aliases, pinning, AppBundle continuity, and system-package behavior remain unchanged.


- [ ] Add serialization and portable export/import regressions for absent, new, and derived selection descriptors, including backward-compatible old records.
- [ ] Add lifecycle regressions proving a deliberate product, target, ABI/CPU variant, and archive-member choice survives install, update, and ensure across changing versioned source filenames.
- [ ] Add regressions proving aliases, release lanes, and pinned versions remain intact while persisted selection intent is applied.
- [ ] Add regressions proving unavailable stored variants or targets fail explicitly and do not switch builds through ranking or fallback.
- [ ] Exercise common ensure-time resolution and command-level update behavior to confirm source provenance and selector intent remain distinct.
- [ ] Run the focused command, asset-processing, provider, and configuration test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because lifecycle persistence and update/ensure behavior cross shared configuration and filesystem state.
