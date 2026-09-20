# 17: Refresh opted-in completions and clean up owned files

**Status:** ready-for-agent

**Blocked by:** 14 / Prefer exact bundled completion files.

## What to build

Connect the usable sync/bundled helpers from 13–14 at the existing direct-binary lifecycle seams. Follow the [spec](../spec.md); do not introduce another coordinator, policy layer, or cache.

Add `install --completions=<bash|zsh|fish|off>` and one optional machine-local shell choice per binary. New records default to off; omitted flags preserve an existing local choice. Explicit sync remains one-shot. Preserve the choice and ownership through ordinary updates/config cloning and exclude them from portable export/import without erasing destination-local values.

For an opted-in direct install or update, request the selected bundled script from the artifact already being fetched. After the existing executable/config commit succeeds, publish that script or try the native fallback. Update and actual ensure reinstalls already use `installBinary`; put the work there once. An unchanged binary does no completion work. Errors warn without changing binary success, hooks, or aggregate update behavior. System packages, run, inspect, and dry runs remain outside this path. A failed install discards any temporary bundled bytes without execution, publication, or completion-state writes.

After a direct binary has been removed, or prune has confirmed its absence, attempt completion cleanup before forgetting its captured ownership record. Under the existing lock, delete only recorded regular files with matching owner/path/hash and no conflicting current claim. Preserve everything else and warn with the leftover path. Binary removal proceeds even if optional completion cleanup fails; do not retain orphan records, retries, or recovery journals. Disabling automatic refresh leaves existing files until normal removal or manual cleanup.

## Acceptance

- [ ] Opt-in install publishes only after successful binary commit; omission preserves the old setting, `off` disables future refresh, and fresh/default installs perform no completion work.
- [ ] Update and actual ensure reinstall reuse the same hook and fetched artifact. Unchanged ensure/update, dry runs, run, inspect, and system-package operations request no managed completions. Failed installs never execute generators, publish scripts, or save completion state.
- [ ] Completion generation, publication, and metadata failures remain warnings after install/update; binary metadata and established exit/hook behavior remain correct.
- [ ] Explicit sync does not change automatic policy; config reload/update/import preserve local policy and ownership, and export excludes both.
- [ ] Remove/prune delete matching owned scripts, tolerate missing files, preserve modified/unmanaged/symlink/conflicting files, and report failed cleanup without blocking binary removal or adding persistent retry state.

## Verification

Extend the existing install/update/ensure/remove/prune tests and portable-config tests at their current injectable seams. Reuse the helper and filesystem cases from 13 instead of duplicating them for every command. Run the focused tests and the repository lint, race, test, verify, and build checks from the spec.
