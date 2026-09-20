# Reliability and managed completions

Status: ready-for-agent

## Current baseline

Tickets 01–12 are implemented at `3240497`. Keep their behavior and regression coverage: safe remote names, checksum binding, executable replacement and config recovery, bounded archive processing, deterministic product/member selection, persisted selection intent, DMG selection, generic URLs, and read-only inspection. Their resolved tickets remain the detailed record; this revision does not reopen them.

The remaining requirement is convenient, opt-in Bash, Zsh, and Fish completions for managed direct binaries. Implement that at personal CLI scale. The previous completion policy/cache framework, shell parsers, process containment, and publication transactions are superseded by the contract below.

## User-visible behavior

- `bin completions sync <binary> <shell>` installs or refreshes completions for one existing managed binary and one explicit `bash`, `zsh`, or `fish` shell. Reuse managed-target lookup. It authorizes this invocation only.
- `bin completions sync <binary> <shell> -- <generator-args...>` explicitly runs that installed binary with exactly those arguments, bypassing bundled discovery. Arguments are argv, never a shell command string; they are not saved.
- Without an override, prefer one matching bundled script from the installed release; otherwise try exactly `<installed-executable> completion <shell>`. A nonzero exit or unusable output is an unsupported/failed attempt with a useful message. Do not probe other command shapes, infer frameworks from help, or maintain an adapter registry.
- `bin install --completions=<shell>` opts that binary into automatic completion refresh after successful installs and updates. New installs default to off. Reinstall without the flag preserves the local choice; `--completions=off` disables future automatic refresh. Explicit sync does not change it.
- Automatic refresh runs once at the shared direct-binary post-commit boundary. An actual reinstall by `ensure` uses the same path; an unchanged binary does no completion work. Completion failure warns and preserves binary success and update exit behavior.
- `remove` and `prune` make a best-effort attempt to delete only that binary's recorded, unmodified completion files. Failures name the leftover path for manual cleanup and do not undo binary removal.
- `run`, `inspect`, dry runs, and system-package operations do not request managed completions. Failed installs never execute generators, publish shell files, or save completion state; temporary bundled bytes collected during their ordinary fetch are simply discarded. Cobra's existing `bin completion <shell>` still generates completions for `bin` itself.

## Small implementation contract

### Execution and trust

Use `os/exec` with an absolute installed executable, direct argv, closed stdin, a temporary working directory, and a small explicit environment without inherited credentials or shell startup variables. Never elevate; skip generation when elevated. Use a fixed five-second command deadline, at most 1 MiB of stdout, 64 KiB of stderr, and a 250 ms pipe wait after cancellation/exit. Cancel on output overflow and discard partial output after any failure. These are constants, not user configuration.

Require successful exit and nonempty UTF-8 text without NUL bytes. Do not implement a shell grammar or registration parser, source the output, or attempt to prove that a completion is safe. The user is authorizing code from the installed tool/release, including code the shell will later load. A normal command timeout limits the launched process; it is not a sandbox or a guarantee that every descendant is killed. No cgroups, namespaces, containers, Job Objects, process-tree inventory, or platform support gate based on containment.

### Bundled scripts

Reuse the current bounded artifact inventory and its single cleanup owner. Only consider regular completion entries belonging to the selected executable's archive scope. Match exact command filenames in the existing `completions`, `autocomplete`, or `complete` directories: `<command>.bash` or `bash/<command>` for Bash, `_<command>` for Zsh, and `<command>.fish` for Fish. One matching candidate wins; ambiguity or malformed text warns/skips rather than guessing. Do not rewrite script registrations or rank approximate names. For renamed installs without an exact matching command, skip automatic discovery/generation; explicit generator argv remains available to the user.

During install/update, use the artifact already fetched. Copy at most the selected bounded script into the existing result before its temporary inventory closes. Add only the request/result fields needed to carry the selected shell and script; do not expose the whole inventory or build a new provider interface.

For explicit sync without an argv override, reuse the existing provider fetch path at the stored version and selection intent for HTTP release/generic sources. Preserve HTTP/auth policy. Use bundled content only when the fetched executable matches the current installed bytes; otherwise skip that content and use the installed generator. Preserve fetch/security errors rather than treating them as absent bundled content; explicit generator argv lets the user bypass fetching. Docker and Go-install sources go directly to native generation rather than re-running those providers. A missing/changed installed binary fails before mutation. Clean up the fetched artifact on every path. Do not add a completion download cache or persist discovery results.

### Destinations and ownership

