# Codebase Structure

**Analysis Date:** 2026-04-10

## Directory Layout

```text
bin/
├── cmd/                 # Cobra commands and shared command helpers
├── pkg/                 # Reusable packages for config, providers, assets, prompts, and terminal UX
├── action/              # JavaScript scripts used by the composite GitHub Action
├── .github/workflows/   # CI, smoke tests, and release automation
├── .planning/codebase/  # Generated mapper reference documents
├── dist/                # Build/release artifacts checked into the repository
├── scripts/             # Reserved helper script directory (currently empty)
├── main.go              # CLI process entry point
├── action.yml           # Composite GitHub Action definition
├── install.sh           # Shell bootstrap installer
├── go.mod               # Go module definition
└── Makefile             # Common developer commands
```

## Directory Purposes

**`cmd/`:**
- Purpose: CLI-facing command implementations.
- Contains: one file per command plus shared helpers like `cmd/root.go`, `cmd/installer.go`, `cmd/error.go`, and `cmd/spinner.go`.
- Key files: `cmd/root.go`, `cmd/install.go`, `cmd/update.go`, `cmd/installer.go`, `cmd/ensure.go`.

**`pkg/`:**
- Purpose: Internal packages consumed by `cmd/` and occasionally by other packages.
- Contains: domain-specific helpers grouped by responsibility.
- Key files: `pkg/config/config.go`, `pkg/providers/providers.go`, `pkg/assets/assets.go`, `pkg/prompt/prompt.go`, `pkg/options/options.go`.

**`pkg/config/`:**
- Purpose: Persisted config model and platform-aware config path/default path logic.
- Contains: JSON config loading/writing, binary metadata structs, lifecycle hooks, OS-specific path helpers.
- Key files: `pkg/config/config.go`, `pkg/config/config_unix.go`, `pkg/config/config_windows.go`.

**`pkg/providers/`:**
- Purpose: External source integrations behind a common interface.
- Contains: factory logic, provider implementations, URL normalization, checksum helpers, release-age helpers.
- Key files: `pkg/providers/providers.go`, `pkg/providers/github.go`, `pkg/providers/docker.go`, `pkg/providers/gitlab.go`, `pkg/providers/goinstall.go`.

**`pkg/assets/`:**
- Purpose: Asset scoring, filtering, archive extraction, and payload normalization.
- Contains: the large asset-selection engine in `pkg/assets/assets.go` and its tests.
- Key files: `pkg/assets/assets.go`, `pkg/assets/assets_test.go`.

**`pkg/prompt/`, `pkg/options/`, `pkg/spinner/`:**
- Purpose: Terminal interaction utilities.
- Contains: confirmation prompts, interactive option selection, spinner output coordination.
- Key files: `pkg/prompt/prompt.go`, `pkg/options/options.go`, `pkg/spinner/spinner.go`.

**`pkg/strings/`:**
- Purpose: Small shared string helpers used by asset filtering.
- Contains: low-level string utility functions.
- Key files: `pkg/strings/strings.go`.

**`action/`:**
- Purpose: Implementation for the composite action declared in `action.yml`.
- Contains: setup/install scripts run with Node.js inside GitHub Actions.
- Key files: `action/setup.js`, `action/install.js`.

**`.github/workflows/`:**
- Purpose: Repository automation definitions.
- Contains: build/test/lint workflow and release automation workflow.
- Key files: `.github/workflows/build.yml`, `.github/workflows/release-please.yml`.

**`dist/`:**
- Purpose: Generated release outputs and artifact metadata.
- Contains: platform binaries, archives, checksums, metadata, and release config snapshots.
- Key files: `dist/artifacts.json`, `dist/checksums.txt`, `dist/metadata.json`.

**`.planning/codebase/`:**
- Purpose: Generated codebase reference docs for future planning/execution.
- Contains: mapper outputs such as `ARCHITECTURE.md` and `STRUCTURE.md`.
- Key files: `.planning/codebase/ARCHITECTURE.md`, `.planning/codebase/STRUCTURE.md`.

## Key File Locations

