# AGENTS.md

## Project overview

`bin` is a cross-platform Go 1.24 CLI for installing and managing binaries from release providers, Docker images, `go install`, generic URLs, and selected system-package artifacts. Cobra commands orchestrate provider lookup, asset selection, installation lifecycle, and persistent JSON config. The repository also ships a dependency-free Node 20 composite GitHub Action and a POSIX installer script.

## Source of truth

- Tool versions and local tasks: `mise.toml`
- Lint policy: `.golangci.yml`
- Git hooks: `hk.pkl`
- CI and smoke tests: `.github/workflows/build.yml`
- Release pipeline: `.github/workflows/release.yaml`, `release-please-config.json`, `.goreleaser.yml`
- Runtime behavior: `docs/architecture.md`, `docs/cli.md`, `docs/configuration.md`
- Test workflow: `docs/testing.md`

## Commands

Use `mise` tasks for repository-wide workflows; they encode the intended commands and tool versions.

| Purpose | Command | When to run |
| --- | --- | --- |
| Install Go 1.24 and `hk` | `mise install` | First checkout or tool-version changes |
| Download modules | `mise run download` | First checkout or dependency changes |
| Build CLI | `mise run build` | After command or packaging changes |
| Run all Go tests | `mise run test` | Before final handoff |
| Run one Go package | `go test ./cmd` or `go test ./pkg/providers` | While iterating in that package |
| Run one Go test | `go test ./cmd -run '^TestName$'` | Focused iteration |
| Run race detector | `mise run test-race` | Concurrency, config locking, or final CI-level validation |
| Run Action tests | `node --test action/setup.test.js` | After changes under `action/` or `action.yml` |
| Check format and vet | `mise run lint` | After Go edits |
| Full non-mutating verification | `mise run verify` | Before final handoff; includes module, format, and golangci-lint checks |
| Apply format and module fixes | `mise run fix` | Only when checks report drift |
| Coverage | `mise run coverage` | When coverage inspection is useful; writes `coverage.out` |
| Install Git hooks | `mise run hooks` | Once per clone |

`mise run check` runs formatting checks, `go vet`, and Go tests, but not `golangci-lint` or module-tidy validation. `mise run verify` runs dependency download, formatting check, `go mod tidy -diff`, and `golangci-lint`, but not tests. Run both tests and verification for complete local validation.

CI additionally exercises `install.sh` and live provider/system-package installs. Those jobs require network access, GitHub tokens, and sometimes `sudo`; do not treat them as routine local unit tests.

## Architecture and control flow

- `main.go` supplies build metadata and delegates all execution to `cmd.Execute`.
- `cmd/` owns the Cobra surface and orchestration. `cmd/root.go` configures logging, loads config before all commands except `version`, and starts the spinner. Command files execute lifecycle hooks and delegate provider/asset/system-package work.
- `pkg/providers/` detects sources and implements a shared `Provider` contract (`Fetch`, latest-version lookup, cleanup, ID). Release-history support is optional through `ReleaseHistoryProvider`.
- `pkg/assets/` performs platform filtering, product ranking, archive extraction, checksum handling, and executable validation. Keep provider transport concerns out of this package and asset-selection concerns out of command handlers.
- `pkg/config/` owns config-path resolution, locking, JSON persistence, lifecycle hooks, and stored binary metadata.
- `pkg/systempackage/` normalizes package types; command-side lifecycle strategies in `cmd/install_mode.go` distinguish direct binaries from system packages.
- `pkg/prompt/`, `pkg/options/`, and `pkg/spinner/` isolate terminal interaction.
- `action.yml` invokes `action/setup.js` to install `bin`, then `action/install.js` for requested tools. These scripts intentionally use only Node built-ins.

Typical install flow: root loads config, the command normalizes the request, provider detection fetches release metadata, asset filtering resolves a compatible logical product, an install-mode lifecycle strategy installs it, and config persists provider, archive path, release lane, pinning, and package metadata for future `update`, `ensure`, and `remove` calls.

## Code conventions

- Follow standard `gofmt -s` formatting and package-local naming patterns. Lint intentionally permits capitalized error text and legacy initialisms such as `Url`; do not rename established APIs solely to satisfy generic Go style advice.
- Cobra constructors are named `new<Name>Cmd`, return a small wrapper containing `*cobra.Command` and options, use `RunE`, and set `SilenceUsage`/`SilenceErrors`. Register new top-level commands in `cmd/newRootCmd`.
- Return errors to the root command rather than printing and exiting inside command handlers. Root maps `exitError` values to specific exit codes and logs failures once.
- Use `github.com/caarlos0/log` for runtime diagnostics. Preserve spinner-aware stdout/stderr behavior when changing command output or logging.
- Provider-specific behavior belongs behind the interfaces in `pkg/providers/providers.go`. Provider errors used for branching are sentinel errors wrapped with `%w` and checked with `errors.Is`.
- Persist all information needed for later lifecycle operations in `config.Binary`; install/update/ensure/remove must agree on fields such as `InstallMode`, `PackageType`, `AppBundle`, `PackagePath`, and `ReleaseTagPrefix`.
- Platform-specific behavior uses `_unix.go` and `_windows.go` files. Keep Linux/macOS and Windows semantics covered when changing paths, locking, executable handling, or process invocation.

