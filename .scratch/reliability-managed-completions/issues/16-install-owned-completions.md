# 16: Install completion files with explicit ownership

**What to build:**

Install validated, policy-authorized Bash, Zsh, and Fish completion content only after bundled discovery and bounded native generation have produced an applicable result. Resolve Bash and Fish destinations from the supported per-user, default, or explicitly configured destination policy; use only bin-owned destinations for Zsh. Reject invalid, unavailable, or unsafe destinations with an actionable repairable outcome rather than guessing a location or changing shell startup configuration.

Stage completion content and safely replace a destination only after validating the existing filesystem entry and its ownership. Preserve user-created, user-modified, unmanaged, symlinked, and package-manager-owned files. Record sufficient ownership identity after a successful write to distinguish the exact managed file from a later modification, and use the recorded hash and path identity before repair, replacement, or removal. When another managed command claims the same destination, resolve the active owner explicitly rather than silently overwriting either command's completion.

Coordinate completion-file publication and local completion state so a successful write records ownership, while state failures roll back where possible or leave an explicit repairable recovery result. Never let completion installation undo or fail an already committed binary installation. Provide shell setup guidance and completion status/repair direction without editing startup files, loading the current shell, elevating privileges, or modifying package-manager configuration.

Keep destination policy, ownership, and recovery within managed-completion boundaries. Preserve provider, artifact, executable selection and validation, lifecycle-hook, system-package, persistent portable configuration, and Cobra completion behavior.

**Blocked by:** 15 / Generate completions through a bounded native runner

**Status:** ready-for-agent

- [ ] Validated, authorized Bash, Zsh, and Fish completion content installs only after applicable bundled discovery or bounded generation succeeds.
- [ ] Bash and Fish destination resolution supports per-user, default, and explicit configured destinations; Zsh installation is limited to bin-owned destinations.
- [ ] Invalid, unavailable, or unsafe destination choices return actionable repairable outcomes without guessing a location.
- [ ] Publication stages content before replacement and preserves existing user-created, modified, unmanaged, symlinked, and package-manager-owned files.
- [ ] Ownership is recorded only after a successful write and identifies the managed destination and its unmodified content for later repair, replacement, and removal decisions.
- [ ] Completion-file and state-write failures roll back when possible or leave explicit repairable recovery state without corrupting managed-binary metadata.
- [ ] Destination collisions between managed commands identify and resolve the active owner explicitly; no command silently overwrites another command's completion.
- [ ] Setup guidance explains how users may enable supported shell loading and repair completions without editing shell startup files or loading completions into the current shell.
- [ ] Completion publication failures occur after binary commit and remain warnings or repairable outcomes; an otherwise successful install remains successful.
- [ ] Existing provider, artifact, executable selection and validation, lifecycle-hook, system-package, portable configuration, and Cobra completion behavior remains unchanged.


- [ ] Add completion and command regressions for policy-gated publication from bundled and generated evidence across Bash, Zsh, and Fish.
- [ ] Add filesystem regressions for supported destination resolution, invalid or unavailable destinations, staged replacement, and preservation of user, modified, unmanaged, symlink, and package-manager-owned entries.
- [ ] Add ownership regressions for post-write recording, hash/path verification before mutation, repairable write/state failures, rollback where possible, and recovery without managed-binary metadata corruption.
- [ ] Add collision regressions for competing managed commands and explicit active-owner resolution without silent overwrite.
- [ ] Add command regressions for setup guidance and status/repair direction, proving shell startup files and the current shell remain unmodified.
- [ ] Add lifecycle regressions proving completion publication runs after binary commit and cannot convert a successful installation into a failure.
- [ ] Run the focused completion, command, configuration, and artifact-processing test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because completion ownership, state recovery, and filesystem publication cross shared locking boundaries.
