# Architecture

**Analysis Date:** 2026-04-10

## Pattern Overview

**Overall:** Cobra-based command application with a thin command layer over shared package services.

**Key Characteristics:**
- Commands in `cmd/` own CLI parsing, flag handling, and user-facing output.
- Shared packages in `pkg/` own persistence, provider integration, asset resolution, prompts, and terminal helpers.
- Most command flows converge on shared orchestration helpers such as `cmd/install.go`, `cmd/update.go`, and `cmd/installer.go` instead of duplicating fetch/install logic.

## Layers

**CLI entry layer:**
- Purpose: Start the binary and dispatch CLI arguments.
- Location: `main.go`, `cmd/root.go`
- Contains: build metadata assembly, root command setup, default-command behavior, persistent flags, spinner/logger bootstrapping.
- Depends on: `cmd/`, `pkg/config`, `pkg/spinner`, Cobra, logging.
- Used by: direct CLI execution of `bin`.

**Command layer:**
- Purpose: Implement each user command as a Cobra subcommand.
- Location: `cmd/*.go`
- Contains: `install`, `update`, `ensure`, `remove`, `list`, `export`, `import`, `pin`, `unpin`, `prune`, `set-config`, and `version` command handlers.
- Depends on: `pkg/config`, `pkg/providers`, `pkg/assets`, `pkg/prompt`, `pkg/spinner`.
- Used by: `cmd/root.go` through `newInstallCmd()`, `newUpdateCmd()`, `newEnsureCmd()`, and related constructors.

**Installation orchestration layer:**
- Purpose: Reuse the same fetch/save/config-update workflow across install-like operations.
- Location: `cmd/installer.go`, `cmd/install.go`, `cmd/update.go`, `cmd/ensure.go`
- Contains: `InstallOpts`, `installBinary`, path resolution, overwrite checks, release-age checks, duplicate-hash warnings, target resolution.
- Depends on: `pkg/providers`, `pkg/config`, `pkg/assets`, `pkg/prompt`.
- Used by: `cmd/install.go`, `cmd/update.go`, `cmd/ensure.go`.

**Configuration/state layer:**
- Purpose: Load, validate, mutate, and persist the local managed-binary registry.
- Location: `pkg/config/config.go`, `pkg/config/config_unix.go`, `pkg/config/config_windows.go`
- Contains: in-memory singleton config state, JSON serialization, default-path discovery, writable-path selection, lifecycle hooks, OS/arch helpers.
- Depends on: filesystem, process environment, `pkg/options`, logging.
- Used by: every command through `config.CheckAndLoad()`, `config.Get()`, `config.UpsertBinary()`, and `config.RemoveBinaries()`.

**Provider abstraction layer:**
- Purpose: Normalize repository/image URLs and fetch binaries or version metadata from external sources.
- Location: `pkg/providers/providers.go`, `pkg/providers/github.go`, `pkg/providers/gitlab.go`, `pkg/providers/codeberg.go`, `pkg/providers/hashicorp.go`, `pkg/providers/docker.go`, `pkg/providers/goinstall.go`, `pkg/providers/generic_url.go`
- Contains: `Provider` interface, provider factory, provider-specific API clients, cleanup hooks, release-age metadata.
- Depends on: HTTP clients, Docker client, provider SDKs, `pkg/assets`.
- Used by: `cmd/install.go`, `cmd/update.go`, `cmd/ensure.go`, `cmd/update_discovery.go`.

**Asset selection and extraction layer:**
- Purpose: Score, filter, download, and unpack release assets into a single executable payload.
- Location: `pkg/assets/assets.go`
- Contains: platform-aware scoring, archive inspection, file extraction, checksum asset handling, HTTP download helpers.
- Depends on: `pkg/config` for runtime platform data, `pkg/options` for interactive selection, archive/filetype libraries.
- Used by: provider implementations such as `pkg/providers/github.go` and `pkg/providers/gitlab.go`.

**Terminal interaction layer:**
- Purpose: Handle prompts, menu selection, and spinner-aware terminal output.
- Location: `pkg/prompt/prompt.go`, `pkg/options/options.go`, `pkg/spinner/spinner.go`, `cmd/spinner.go`
- Contains: confirmation prompts, interactive selection, terminal detection, spinner pausing/resuming.
- Depends on: stdin/stdout, terminal detection packages.
- Used by: install/update flows and path selection during config bootstrap.

**Automation entry layer:**
- Purpose: Reuse repository behavior outside the main Go CLI.
- Location: `action.yml`, `action/setup.js`, `action/install.js`, `install.sh`
- Contains: GitHub Action wrapper, release bootstrap installer, CI-oriented batch install flow.
- Depends on: GitHub Actions runtime, GitHub releases, shell utilities.
- Used by: GitHub Actions consumers and curl-pipe installer usage.

## Data Flow

**CLI command execution:**

1. `main.go` builds the version string and calls `cmd.Execute(...)`.
2. `cmd/root.go` constructs the root Cobra command, registers subcommands, and applies persistent pre-run setup.
3. `cmd/root.go` loads config through `config.CheckAndLoad()` and wires spinner-aware logging before the selected command runs.
4. A command in `cmd/*.go` resolves targets, flags, and user interaction, then delegates to shared helpers or package APIs.

**Install/update/ensure binary flow:**

