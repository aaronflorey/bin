# 04: Replace direct binaries without deleting the working version

**What to build:**

Make direct-binary installation and update replacement transactional at the filesystem boundary. Stage the candidate as a sibling of the managed destination, then complete copying, hashing, payload validation, executable-mode preparation, and stream closure before attempting publication. A failed staging or validation step must remove only its staging material and leave the existing executable untouched.

Use platform-native replacement semantics rather than deleting the destination first. On Unix, publish the prepared sibling atomically while preserving the existing destination until that publication succeeds. On Windows, use replacement behavior that accounts for locked files and platform rename constraints without treating a delete-then-move sequence as safe. Keep rollback material only as long as replacement needs it, and clean it after a successful outcome.

Preserve managed symlink behavior: do not silently replace a link target or follow a link into an unintended location. Concurrent non-force operations must not overwrite another operation's successful result or remove its rollback material. Keep direct-binary semantics distinct from system-package installation, and retain existing lifecycle, validation, aliases, release lanes, pinning, and non-interactive behavior.

Keep integrity evidence aligned with the bytes ultimately installed. Failed replacement must preserve the prior executable and its managed metadata; successful replacement must not retain stale staging files, backups, or integrity claims from the previous bytes. Leave the later config-persistence transaction and recovery coordination to its dedicated follow-up work.

**Blocked by:** 01 / Contain remote names and clean failed fetches; 03 / Verify download and installed-byte integrity

**Status:** ready-for-agent

- [ ] A direct-binary candidate is fully staged beside its destination, copied, hashed, validated, mode-prepared, and closed before replacement is attempted.
- [ ] Any download, copy, hash, validation, mode, close, or staging failure preserves the prior executable and removes newly created temporary material.
- [ ] Unix replacement publishes the prepared candidate without a delete-first gap and leaves the old destination intact when publication fails.
- [ ] Windows replacement handles locked destinations and replacement failures without deleting the working executable before a replacement can succeed.
- [ ] Replacement does not follow or silently retarget managed symlinks; unsafe or unsupported symlink cases fail while preserving the existing installation.
- [ ] Concurrent non-force replacement attempts cannot delete, overwrite, or clean up the executable or rollback material owned by another successful operation.
- [ ] Successful replacement leaves the intended new executable with correct executable permissions and installed-byte integrity metadata, and removes obsolete staging and rollback material.
- [ ] Direct-binary changes do not alter system-package installation semantics, existing payload validation, provider behavior, aliases, release lanes, pinning, or non-interactive safety.


- [ ] Add native filesystem regressions for staged validation failure, copy/close failure, publication failure, successful cleanup, and preservation of the old executable and managed metadata.
- [ ] Add platform-focused tests for Unix atomic publication and Windows locked-file/replacement behavior using controlled filesystem or process seams where needed.
- [ ] Add regressions covering managed symlinks and concurrent non-force replacement attempts, including ownership-safe cleanup.
- [ ] Run the focused command, asset-processing, and configuration test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because replacement changes filesystem and concurrent-state behavior.
