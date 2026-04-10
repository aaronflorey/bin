# Codebase Concerns

**Analysis Date:** 2026-04-10

## Tech Debt

**Config persistence and mutation flow:**
- Issue: Configuration state is held in the global `cfg` singleton and every mutation rewrites the entire JSON file in place with `os.O_TRUNC`, without atomic rename or locking.
- Files: `pkg/config/config.go`, `cmd/install.go`, `cmd/update.go`, `cmd/remove.go`, `cmd/import.go`, `cmd/prune.go`
- Impact: Interrupted writes can leave `config.json` partially written, and concurrent command execution can clobber changes because `UpsertBinary`, `UpsertBinaries`, and `RemoveBinaries` all mutate shared in-memory state then overwrite the same file.
- Fix approach: Write to a temp file in the same directory and `os.Rename` it into place, add file locking around writes, and keep config mutation isolated behind a single persistence layer.

**Removal path is not transactional:**
- Issue: `bin remove` deletes entries from config before provider cleanup and before removing the installed file.
- Files: `cmd/remove.go`, `pkg/providers/providers.go`
- Impact: If provider cleanup fails or `os.Remove` fails, the binary can remain on disk while disappearing from config, leaving orphaned files and making retry/recovery manual.
- Fix approach: Stage the removal as: validate targets → provider cleanup/file deletion → config update only after successful completion, or add rollback when later steps fail.

**Asset-selection logic is concentrated in one large heuristic-heavy module:**
- Issue: Archive inspection, asset scoring, metadata filtering, package filtering, recursive decompression, and HTTP download handling all live in `pkg/assets/assets.go`.
- Files: `pkg/assets/assets.go`, `pkg/assets/assets_test.go`, `TODO.md`
- Impact: Small selection changes can affect install, update, ensure, and provider flows at once. The file is large enough that regression risk is high, especially around edge-case asset names and nested archives.
- Fix approach: Split `pkg/assets/assets.go` into smaller units for scoring, metadata filtering, archive extraction, and transport; keep behavior pinned with focused tests for real-world asset naming edge cases.

## Known Bugs

**Checksum failure can leave a bad binary at the destination path:**
- Symptoms: `saveToDisk` writes the file first and verifies `ExpectedSHA` only after the write completes.
- Files: `cmd/installer.go`, `pkg/providers/github.go`, `pkg/providers/gitlab.go`, `pkg/providers/codeberg.go`, `pkg/providers/hashicorp.go`
- Trigger: Install or update a release where the downloaded payload does not match the fetched checksum.
- Workaround: Manually delete the partially installed file before retrying.

**`bin remove` can report success while leaving artifacts behind:**
- Symptoms: Config entries are removed even when provider cleanup emits warnings or later file deletion fails.
- Files: `cmd/remove.go`, `pkg/providers/docker.go`
- Trigger: Remove a managed binary whose provider cleanup fails or whose on-disk file cannot be deleted.
- Workaround: Recreate the config entry manually or reinstall the binary, then remove it again after fixing the filesystem/provider issue.

## Security Considerations

**Plain HTTP downloads are accepted for generic URLs:**
- Risk: `providers.New` accepts `http://` inputs and `pkg/providers/generic_url.go` downloads them with the default client. Integrity protection is only best-effort via optional checksum sidecar discovery.
- Files: `pkg/providers/providers.go`, `pkg/providers/generic_url.go`, `cmd/installer.go`, `pkg/providers/checksum.go`
- Current mitigation: SHA-256 verification is attempted when providers expose checksum assets, and HTTPS works when upstream URLs use it.
- Recommendations: Reject plain HTTP by default, or require an explicit `--allow-insecure-http` flag; keep failed checksum installs from persisting files at the destination path.

**Lifecycle hooks execute arbitrary local commands from config:**
- Risk: `ExecuteHooks` runs the configured command directly from `config.json` during install, update, and remove flows.
- Files: `pkg/config/config.go`, `cmd/install.go`, `cmd/update.go`, `cmd/remove.go`
- Current mitigation: Hooks are explicit config entries and are not shell-interpolated.
- Recommendations: Treat `config.json` as trusted local code, document the risk clearly, and consider a dry-run or confirmation mode before first execution of newly added hooks.

## Performance Bottlenecks

**Downloads are buffered fully in memory before processing:**
- Problem: `ProcessURL` copies the full HTTP response into a `bytes.Buffer` before archive detection and extraction.
- Files: `pkg/assets/assets.go`
- Cause: Archive selection needs random access-like behavior for inspection, but the current implementation loads the whole asset first.
- Improvement path: Stream to a temp file instead of memory, then inspect/extract from disk so large releases do not scale memory usage linearly.

**Go-install provider duplicates binary content in memory:**
- Problem: `pkg/providers/goinstall.go` runs `go install`, opens the resulting binary, then reads the whole file with `io.ReadAll` into memory before returning it as a `bytes.Reader`.
- Files: `pkg/providers/goinstall.go`
- Cause: The provider converts an on-disk binary back into an in-memory reader for the shared install path.
- Improvement path: Return a file-backed reader with cleanup semantics, or bypass the extra copy when the provider already produced the final executable on disk.

