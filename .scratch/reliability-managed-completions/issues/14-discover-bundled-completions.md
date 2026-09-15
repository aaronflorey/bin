# 14: Discover completion scripts from artifact inventory

**What to build:**

Discover bundled Bash, Zsh, and Fish completion scripts from the bounded artifact inventory after completion policy has authorized the work. Document the supported shell-specific filename, syntax, and registration conventions, and validate scripts and registrations without sourcing them or treating format validation as a trust decision.

Associate a bundled script only with the selected product and installed command identity. Prefer applicable corroborated bundled evidence over later generator work, keep completion entries separate from executable candidates, and reject unrelated tools, unsupported aliases, malformed registrations, and ambiguous matching evidence rather than choosing by archive order or filename resemblance.

Produce a bounded cache handoff for later completion work that is tied to the relevant installed binary hash, selected product and command identity, shell, script/member identity, and adapter version. Retain only enough validated discovery evidence to avoid reprocessing unchanged artifacts; do not cache transient discovery failures as final outcomes. A sync repair must re-fetch and revalidate bundled evidence when needed, and it may reuse a handoff only when the current binary hash and selected identity still match.

Keep discovery non-executing and contained within the existing artifact-processing and completion-policy boundaries. Preserve existing executable selection, payload validation, provider, lifecycle, state, and Cobra completion behavior; unsupported or repairable completion discovery outcomes must not turn a successful binary installation into a failure.

**Blocked by:** 13 / Define managed-completion policy and local state

**Status:** ready-for-agent

- [ ] Bundled Bash, Zsh, and Fish scripts are discovered only from the bounded artifact inventory after completion policy authorizes discovery.
- [ ] Supported shell-specific filename, syntax, and registration conventions are documented and validated without sourcing scripts or executing downloaded commands.
- [ ] A discovered script is associated with the selected product and installed command identity; completion entries remain distinct from executable candidates.
- [ ] Applicable corroborated bundled evidence is preferred over generator fallback, while unrelated tools, malformed registrations, unsupported aliases, and ambiguous evidence are rejected with actionable outcomes.
- [ ] Discovery hands off bounded validated evidence keyed to the installed binary hash, selected identity, shell, member/script identity, and adapter version; transient failures do not become suppressing cache entries.
- [ ] Sync repair re-fetches and validates bundled evidence as needed and accepts a cached handoff only when the current binary hash and selected identity match.
- [ ] Existing artifact processing, executable selection and validation, provider, lifecycle, state, and Cobra completion behavior remains unchanged; completion discovery failures remain non-fatal after binary commit.


- [ ] Add artifact and completion regressions for supported Bash, Zsh, and Fish conventions, registration validation, non-execution, and selected-product command association.
- [ ] Add regressions for bundled preference, unrelated-tool rejection, malformed or unsupported aliases, deterministic ambiguity failures, and separation from executable candidates.
- [ ] Add state and sync regressions for bounded hash-bound handoff reuse, stale-hash or identity rejection, re-fetch and revalidation, and retryable discovery failures.
- [ ] Add lifecycle regressions proving discovery occurs only when authorized and a repairable discovery outcome does not fail an otherwise successful installation.
- [ ] Run the focused artifact-processing, completion, command, and configuration test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because bounded completion evidence and local state cross shared filesystem and locking boundaries.
