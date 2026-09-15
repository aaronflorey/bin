# 18: Complete documentation and native acceptance coverage

**What to build:**

Complete the user-facing and maintainer documentation for the reliability work as a coherent set of contracts. Document the stricter remote-name, checksum/integrity, bounded artifact processing and selection, transaction recovery, read-only inspection, and managed-completion behavior. Explain supported formats and limits, persisted selection and integrity meaning, recovery and repair outcomes, completion consent, shell setup, status, ownership, conflict handling, and the distinction between process restrictions and a trust boundary. Preserve existing command behavior and clearly retain the separation between managed completions and the CLI's own completion surface.

Add native acceptance coverage for the completed lifecycle rather than relying only on unit seams. Exercise supported Linux, macOS, and Windows filesystem, replacement, process, and shell-related behavior where those native environments are available, plus Unix shell integration acceptance. Use controlled helpers and isolated fixtures only; never depend on live providers, privileged installation, developer configuration, installed binaries, shell startup files, or other developer state. Treat unavailable native environments as unavailable coverage rather than claiming they passed.

Close the reliability-managed-completions effort only after the acceptance evidence, documentation review, and implementation review can be traced back to the completed tickets and their acceptance criteria.

**Blocked by:** 17 / Integrate completion lifecycle and sync

**Status:** ready-for-agent

- [ ] Documentation covers the reliability, integrity, artifact-selection, inspection, completion, and recovery contracts introduced across this effort, including compatibility boundaries and actionable failure or repair outcomes.
- [ ] Documentation explains supported artifact and checksum formats, finite processing limits, strict selection behavior, persisted intent and integrity scope, and the retained meaning of established metadata.
- [ ] Documentation explains inspection's read-only and redaction guarantees, rejected effectful sources, and its relationship to installation decisions.
- [ ] Documentation explains completion opt-in and precedence, supported shells and setup guidance, sync and repair behavior, ownership/conflict preservation, lifecycle warning semantics, and the distinct manager-completion command surface.
- [ ] Documentation states that constrained completion generation is not a sandbox or proof of trustworthiness and does not imply shell-startup editing, elevation, or execution without authorization.
- [ ] Native acceptance coverage validates applicable Linux, macOS, and Windows replacement, filesystem, and process behavior in isolated fixtures, reporting unavailable platform coverage accurately.
- [ ] Unix shell integration acceptance validates supported completion behavior without modifying startup files, using developer shell state, or requiring installed user tools.
- [ ] Acceptance fixtures use controlled local inputs and helpers; they perform no live-provider requests, privileged/package-manager installation, or developer-state mutation.
- [ ] The effort is marked complete only after each completed ticket's acceptance evidence is reviewed and final review findings are traceable to the relevant ticket or documented resolution.
- [ ] Existing install, update, ensure, remove, prune, inspect, completion, provider, artifact, integrity, configuration, system-package, hook, and aggregate update-failure behavior remains unchanged except for the documented completed contracts.


- [ ] Add documentation review checks covering the completed reliability, selection, integrity, inspection, completion, and recovery contracts and their user-visible boundaries.
- [ ] Add isolated native acceptance regressions for Linux, macOS, and Windows behavior where available, and Unix shell integration acceptance without startup-file or developer-state mutation.
- [ ] Add acceptance-fixture regressions proving no live-provider, privileged, package-manager, or developer-state dependency.
- [ ] Review every completed ticket against its acceptance criteria; record review findings and resolutions with ticket-level traceability before marking this issue complete.
- [ ] Run the focused command, configuration, provider, asset, completion, and platform-specific test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because the completed work includes filesystem replacement, process handling, ownership, locking, and persisted lifecycle state.
- [ ] Run `mise run test`, `mise run verify`, and `mise run build`.
- [ ] Run the documented Action test command only if Action files change.