**Entry Points:**
- `main.go`: process entry for the Go CLI.
- `cmd/root.go`: Cobra root command and global pre-run behavior.
- `action.yml`: entry point for the composite GitHub Action.
- `install.sh`: shell bootstrap installer entry point.

**Configuration:**
- `go.mod`: Go module and dependency source of truth.
- `.golangci.yml`: lint configuration.
- `.goreleaser.yml`: cross-platform release packaging.
- `release-please-config.json`: automated release config.
- `.release-please-manifest.json`: current tracked release versions.

**Core Logic:**
- `cmd/install.go`: install command parsing and target resolution.
- `cmd/update.go`: update detection and update execution.
- `cmd/installer.go`: shared install/save/config-write pipeline.
- `pkg/config/config.go`: persistent managed-binary registry.
- `pkg/providers/providers.go`: provider factory and interface.
- `pkg/assets/assets.go`: asset scoring and extraction engine.

**Testing:**
- `cmd/*_test.go`: command-level unit tests beside the command files.
- `pkg/*/*_test.go`: package-level unit tests beside implementation files.
- `.github/workflows/build.yml`: CI smoke tests for installer/action behavior.

## Naming Conventions

**Files:**
- Lowercase snake-style Go filenames grouped by responsibility: `cmd/update_discovery.go`, `pkg/config/config_unix.go`, `pkg/providers/docker_windows.go`.
- Tests sit beside source files with `_test.go` suffix: `cmd/install_test.go`, `pkg/providers/providers_test.go`.
- Platform-specific variants use Go build naming conventions: `pkg/config/config_unix.go`, `pkg/config/config_windows.go`, `pkg/providers/docker_unix.go`, `pkg/providers/docker_windows.go`.

**Directories:**
- Short lowercase package names: `cmd/`, `pkg/config/`, `pkg/providers/`, `pkg/assets/`.
- Non-Go automation directories mirror their runtime context: `action/` for GitHub Action scripts, `.github/workflows/` for GitHub workflows.

## Where to Add New Code

**New CLI feature:**
- Primary code: add a new command file under `cmd/` following the `new<Name>Cmd()` pattern from `cmd/install.go` or `cmd/update.go`.
- Registration: wire the new command into `cmd/root.go` via `cmd.AddCommand(...)`.
- Tests: add `cmd/<name>_test.go` beside the command file.

**New provider/source integration:**
- Implementation: add a provider file under `pkg/providers/`, implement the interface from `pkg/providers/providers.go`, and register it in `providers.New(...)` in `pkg/providers/providers.go`.
- Asset handling reuse: use `pkg/assets/assets.go` instead of adding source-specific extraction logic in `cmd/`.
- Tests: add `pkg/providers/<provider>_test.go` beside the implementation.

**New install/update helper logic:**
- Shared orchestration: place reusable install/update helpers in `cmd/installer.go` or a new focused helper file inside `cmd/` when the logic is command-centric.
- Config mutations: keep persistence changes inside `pkg/config/` rather than spreading JSON writes through commands.

**New reusable utility:**
- Shared helpers: add a focused package under `pkg/` only when the logic is reused across commands or providers.
- Command-only helpers: keep them in `cmd/` if they are only meaningful to CLI orchestration.

**New automation behavior:**
- GitHub Action changes: edit `action.yml` and `action/*.js`.
- Shell bootstrap behavior: edit `install.sh`.
- CI/release pipeline changes: edit `.github/workflows/*.yml`, `.goreleaser.yml`, or `release-please-config.json`.

## Special Directories

**`dist/`:**
- Purpose: packaged release outputs and metadata snapshots.
- Generated: Yes.
- Committed: Yes.

**`.github/workflows/`:**
- Purpose: repository automation definitions.
- Generated: No.
- Committed: Yes.

**`.planning/codebase/`:**
- Purpose: generated planning reference documents.
- Generated: Yes.
- Committed: Intended to be committed as planning artifacts.

**`scripts/`:**
- Purpose: repository helper scripts.
- Generated: No.
- Committed: Yes.
- Current state: directory exists but is empty.

---

*Structure analysis: 2026-04-10*
