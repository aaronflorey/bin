# 17: Integrate completion lifecycle and sync

**What to build:**

Integrate authorized managed Bash, Zsh, and Fish completion work into the binary lifecycle only after a binary installation or update has committed successfully. Use the installed binary hash and retained completion identity to distinguish changed binaries, which require refreshed discovery or generation, from unchanged opted-in binaries, which may repair missing or stale managed completion files without unnecessarily rerunning work. Completion integration remains best-effort: report actionable warnings and local repair state, but never reverse or fail a committed binary installation or alter the established aggregate update failure behavior.

Make `completions sync` perform one authorized repair using current binary, policy, shell, source, ownership, and state inputs without persisting opt-in. Report useful per-shell results, retain retryable repair failures, and avoid treating disabled, unsupported, stale, or off-policy records as successful. A disabled or stale record must not recreate files; changed identity, missing binary, invalid ownership, or altered content must revalidate before any completion mutation. Preserve user-modified, unmanaged, symlinked, package-manager-owned, and inactive-owner files throughout installation, repair, removal, and pruning.

Remove or prune completion state and files only when the recorded active owner and unmodified ownership evidence permit it. Keep retryable cleanup state when removal cannot complete, resolve shared destinations through the active owner rather than deleting another command's completion, and preserve the distinct lifecycle semantics of system-package installations. Completion work must have no side effects for `run`, inspection, dry runs, or failed binary installations, and must not change hooks, provider, artifact, executable validation, portable configuration, or Cobra's existing completion behavior.

**Blocked by:** 16 / Install completion files with explicit ownership

**Status:** ready-for-agent

- [ ] Authorized completion integration runs only after a successful binary commit; completion failures are actionable warnings or repairable outcomes and cannot undo or fail an otherwise successful install.
- [ ] Changed binary hashes refresh applicable completion evidence, while unchanged opted-in binaries repair eligible managed files without unnecessary discovery or generation.
- [ ] Sync evaluates current binary, shell, policy, source, ownership, and local-state inputs; it reports per-shell results, retries repairable outcomes, and does not persist opt-in.
- [ ] Disabled, off-policy, unsupported, stale, missing-binary, changed-identity, invalid-ownership, and altered-content states do not silently recreate or overwrite completion files.
- [ ] User-modified, unmanaged, symlinked, package-manager-owned, and non-active-owner files remain preserved during lifecycle integration, sync, removal, and pruning.
- [ ] Removal and pruning delete completion files and state only for the recorded unmodified active owner; incomplete cleanup remains retryable without deleting another command's shared completion.
- [ ] System-package completion ownership and cleanup retain their distinct install, update, remove, and prune semantics.
- [ ] `run`, inspection, dry runs, and failed binary installations perform no completion discovery, generation, publication, removal, or state mutation.
- [ ] Existing binary/update success and aggregate failure semantics, lifecycle hooks, provider, artifact, executable validation, portable configuration, and Cobra completion behavior remain unchanged.


- [ ] Add command and lifecycle regressions for post-commit ordering, changed-hash refresh, unchanged-hash repair, warning-only completion failures, and preserved install/update failure semantics.
- [ ] Add sync regressions for authorization, current input evaluation, per-shell results, transient retry behavior, stale/disabled/off-policy skips, and no persistent opt-in change.
- [ ] Add ownership and cleanup regressions for user modifications, unmanaged and protected entries, active-owner conflicts, removal/prune retry state, and system-package boundaries.
- [ ] Add no-side-effect regressions for `run`, inspection, dry runs, and failed installations.
- [ ] Run the focused completion, command, configuration, lifecycle, and system-package test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because completion lifecycle, ownership, cleanup retry state, and configuration mutation cross shared filesystem and locking boundaries.
- [ ] Run `mise run test`, `mise run verify`, and `mise run build`.
