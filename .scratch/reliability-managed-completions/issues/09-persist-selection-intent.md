# 09: Persist stable selection intent

**What to build:**

Add an optional, portable selection descriptor that records deliberate logical-product, applicable target, ABI/CPU-variant, and archive-member intent independently of versioned source filenames. Carry it through the complete managed-binary lifecycle: resolution and fetch, installation, update, ensure, config cloning and mutation, and portable export/import. Use it to preserve a prior deliberate selection without turning transient ranking details into an update constraint.

Keep source provenance separate from selector intent. Preserve remote logical name, source asset, archive-member/package identity, release lane, pinning, aliases, and existing system-package metadata according to their established meanings: source metadata describes where bytes came from, while the descriptor constrains the intended product and compatible build. Do not overload the descriptor with macOS application-bundle identity or local-only state.

Derive intent for old records only where the existing persisted facts unambiguously support it. Records without supported derived intent must retain compatible legacy behavior, while a stored target or variant that is no longer available fails explicitly rather than silently selecting a different build. Centralize ensure-time selection through the same resolver used by install and update so aliases, release lanes, pinned versions, archive members, and deliberate variant choices agree across lifecycle operations.

**Blocked by:** 08 / Resolve release products before packaging

**Status:** resolved

- [x] An optional portable selection descriptor represents logical product, applicable target, ABI/CPU-variant constraints, and archive-member identity without depending on versioned source filenames.
- [x] New selection intent is retained through fetch, install, update, ensure, config mutation/cloning, and portable export/import round trips.
- [x] Provenance remains distinct from selector intent; existing remote name, source asset, package/member identity, release lane, pinning, aliases, and system-package metadata preserve their established semantics.
- [x] Old records derive descriptor fields only from supported unambiguous facts and remain backward compatible when no derivation is possible.
- [x] Missing stored variants or targets fail with an explicit stable selection outcome rather than falling back to a different compatible-looking build.
- [x] Install, update, and ensure use a common persisted-intent resolver so lifecycle operations produce consistent product, target, variant, and member decisions.
- [x] Existing interactive and non-interactive safety, compatibility and runnable-payload validation, integrity behavior, release lanes, aliases, pinning, AppBundle continuity, and system-package behavior remain unchanged.


- [x] Add serialization and portable export/import regressions for absent, new, and derived selection descriptors, including backward-compatible old records.
- [x] Add lifecycle regressions proving a deliberate product, target, ABI/CPU variant, and archive-member choice survives install, update, and ensure across changing versioned source filenames.
- [x] Add regressions proving aliases, release lanes, and pinned versions remain intact while persisted selection intent is applied.
- [x] Add regressions proving unavailable stored variants or targets fail explicitly and do not switch builds through ranking or fallback.
- [x] Exercise common ensure-time resolution and command-level update behavior to confirm source provenance and selector intent remain distinct.
- [x] Run the focused command, asset-processing, provider, and configuration test suites.
- [x] Run `mise run lint`.
- [x] Run `mise run test-race` because lifecycle persistence and update/ensure behavior cross shared configuration and filesystem state.

## Comments

- 2026-09-15: Claimed for implementation. Planned slices: (1) portable config model and round trips, (2) shared asset/provider persisted-intent resolution, (3) install/update/ensure lifecycle integration and regressions, (4) final verification and review.
- 2026-09-15: Slice 1 complete: added the optional descriptor, deep cloning, and config/export/import round-trip coverage. Focused config and export/import tests plus lint passed.
- 2026-09-15: Slice 2 complete: added shared release/member persisted-intent resolution, stable unavailable-selection reasons, conservative legacy derivation, and provider transport of resolved intent. Focused asset/provider tests and lint passed; command lifecycle wiring remains.
- 2026-09-15: Slice 3 complete: wired persisted/derived intent through direct and system-package install, update, and ensure; blocked provider/package-path fallback for stored intent; added command lifecycle regressions. Focused suites, lint, and race tests passed.
- 2026-09-15: Initial two-axis review requested corrections: normalize versioned archive wrapper paths in persisted member intent, add derived/lifecycle regressions across version changes, and document the new config semantics.
- 2026-09-15: Review corrections complete. Versioned top-level archive wrappers are normalized without erasing meaningful internal identity; supported legacy records derive intent during lifecycle reuse and portable export; unsupported records remain descriptor-less.
- 2026-09-15: Final Standards and Spec re-reviews approved with no remaining findings. Full tests, verification, race tests, build, and diff checks passed.

## Answer

Implemented portable persisted selection intent across config, provider fetches, release/member resolution, install, update, ensure, system-package metadata, and export/import. Stored constraints now fail with typed stable reasons when unavailable, while conservative legacy derivation and narrow archive-wrapper normalization preserve compatible updates without conflating selector intent with source provenance.