Use one bin-owned `completions/<shell>` directory beside the active config file. Create the private directories as needed; do not support custom destinations, shell-directory discovery, or package-manager directories. Use `<command>` for Bash, `_<command>` for Zsh, and `<command>.fish` for Fish. Validate command names with the existing filename rules and keep operations within the owned subtree. Reject symlinked owned directories, symlink destinations, and non-regular destination entries.

The only new persisted data is one optional local automatic-shell choice per binary and a per-shell ownership record containing the written path and SHA-256. The containing binary record identifies the owner. No source/generator identities, adapter versions, cache keys, binary-hash copies, status enum, retry ledger, or stored failure reasons. Exclude the new fields from portable export/import, preserve destination-local values when importing over an existing record, and default old configs to off.

Stage the complete script next to its destination and reuse the existing platform publication primitives. Serialize bin's ownership check, publication, and ownership save through the existing config lock; keep downloads and generator execution outside it. Before publication, check that the binary is still managed with the same installed hash used for this request; discard results made stale by a concurrent update/remove. Replace only a regular file whose recorded owner/path/hash still match. Refuse an unmanaged, user-modified, or differently owned destination; do not choose an active owner or transfer ownership automatically. A missing owned file can be recreated by an authorized request. Preserve the current file if staging or publication fails.

Save ownership only after publication. If that config save fails, leave the published file and warn with its path. Later operations still require matching stored ownership: an unrecorded file or hash mismatch requires manual removal before retrying sync. This intentionally accepts manual cleanup instead of a second file/config transaction. Do not add backups, rollback, recovery journals, or automatic adoption of an unrecorded file. Clean up ordinary staging files; a cleanup warning must not roll back a valid completion or binary.

Before removal, check the same owner/path/hash and filesystem rules. Preserve changed, unrecorded, or conflicting files and report why. After an unsuccessful cleanup, forget ownership when the binary record is removed and report the leftover path; no orphan-record database or background retry. These checks protect ordinary user files and serialize cooperating bin operations; do not expand this feature into protection against a hostile process running as the same OS user.

### Shell setup

Print the actual completion directory and concise manual setup guidance. Bash users source the owned files or add their directory to their existing completion setup; Zsh users add the directory to `fpath` before `compinit`; Fish users add it to `fish_complete_path`. Preserve Zsh's first-line `#compdef` convention. Do not edit startup files or load scripts into the current shell.

## Delivery

Each implementation ticket delivers usable behavior, with focused regressions in existing suites. Use existing helpers and dependencies; no new framework, service layer, or dependency is expected.

| Ticket | Deliverable | Depends on |
| --- | --- | --- |
| [13](issues/13-sync-managed-completions.md) | Explicit native sync and safe owned-file writes | Completed 12 |
| [14](issues/14-use-bundled-completions.md) | Prefer exact bundled scripts using existing artifact processing | 13 |
| [17](issues/17-integrate-completion-lifecycle.md) | Per-binary opt-in, post-install refresh, and safe cleanup | 14 |
| [18](issues/18-document-and-verify.md) | Usage docs and the remaining native integration checks | 17 |

Former tickets 15 and 16 are folded into ticket 13; separate runner and publication framework tickets are removed. Numbers 17 and 18 remain stable for existing references. The independent Windows compatibility tickets remain necessary.

## Validation

Reuse `setupTestConfig`, temporary directories, synthetic archives, local HTTP servers, and controlled helper executables. Test the useful boundaries: supported output, timeout/overflow/nonzero exit, bundled preference and ambiguity, preserving modified/unmanaged/symlink files, file/config failure handling, opt-in and export isolation, post-commit ordering, and removal. No tests that merely assert documentation wording, source structure, or a second copy of the implementation.

Follow the existing AGENTS.md checks: focused tests while iterating, `mise run lint`, `mise run test-race` for filesystem/shared-state/process changes, and `mise run test`, `mise run verify`, and `mise run build` before implementation handoff. Run them once on the final change unless another edit or failure justifies repeating them.

Native Linux/macOS coverage uses the existing Go suite plus small isolated shell smoke tests. Windows compatibility ticket 03 owns its native CI job; reuse it instead of creating a second matrix. Do not require live providers, privileged installation, developer config, or a new acceptance framework. Record unavailable native coverage accurately. Resolved tickets 01–12 do not need another ticket-by-ticket approval exercise.

## Deliberately deferred

Multi-shell automatic policy, global policy precedence, `$SHELL` inference, persistent custom generators, arbitrary protocol probing, alias rewriting, configurable destinations, automatic repair of unchanged binaries, status commands, repair queues, ownership transfer, shell parsing, process containment, completion transactions, PowerShell completions, and package-manager completion management. Revisit a specific item only for a demonstrated user need.
