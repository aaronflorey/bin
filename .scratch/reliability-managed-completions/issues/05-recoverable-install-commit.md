# 05: Commit executable and config state recoverably

**What to build:**

Coordinate successful direct-binary replacement with managed-config persistence as a narrow recoverable transaction. Keep download, prompting, staging, hashing, payload validation, and executable preparation outside the transaction. Once replacement is ready, acquire a short cross-process install/config lock, reload current managed state, merge only the transaction's intended binary record, publish the executable, and persist the merged config while retaining rollback material until the commit succeeds.

If config persistence fails after publication, restore only the executable and metadata owned by this transaction. Do not restore a stale whole-config snapshot or undo unrelated concurrent changes. If rollback cannot complete, record explicit recoverable unresolved state that prevents later lifecycle operations from claiming either the old or new executable and metadata were committed successfully; provide exceptional reconciliation that can safely resolve that state against the observed filesystem and current config.

Isolate Go-install build output before it enters the direct-binary transaction, so build or preparation failures cannot affect an existing managed executable. Preserve distinct system-package semantics. Keep lifecycle boundaries intact: hooks and normal install/update/ensure/remove behavior must continue to observe a committed binary/config pair, and failed commits must preserve prior managed state without triggering success-only behavior.

**Blocked by:** 04 / Replace direct binaries without deleting the working version

**Status:** ready-for-agent

- [ ] Direct-binary executable publication and its managed-config update use a short cross-process transaction lock; downloading, prompting, staging, hashing, validation, and mode preparation remain outside that lock.
- [ ] The transaction reloads current managed state while locked and merges only its intended binary mutation, preserving unrelated records and concurrent changes rather than writing a stale config snapshot.
- [ ] Config persistence occurs while transaction rollback material remains available; successful commit removes only transaction-owned rollback material.
- [ ] A config-write failure after executable publication restores this transaction's prior executable and managed record without rolling back or deleting another operation's successful executable or unrelated config mutation.
- [ ] A publication, persistence, or rollback failure cannot leave ordinary managed metadata falsely asserting a successfully committed old or new installation.
- [ ] An incomplete rollback is recorded as explicit recoverable unresolved state, and exceptional reconciliation safely compares current filesystem and managed state before resolving or reporting the inconsistency.
- [ ] Concurrent installs or updates of different binaries retain each other's records; concurrent work for the same binary cannot use, remove, or restore rollback material owned by another transaction.
- [ ] Go-install output is isolated before direct-binary commit preparation, preserving an existing destination on build, staging, replacement, or persistence failure.
- [ ] System-package behavior remains separate, and existing payload validation, integrity evidence, aliases, release lanes, pinning, non-interactive safety, lifecycle hooks, and update aggregate failure behavior remain intact.


- [ ] Add command and persistence regressions for config-write failure after executable publication, successful commit cleanup, and preservation of the prior executable and record on failed commit.
- [ ] Add concurrency regressions for reload-and-merge behavior, unrelated binary mutations, same-binary contention, and transaction-owned rollback isolation.
- [ ] Add recovery regressions for rollback failure and exceptional reconciliation, including prevention of false successful-state reporting.
- [ ] Add Go-install isolation regressions covering build and commit failures with an existing managed executable.
- [ ] Exercise install, update, ensure, and remove lifecycle boundaries to confirm hooks and success behavior run only for committed state and system-package behavior is unchanged.
- [ ] Run the focused command, configuration, provider, and artifact-processing test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because transaction locking, filesystem replacement, rollback, and persisted state are concurrent-state behavior.
