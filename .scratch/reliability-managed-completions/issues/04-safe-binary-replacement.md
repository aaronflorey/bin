# 04: Replace direct binaries without deleting the working version

**What to build:**

Make direct-binary installation and update replacement transactional at the filesystem boundary. Stage the candidate as a sibling of the managed destination, then complete copying, hashing, payload validation, executable-mode preparation, and stream closure before attempting publication. A failed staging or validation step must remove only its staging material and leave the existing executable untouched.

Use platform-native replacement semantics rather than deleting the destination first. On Unix, publish the prepared sibling atomically while preserving the existing destination until that publication succeeds. On Windows, use replacement behavior that accounts for locked files and platform rename constraints without treating a delete-then-move sequence as safe. Keep rollback material only as long as replacement needs it, and clean it after a successful outcome.

Preserve managed symlink behavior: do not silently replace a link target or follow a link into an unintended location. Concurrent non-force operations must not overwrite another operation's successful result or remove its rollback material. Keep direct-binary semantics distinct from system-package installation, and retain existing lifecycle, validation, aliases, release lanes, pinning, and non-interactive behavior.

Keep integrity evidence aligned with the bytes ultimately installed. Failed replacement must preserve the prior executable and its managed metadata; successful replacement must not retain stale staging files, backups, or integrity claims from the previous bytes. Leave the later config-persistence transaction and recovery coordination to its dedicated follow-up work.

**Blocked by:** 01 / Contain remote names and clean failed fetches; 03 / Verify download and installed-byte integrity

**Status:** resolved

- [x] A direct-binary candidate is fully staged beside its destination, copied, hashed, validated, mode-prepared, and closed before replacement is attempted.
- [x] Any download, copy, hash, validation, mode, close, or staging failure preserves the prior executable and removes newly created temporary material.
- [x] Unix replacement publishes the prepared candidate without a delete-first gap and leaves the old destination intact when publication fails.
- [x] Windows replacement handles locked destinations and replacement failures without deleting the working executable before a replacement can succeed.
- [x] Replacement does not follow or silently retarget managed symlinks; unsafe or unsupported symlink cases fail while preserving the existing installation.
- [x] Concurrent non-force replacement attempts cannot delete, overwrite, or clean up the executable or rollback material owned by another successful operation.
- [x] Successful replacement leaves the intended new executable with correct executable permissions and installed-byte integrity metadata, and removes obsolete staging and rollback material.
- [x] Direct-binary changes do not alter system-package installation semantics, existing payload validation, provider behavior, aliases, release lanes, pinning, or non-interactive safety.


- [x] Add native filesystem regressions for staged validation failure, copy/close failure, publication failure, successful cleanup, and preservation of the old executable and managed metadata.
- [x] Add platform-focused tests for Unix atomic publication and Windows locked-file/replacement behavior using controlled filesystem or process seams where needed.
- [x] Add regressions covering managed symlinks and concurrent non-force replacement attempts, including ownership-safe cleanup.
- [x] Run the focused command, asset-processing, and configuration test suites.
- [x] Run `mise run lint`.
- [x] Run `mise run test-race` because replacement changes filesystem and concurrent-state behavior.

## Comments

### Implementation plan (2026-09-15)

1. Stage and publish direct binaries safely in `cmd/installer.go` plus platform replacement files. Reuse existing payload validation, hashing, chmod, provider-close, and config persistence flow; preserve system-package behavior. Acceptance: all staging work and stream/file closure precedes publication, overwrite publication never deletes first, and failed publication preserves the destination.
2. Add focused native regressions for close/publication failure, cleanup, symlinks, and concurrent non-force publication; add Windows-specific replacement coverage or controlled cross-platform checks. Acceptance: tests prove ownership-safe cleanup and old-byte/config preservation without asserting trivial assigned values.
3. Run focused command/assets/config tests, lint, race tests, full tests, verify, and build where relevant. Excludes config-persistence rollback/locking (ticket 05), system-package transaction changes, and unrelated refactors.

### Slice 1 completed (2026-09-15)

- Added sibling staging and close-before-publication to the direct install/update path while keeping cache and system-package stream ownership unchanged.
- Added Linux `RENAME_NOREPLACE`, Darwin `RENAME_EXCL`, and Windows `MoveFileEx` publication, with overwrite publication retaining the prior destination until the native replacement succeeds.
- Added initial cleanup, close-failure, publication-failure, and destination-symlink regressions.
- Focused tests, full tests, lint, verify, build, and Linux/Darwin/Windows compile checks passed in the slice agent.

### Slice 2 completed (2026-09-15)

- Added successful overwrite and deterministic concurrent non-force regressions, proving one winner and operation-owned staging cleanup.
- Added install-level publication-failure coverage for prior metadata and a Windows-only locked-destination preservation regression.
- Focused tests, focused race coverage, assets/config tests, lint, and Darwin/Windows compile checks passed. Windows behavior was compile-checked only on this Linux host.

### Slice 3 completed (2026-09-15)

- Reviewed close ownership and made chmod-file close errors fail before publication.
- Focused command/assets/config tests, lint, full race tests, full tests, verify, build, and Darwin/Windows cross-compilation all passed.
- Native Darwin and Windows behavioral execution remains unavailable on this Linux host; their command test binaries compile successfully.

### Review correction (2026-09-15)

- Code review found a check-then-rename race on fallback platforms. Non-force fallback publication now uses an atomic same-directory hard link and removes only its own stage after publication.
- Added FreeBSD-tagged fallback regressions; command tests and lint pass, and FreeBSD/OpenBSD command tests cross-compile. Native fallback-platform execution is unavailable on this Linux host.

### Independent review (2026-09-15)

- Standards review: approved with no documented-standard violations or actionable smells.
- Spec review: approved after the fallback-platform concurrency correction; no remaining blocking ticket-04 issue.

## Answer

Direct binary installs and updates now fully prepare and close a sibling staging file before platform-native publication. Overwrite failures preserve existing executables, non-force publication is atomic against concurrent winners, destination symlinks are rejected, and each operation removes only its own staging material. Added native filesystem, concurrency, metadata-preservation, fallback-platform, and Windows locked-destination regressions. Focused and full tests, race tests, lint, verify, build, and cross-platform compile checks pass; native non-Linux behavioral execution was unavailable on this host. Independent standards and spec reviews approved the final diff.