## Testing patterns

- Go tests are colocated as `*_test.go` and use the standard `testing` package, table tests where appropriate, `t.TempDir`, `t.Setenv`, `t.Cleanup`, and `httptest` rather than external services.
- Command tests share `setupTestConfig` from `cmd/export_import_test.go`. It points `BIN_CONFIG` at a temporary file and resets the package-global config; use it before command tests that load or mutate config to avoid touching user state.
- External effects are commonly exposed through package variables (`installProviderFactory`, `lifecycleRegistry`, prompt functions, `execCommand`, `lookPath`, `runGHAuthToken`). Replace them in tests and restore them with `defer` or `t.Cleanup`.
- Global config and injectable package variables make many tests unsafe for `t.Parallel`; follow the surrounding package before introducing parallel tests.
- Asset/provider tests should use local `httptest` servers and in-memory archives. Test both success and rejection paths because downloaded payloads must never be trusted based on filenames alone.
- Action tests synthesize tar/zip fixtures and mock HTTPS behavior in `action/setup.test.js`; keep archive and redirect security cases covered when changing extraction or downloads.

## State, security, and compatibility gotchas

- Never run the built CLI casually against the developer's normal environment during tests. Most commands load or create config and may install/remove files. Isolate manual runs with temporary `BIN_CONFIG`, `BIN_EXE_DIR`, and `HOME` values.
- Config contains executable lifecycle hooks, is locked during mutation, and is written as `0600` on non-Windows systems. Preserve atomic/locked mutation paths instead of writing JSON directly.
- Missing `use_gh_for_github_token` means `true`, while explicit `false` must remain false. Config loading tracks key presence to distinguish these cases.
- Hooks use `exec.Command(command, args...)`; shell expansion occurs only when the configured command explicitly invokes a shell. Hook failures block the surrounding lifecycle operation.
- Generic plain HTTP downloads are rejected unless `--allow-insecure-http` opts in. Do not bypass `newProviderWithPolicy` when creating providers from command code.
- Provider and Action download code deliberately restricts authentication across redirects and refuses insecure redirect targets. Preserve these boundaries when changing HTTP handling.
- Asset selection must preserve non-interactive safety: ambiguous products or release lanes fail instead of silently choosing. `--select` bypasses ranking, not platform or executable validation.
- Explicit release URLs and discovered release-tag prefixes represent release lanes. Preserve `ReleaseTagPrefix` through install/update/ensure flows so multi-track repositories do not jump lanes.
- `install --prefer-system-package` falls back only on compatibility errors, not arbitrary failures. System-package installs have different path and removal semantics; macOS DMG installs persist the app bundle name.
- `update` defaults to continuing after per-binary failures and exits with code 4 if any failed; pre/post update hooks remain global blockers. Non-interactive updates require `--yes` or `--dry-run` when updates exist.
- A no-argument invocation behaves as `bin list`; unknown first arguments are errors rather than implicit install targets.

## Dependencies, generated output, and releases

- Manage dependencies with Go modules and commit both `go.mod` and `go.sum` changes. Use `mise run mod-check` to verify tidy state without rewriting files.
- `coverage.out`, `bin`, `dist/`, and `__debug_bin` are local outputs and ignored. Release archives are generated by GoReleaser; do not edit `dist/` artifacts.
- `main.go` contains `go:generate` directives that install lint tools, but the supported daily workflow is the `mise` task set; `go generate` is not required for normal source changes.
- Releases are driven by Conventional Commits through release-please. The release workflow builds tagged `darwin`, `linux`, and `windows` binaries for `amd64` and `arm64`, with `CGO_ENABLED=0`, then publishes archives/checksums and updates the Homebrew tap.
- Changes to `action.yml`, `action/setup.js`, or `action/install.js` require Node 20 compatibility and `node --test action/setup.test.js`; there is no npm package or dependency-install step.

## Change-aware validation

1. Inspect `git status --short` and select the smallest relevant package or named test.
2. After Go changes, run the focused test, then `mise run lint`.
3. After config locking, shared state, concurrent update, or filesystem changes, add `mise run test-race`.
4. After Action changes, run `node --test action/setup.test.js` in addition to Go checks if Go code also changed.
5. Before final handoff, run `mise run test` and `mise run verify`; add `mise run build` for CLI/build-metadata changes.
6. Do not run deploy/release commands or system-package/live-install smoke tests unless explicitly required and an isolated environment is available.
