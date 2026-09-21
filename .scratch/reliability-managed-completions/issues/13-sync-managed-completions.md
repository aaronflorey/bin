# 13: Sync completions for one managed binary

**Status:** resolved

**Blocked by:** 12 / Inspect artifact decisions without side effects (resolved).

## What to build

Deliver `bin completions sync <binary> <shell> [-- <generator-args...>]` end to end, following the [revised completion contract](../spec.md). Reuse command constructors, managed-target lookup, config mutation, filename validation, and platform file publication already in `cmd/` and `pkg/config/`.

Require an explicit supported shell and an existing, unchanged direct managed binary. Invoke its absolute path with the supplied argv, or exactly `completion <shell>` when none is supplied. This invocation authorizes execution; it does not enable automatic work. Use a normal `os/exec` deadline, bounded stdout/stderr and pipe wait, temporary cwd, closed stdin, and a credential-free environment. Skip elevated execution and default generation for renamed commands. No shell invocation, protocol detection, shell parser, process containment, or runner framework.

Accept only successful, nonempty bounded text as specified in the spec. Write it to the bin-owned shell directory beside the active config, using staged publication and the existing config lock. The only completion metadata is per-shell path/hash ownership in the existing binary record. Reject unmanaged, modified, symlinked, non-regular, or competing destinations. Preserve existing content on failed staging/publication. If ownership persistence fails after publication, leave the file and report its exact path for manual removal and retry; do not add rollback or recovery state.

Keep these fields local during config cloning and export/import. Print the installed path and manual shell setup guidance. Return a normal command error on failure; do not add a persistent status model. Bundled selection and automatic lifecycle work belong to 14 and 17.

## Acceptance

- [x] A local helper generates Bash, Zsh, and Fish files through both the default argv and an explicit argv override, with no shell interpretation of arguments.
- [x] Invalid shell, missing/changed/unmanaged binary, system-package target, timeout, output overflow, empty/non-text output, and nonzero exit leave an existing completion intact.
- [x] Owned regular files can be refreshed or recreated if missing; unowned, modified, symlinked, non-regular, and differently owned files remain untouched, including cooperating concurrent sync attempts. Results made stale by concurrent binary update/removal are discarded before publication.
- [x] Ownership save failure leaves an explicit manual-cleanup result and never rolls back the installed binary or silently adopts an unrecorded file later.
- [x] Local ownership survives ordinary config updates and stays out of portable export/import. No invocation enables future execution.
- [x] Cobra's singular `completion` command and shell startup files remain unchanged.

## Verification

Add focused behavioral cases to the existing command/config suites using `setupTestConfig` and controlled helpers. Reuse platform publication coverage; do not build a parallel publication test framework. Run the applicable focused, lint, race, test, verify, and build checks from the spec once the implementation is complete.

## Implementation progress

- [x] Add local completion ownership metadata with clone/export/import preservation rules.
- [x] Implement bounded native generator execution and owned-file publication.
- [x] Add and register `completions sync` with focused command coverage.
- [x] Run final repository verification and address failures.
- [x] Complete independent code review.