1. `cmd/install.go`, `cmd/update.go`, or `cmd/ensure.go` resolve the target URL or configured binary records from `config.Get().Bins`.
2. `cmd/installer.go` calls `providers.New(...)` from `pkg/providers/providers.go` to select the provider implementation.
3. The provider fetches release metadata and hands candidate assets to `pkg/assets/assets.go` for filtering and extraction.
4. `cmd/installer.go` writes the final executable to disk, computes the hash, and persists the resulting record with `config.UpsertBinary(...)`.

**Export/import config flow:**

1. `cmd/export.go` reads `config.Get().Bins`, recomputes hashes from disk, and serializes `portableBinary` records.
2. `cmd/import.go` decodes the same portable format, remaps paths onto `config.Get().DefaultPath`, and upserts records with `config.UpsertBinaries(...)`.

**State Management:**
- Runtime state is centralized in the package-level singleton `cfg` inside `pkg/config/config.go`.
- Commands treat config records as the source of truth for managed binaries, keyed by installed path.
- Persisted state lives in the user config file resolved by `pkg/config/config.go`; write operations happen through `write()`, `UpsertBinary()`, `UpsertBinaries()`, and `RemoveBinaries()`.

## Key Abstractions

**Root command wrapper:**
- Purpose: Combine Cobra setup with repository-specific execution behavior.
- Examples: `cmd/root.go`
- Pattern: wrapper struct (`rootCmd`) holding `*cobra.Command`, shared flags, and custom execution/error-exit behavior.

**Command wrapper structs:**
- Purpose: Keep each Cobra command's flags and handler state together.
- Examples: `cmd/install.go`, `cmd/update.go`, `cmd/ensure.go`, `cmd/export.go`
- Pattern: `type <name>Cmd struct { cmd *cobra.Command; opts ... }` plus `new<Name>Cmd()` constructor.

**Install orchestration contract:**
- Purpose: Represent a complete fetch/install/config-write operation as a single reusable function call.
- Examples: `cmd/installer.go`, `cmd/install.go`, `cmd/update.go`, `cmd/ensure.go`
- Pattern: `InstallOpts` input struct + `installBinary()` returning `InstallResult`.

**Provider interface:**
- Purpose: Decouple command workflows from source-specific fetching logic.
- Examples: `pkg/providers/providers.go`, `pkg/providers/github.go`, `pkg/providers/docker.go`
- Pattern: interface with `Fetch`, `GetLatestVersion`, `Cleanup`, and `GetID`, selected through `providers.New(...)`.

**Managed binary record:**
- Purpose: Persist everything needed to reinstall, update, and verify a binary.
- Examples: `pkg/config/config.go`, `cmd/export.go`, `cmd/import.go`
- Pattern: `config.Binary` record keyed by installed path, plus `portableBinary` for import/export serialization.

## Entry Points

**CLI binary:**
- Location: `main.go`
- Triggers: local execution of the compiled `bin` binary.
- Responsibilities: inject build metadata and hand control to `cmd.Execute(...)`.

**Root Cobra command:**
- Location: `cmd/root.go`
- Triggers: every CLI invocation.
- Responsibilities: config bootstrap, default `list` behavior, command registration, spinner/logging setup, centralized exit handling.

**GitHub Action:**
- Location: `action.yml`, `action/setup.js`, `action/install.js`
- Triggers: `uses: aaronflorey/bin@...` in workflow YAML.
- Responsibilities: install a released `bin` binary into the runner, append it to `PATH`, optionally batch-install additional tools.

**Bootstrap installer script:**
- Location: `install.sh`
- Triggers: shell execution such as `sh ./install.sh` or curl-pipe install.
- Responsibilities: detect platform, download the correct release artifact, bootstrap `bin`, initialize config, and install the managed binary into the target directory.

**CI workflows:**
- Location: `.github/workflows/build.yml`, `.github/workflows/release-please.yml`
- Triggers: GitHub push, pull request, and release automation events.
- Responsibilities: lint/test/build verification, installer smoke tests, action smoke tests, and release automation.

## Error Handling

**Strategy:** command handlers return errors upward; `cmd/root.go` translates them into logging plus process exit codes.

**Patterns:**
- `cmd/error.go` defines `exitError` so commands can return non-default exit codes while still using normal Go errors.
- `cmd/root.go` uses `SilenceUsage` and `SilenceErrors` on Cobra commands, then logs failures once in a centralized place.
- Shared helpers such as `cmd/installer.go` wrap lower-level errors with context before returning them.
- Interactive aborts are surfaced as regular errors from `pkg/prompt/prompt.go` and stop the current command flow.

## Cross-Cutting Concerns

**Logging:** `cmd/root.go` configures `caarlos0/log`; spinner-aware logging is applied through `newSpinnerLogger()` and reused across commands and packages.

**Validation:** `cmd/install.go` validates target shape and flag combinations, `cmd/update.go` validates update targets, `pkg/providers/providers.go` validates provider selection, and provider-specific constructors such as `pkg/providers/docker.go` validate source identifiers.

**Authentication:** provider packages read tokens directly from environment variables when needed, especially `pkg/providers/github.go`; the GitHub Action passes tokens through `action/setup.js` and `action/install.js`, and `install.sh` supports GitHub auth tokens for release downloads.

---

*Architecture analysis: 2026-04-10*
