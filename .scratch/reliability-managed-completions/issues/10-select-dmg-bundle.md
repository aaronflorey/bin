# 10: Select DMG bundles by managed identity

**What to build:**

Make Apple disk-image application selection deterministic by resolving the managed application identity in this order: an explicitly requested identity, the persisted `AppBundle`, legacy logical-name matching, then a unique eligible bundle only when no identity is available. Reuse `AppBundle` as the persisted bundle identity; do not add or overload selection-intent fields with local application-bundle state.

Normalize the optional application suffix and compare bundle identities case-insensitively without allowing suffix variants or case collisions to select arbitrarily. Reject duplicate normalized identities, case-colliding bundles, ambiguous legacy matches, missing requested or stored identities, and embedded helper applications that are not eligible top-level installation candidates. Keep the mounted image boundary explicit so only valid, contained application bundles can participate in selection.

Preserve the selected application bundle and installed executable path through install, reinstall, update, ensure, aliases, portable export/import, removal, and pruning. Lifecycle resolution must retain the existing managed application intent instead of rediscovering a sibling bundle from a later image; removal must act on the actual persisted application/path pair.

Complete bundle containment and source-executable validation before any copy, signing, replacement, or configuration mutation. Selection or validation failures must leave the existing installation untouched and perform no pre-copy side effects. Preserve established direct-binary and system-package behavior, final runnable-payload validation, selection safety, release lanes, pinning, and aliases; do not expand replacement transaction semantics beyond the selection and pre-copy boundary.

**Blocked by:** 01 / Contain remote names and clean failed fetches; 09 / Persist stable selection intent

**Status:** resolved

- [x] DMG bundle resolution follows requested identity, persisted `AppBundle`, legacy logical matching, and unique eligible fallback in that order.
- [x] Application suffix normalization and case-insensitive matching are deterministic; duplicate normalized names, case collisions, ambiguous matches, and missing identities fail explicitly rather than depending on traversal order.
- [x] Embedded helper applications and bundles outside the eligible mounted-image boundary cannot be selected as install targets.
- [x] Persisted `AppBundle` is reused as the managed bundle identity without adding it to or overloading portable selection intent.
- [x] Install, reinstall, update, ensure, alias lookup, portable export/import, removal, and pruning preserve and use the actual selected bundle and installed executable path.
- [x] Bundle containment and source-executable validation finish before copy, signing, replacement, or configuration writes; failed selection or validation has no pre-copy side effects and preserves installed applications.
- [x] Fastpotify-style multi-application images select the managed application rather than a similarly named sibling, including across lifecycle operations.
- [x] Existing lifecycle, payload-validation, non-interactive safety, release-lane, pinning, alias, and system-package behavior remains unchanged.


- [x] Add resolver regressions for requested, stored, legacy, and unique-fallback bundle identity order; suffix/case normalization; collisions; ambiguity; missing identities; and embedded-helper rejection.
- [x] Add command-level Fastpotify regressions proving install, update, ensure, and removal retain the intended managed bundle rather than selecting or removing a sibling application.
- [x] Add persistence and portable export/import regressions for `AppBundle` and the installed executable path, including aliases and legacy records.
- [x] Add isolated process/filesystem regressions proving containment and source-executable failures occur before copy, signing, replacement, or configuration writes and preserve an existing installed application.
- [x] Run the DMG-focused command tests in an isolated Apple-platform adapter environment.
- [x] Run `mise run lint`.
- [x] Run `mise run test-race` because lifecycle persistence, filesystem state, and process invocation boundaries change.

## Progress

- Resolver slice complete: deterministic requested/stored/legacy/unique precedence, suffix/case normalization, collision and ambiguity errors, top-level eligibility, containment, and focused regressions are implemented. Focused command tests and lint pass.
- Lifecycle slice complete: stored `AppBundle` is threaded through install/update/ensure, the installed executable path is retained, and source containment/executable checks precede copy and signing. Focused and full `cmd` package tests plus lint pass.
- Persistence/regression slice complete: explicit bundle identity is separated from provider product selection; update, ensure, alias lookup, portable export/import, and removal regressions preserve the selected bundle/executable pair and protect siblings. Focused and full `cmd` package tests plus lint pass.
- Verification slice complete: focused DMG tests, race tests, full tests, verify, and build pass. DMG behavior was exercised through isolated filesystem/process adapters on Linux; no native Apple mount was available.
- Review corrections complete: persisted bundle metadata is constrained to a contained `.app` component and runnable-payload validation now occurs before copy/signing. Standards and spec re-reviews approved the change with no remaining blockers.

## Answer

DMG installation now resolves only contained top-level application bundles by explicit request, persisted `AppBundle`, legacy logical name, or unique fallback. It validates the selected source executable before side effects, persists the actual bundle/executable pair across lifecycle and portable config operations, and prevents removal from escaping or targeting a sibling bundle. Resolver, Fastpotify-style lifecycle, alias, legacy import/export, removal, pruning, and no-side-effect regressions cover the behavior.