**Config rewrites scale with total binary count:**
- Problem: Every `UpsertBinary`, `UpsertBinaries`, and `RemoveBinaries` call rewrites the full config file.
- Files: `pkg/config/config.go`
- Cause: The configuration format is a single JSON document keyed by path.
- Improvement path: Keep writes atomic and batched, and consider a storage format that does not require full-file rewrites for every small change.

## Fragile Areas

**Archive and asset heuristics:**
- Files: `pkg/assets/assets.go`, `pkg/assets/assets_test.go`, `TODO.md`
- Why fragile: Selection depends on filename heuristics, metadata suffix/token filtering, archive recursion, and archive-entry filtering. Repositories with unusual release names are easy to mis-rank.
- Safe modification: Add or update tests in `pkg/assets/assets_test.go` for every new asset naming rule before changing heuristics in `pkg/assets/assets.go`.
- Test coverage: Good unit coverage exists for many selectors, but there are no end-to-end command tests proving install/update against representative real release pages.

**Removal and cleanup commands:**
- Files: `cmd/remove.go`, `cmd/prune.go`, `cmd/root.go`
- Why fragile: These commands depend on path resolution through `getBinPath`, environment expansion, config mutation ordering, and provider-specific cleanup. Failure handling is asymmetric.
- Safe modification: Preserve the distinction between config path and expanded filesystem path, and add command-level tests before changing ordering.
- Test coverage: No dedicated `cmd/remove_test.go` or `cmd/prune_test.go` was detected.

**Import/export workflow:**
- Files: `cmd/export.go`, `cmd/import.go`, `cmd/export_import_test.go`
- Why fragile: Export omits filesystem paths by design, while import always reconstructs target paths under the current `default_path` and writes config entries immediately.
- Safe modification: Keep import/export schema changes backward-compatible and verify them with round-trip tests in `cmd/export_import_test.go`.
- Test coverage: Round-trip and status-output tests exist, but there is no install/ensure integration test that validates imported entries become usable binaries on disk.

## Scaling Limits

**Update checks fan out to remote APIs quickly:**
- Current capacity: `collectAvailableUpdates` defaults to 10 concurrent workers.
- Limit: Large configs can generate bursts of GitHub, GitLab, Docker Hub, HashiCorp, and generic HTTP requests, increasing rate-limit and timeout risk.
- Scaling path: Add per-provider throttling, backoff, cached release metadata, and a low-concurrency mode for CI or rate-limited environments.

**Single-file config storage:**
- Current capacity: All managed binaries live in one `config.json` map keyed by path.
- Limit: Startup, mutation, and persistence cost grow with the total number of tracked binaries, and corruption affects the whole inventory at once.
- Scaling path: Keep the current schema only for small inventories, or move to a safer persistence layer with atomic snapshots and partial updates.

## Dependencies at Risk

**`github.com/cheggaaa/pb v2.0.7+incompatible`:**
- Risk: Old incompatible module with a legacy API surface in the hot download path.
- Impact: Progress rendering issues can affect install/update UX and make terminal behavior harder to maintain.
- Migration plan: Replace progress handling in `pkg/assets/assets.go` with a maintained progress library or a smaller internal wrapper.

**`github.com/docker/docker v28.3.2+incompatible`:**
- Risk: Heavy client dependency with broad transitive surface for a single provider.
- Impact: Docker provider breakage or API churn affects `docker://` installs, cleanup, and tag discovery in `pkg/providers/docker.go`.
- Migration plan: Isolate Docker-specific behavior behind a thinner adapter and keep provider tests comprehensive before upgrading.

## Missing Critical Features

**Ephemeral execution flow (`run` command):**
- Problem: The repository backlog still tracks an unimplemented `run` command that should fetch to cache and execute without mutating config.
- Blocks: Disposable one-off execution workflows and a cleaner path for trying tools without persisting them.

**Tracked system-package installs:**
- Problem: The backlog still tracks missing support for package-manager artifacts behind `--system-package`.
- Blocks: Managing `.deb`, `.rpm`, `.apk`, and `flatpak` artifacts while preserving update/remove/ensure behavior in `config.json`.

## Test Coverage Gaps

**Destructive command behavior:**
- What's not tested: End-to-end behavior for `bin remove` and `bin prune`, especially failure ordering and config consistency.
- Files: `cmd/remove.go`, `cmd/prune.go`
- Risk: Refactors can silently introduce orphaned binaries or stale config state.
- Priority: High

**Large-download and archive-memory behavior:**
- What's not tested: Command-level behavior for large assets, nested archives, and partial-download failure cleanup.
- Files: `pkg/assets/assets.go`, `cmd/installer.go`
- Risk: Memory spikes and leftover files can go unnoticed until users hit large real-world releases.
- Priority: High

**Provider integration against live API edge cases:**
- What's not tested: Real provider behaviors such as rate limits, missing checksum assets, odd redirect chains, and nonstandard release naming.
- Files: `pkg/providers/github.go`, `pkg/providers/gitlab.go`, `pkg/providers/codeberg.go`, `pkg/providers/hashicorp.go`, `pkg/providers/generic_url.go`, `pkg/providers/docker.go`
- Risk: Update/install regressions surface only against live upstream repositories.
- Priority: Medium

---

*Concerns audit: 2026-04-10*
