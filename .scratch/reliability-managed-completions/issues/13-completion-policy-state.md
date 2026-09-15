# 13: Define managed-completion policy and local state

**What to build:**

Define the disabled-by-default completion policy and machine-local state model for managed Bash, Zsh, and Fish completions. Keep persistent consent distinct from a per-invocation request: an explicit invocation choice takes precedence over configured policy, configured policy takes precedence over the default, and absence of both remains disabled. This policy must prevent discovery, generation, installation, or repair work unless it is authorized.

Resolve a requested shell in this order: an explicit valid shell, a configured valid shell, then a recognized `$SHELL` hint. Reject invalid explicit or configured choices; when the environment hint is absent or unknown, return an explained skip rather than guessing. Add the separate `bin completions sync` contract so one invocation can authorize repair without enabling persistent opt-in, while preserving Cobra's existing `bin completion <shell>` surface for generating this manager's own completion script.

Define structured completion configuration for source and generator choices. Generator overrides must be structured argument vectors associated with an explicit shell and command identity, not shell fragments or free-form command text; retain enough identity to validate later discovery and generation work without executing it in this ticket. Keep bundled-script selection and generation mechanics for their follow-up work.

Persist machine-local per-binary, per-shell completion records with source and generator/member identity, installed command and local path identity, binary and script hashes, adapter version, ownership, status, and actionable reason. Establish cache keys from the relevant binary identity/hash, shell, adapter version, and structured override identity. Cache only successful and unsupported outcomes; transient failures must remain repairable and must not suppress a later attempt. State must distinguish disabled/skipped, unsupported, successful, and repairable failure outcomes without treating a failed completion operation as an installation failure.

Keep completion consent, local paths, ownership, and cached status out of portable export/import. Preserve old configuration behavior when new policy or local-state fields are absent, and use the established locked configuration mutation path for concurrent local-state updates. State and write failures must roll back when possible or leave an explicitly repairable local record; they must not corrupt managed-binary metadata or alter existing lifecycle, provider, artifact, system-package, hook, inspect, or Cobra completion behavior.

**Blocked by:** 10 / Select DMG bundles by managed identity; 12 / Inspect artifact decisions without side effects

**Status:** ready-for-agent

- [ ] Completion management defaults to disabled, and explicit invocation policy overrides configured policy, which overrides the default without implicitly authorizing completion work.
- [ ] Bash, Zsh, and Fish resolution follows explicit shell, configured shell, then recognized environment-shell hint; invalid configured or explicit values fail, while absent or unknown hints produce an explained skip.
- [ ] `bin completions sync` authorizes a one-time repair without persisting opt-in, and `bin completion <shell>` remains the separate Cobra completion interface.
- [ ] Completion source and generator overrides use validated structured identities and argument vectors, never shell fragments or arbitrary free-form command text.
- [ ] Local per-binary/per-shell state records source, generator/member identity, command/path identity, binary/script hashes, adapter version, ownership, status, and actionable reason.
- [ ] Cache keys include relevant binary, shell, adapter, and override identity; only successful and unsupported outcomes are cached, while transient failures remain eligible for repair.
- [ ] Portable export/import excludes local completion consent, paths, ownership, and cached status, and older records/configuration without completion fields remain compatible.
- [ ] Completion-state mutation uses established locking and failure handling leaves either rolled-back state or an explicit repairable record without corrupting binary configuration.
- [ ] Existing install, update, ensure, remove, prune, provider, artifact, system-package, lifecycle-hook, inspection, and Cobra completion behavior remains unchanged.


- [ ] Add policy and command regressions for default-off behavior, explicit/configured/default precedence, valid and invalid shell resolution, explained environment-hint skips, and one-time sync authorization.
- [ ] Add command-contract regressions proving `completions sync` remains distinct from the manager's existing `completion <shell>` command.
- [ ] Add configuration regressions for structured source/generator overrides, local per-shell state serialization, cache-key invalidation inputs, cacheable success/unsupported outcomes, and retryable transient failures.
- [ ] Add portable export/import and backward-compatibility regressions proving machine-local completion fields neither transfer nor break older configuration.
- [ ] Add locking and failure-path regressions for concurrent local-state mutation, rollback or repairable state, and preservation of managed-binary metadata.
- [ ] Run the focused command and configuration test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because completion state and configuration locking cross shared local state.
