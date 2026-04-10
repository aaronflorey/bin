# Coding Conventions

**Analysis Date:** 2026-04-10

## Naming Patterns

**Files:**
- Use lowercase file names with underscores only for OS/test variants: `main.go`, `cmd/install.go`, `pkg/providers/generic_url.go`, `pkg/config/config_unix.go`, `pkg/assets/assets_test.go`.
- Keep package directories lowercase and singular by concern: `cmd/`, `pkg/config/`, `pkg/providers/`, `pkg/prompt/`.

**Functions:**
- Use `CamelCase` for exported APIs and `lowerCamelCase` for package-private helpers: `cmd.Execute`, `config.CheckAndLoad`, `providers.NormalizeGitHubURL`, `parseInstallTargets`, `existingBinaryForInstall`.
- Cobra commands follow `newXxxCmd()` constructors returning a private command wrapper struct: `cmd/newInstallCmd()` in `cmd/install.go`, `cmd/newUpdateCmd()` in `cmd/update.go`, `cmd/newVersionCmd()` in `cmd/version.go`.
- Receiver methods use short receiver names like `cmd *rootCmd`, `root *installCmd`, `f *Filter`: `cmd/root.go`, `cmd/install.go`, `pkg/assets/assets.go`.

**Variables:**
- Local variables are short and contextual when scope is small: `u`, `cfg`, `res`, `err`, `purl` in `cmd/install.go` and `pkg/providers/providers.go`.
- Struct fields are descriptive and domain-specific: `installOpts.minAgeDays` in `cmd/install.go`, `providers.FetchOpts.NonInteractive` in `pkg/providers/providers.go`, `config.Binary.PackagePath` in `pkg/config/config.go`.
- Booleans read as state flags: `force`, `all`, `pin`, `nonInteractive` in `cmd/install.go`; `returnNilRelease` in `cmd/update_test.go`.

**Types:**
- Exported domain types use singular nouns: `config.Binary`, `config.RunHook`, `providers.File`, `providers.ReleaseInfo`, `assets.FilterOpts`.
- Command wrappers and option structs stay unexported and colocated: `installCmd`, `installOpts`, `rootCmd` in `cmd/install.go` and `cmd/root.go`.
- Interface names are concise and behavior-based: `providers.Provider` in `pkg/providers/providers.go`, `assets.platformResolver` in `pkg/assets/assets.go`.

## Code Style

**Formatting:**
- Use `gofmt` formatting with tabs; repository automation runs `go fmt ./...` and `gofmt -w -s ./.` in `Makefile`.
- Keep struct literals and command definitions vertically aligned for readability, as in `cmd/install.go` and `cmd/root.go`.
- Prefer early returns over nested `else` blocks, especially in command handlers and provider factories: `cmd/root.go`, `cmd/install.go`, `pkg/providers/providers.go`.

**Linting:**
- Linting is driven by `golangci-lint` via `.golangci.yml` and `Makefile` target `verify`.
- Staticcheck runs with `all` checks enabled, but `ST1005` and `ST1003` are disabled in `.golangci.yml`, so capitalized error strings and names like `Url` are tolerated when already established.
- Targeted `// nolint` comments are accepted for repeated Cobra boilerplate and intentional globals: `cmd/install.go`, `cmd/update.go`, `cmd/list.go`, `main.go`, `cmd/error.go`.

## Import Organization

**Order:**
1. Go standard library imports
2. Internal project imports under `github.com/aaronflorey/bin/...`
3. External third-party imports

**Path Aliases:**
- No global path alias system is used.
- Local aliases are used only to avoid package-name collisions, for example `bstrings "github.com/aaronflorey/bin/pkg/strings"` in `pkg/assets/assets.go`.

## Error Handling

**Patterns:**
- Return `error` values directly and check them immediately with `if err != nil { return err }`: `cmd/install.go`, `pkg/providers/providers.go`, `pkg/config/config.go`.
- Build user-facing validation errors with `fmt.Errorf(...)`: `cmd/install.go` rejects invalid `--min-age-days`; `pkg/options/options.go` returns `interactive selection required`.
- Use sentinel errors when callers need to branch on type: `providers.ErrInvalidProvider` in `pkg/providers/providers.go`, `config.ErrInvalidConfigKey` in `pkg/config/config.go`.
- Wrap lower-level failures with context using `%w` only where callers may inspect the wrapped error, as in `config.Set()` in `pkg/config/config.go`.
- CLI execution centralizes exit handling in `cmd/root.go`: command errors are converted into log messages and exit codes; special handling exists for `*exitError` from `cmd/error.go`.
- Fatal process termination is rare and reserved for unrecoverable startup failures, e.g. `log.Fatalf` after `config.CheckAndLoad()` in `cmd/root.go`.

## Logging

**Framework:** `github.com/caarlos0/log`

**Patterns:**
- Use `log.Infof` and `log.Debugf` for operational CLI feedback and trace output in `cmd/install.go`, `pkg/config/config.go`, and `pkg/assets/assets.go`.
- Configure debug logging from the root command flag in `cmd/root.go`; logging level changes happen during `PersistentPreRun`.
- Route logs through spinner-aware output when the spinner is enabled in `cmd/root.go` via `newSpinnerLogger()`.
- Use Cobra output streams for command output that should be machine-readable or capturable in tests, as seen in `cmd/export.go`, `cmd/import.go`, and tests in `cmd/export_import_test.go`.

## Comments

**When to Comment:**
- Add comments where behavior is non-obvious, especially around lifecycle hooks, platform handling, and archive-selection heuristics: `pkg/config/config.go`, `pkg/assets/assets.go`, `cmd/installer.go`.
- Keep inline comments short and intent-focused, such as the non-interactive pinning note in `cmd/install.go` and special-case Cobra completion handling in `cmd/root.go`.

**JSDoc/TSDoc:**
- Not applicable; this repository is Go-only.
- Use Go doc comments for exported types and functions that form package APIs: `config.HookType`, `config.RunHook`, `config.GetHooks`, `providers.NormalizeGitHubURL`, `assets.Filter.ParseAutoSelection`.
- Unexported helpers usually omit comments unless the logic is subtle.

## Function Design

**Size:**
- Keep orchestration in medium-sized functions that delegate to helpers. Examples: `(*installCmd).installTarget()` in `cmd/install.go` and `Filter.FilterAssets()` in `pkg/assets/assets.go`.
- Small focused helpers are preferred for reusable checks and parsing: `looksLikeInstallURL()` in `cmd/install.go`, `defaultCommand()` in `cmd/root.go`, `filenameFromContentDisposition()` in `pkg/providers/generic_url.go`.

**Parameters:**
- Group flags and mutable command settings into option structs instead of long parameter lists: `installOpts` in `cmd/install.go`, `providers.FetchOpts` in `pkg/providers/providers.go`, `assets.FilterOpts` in `pkg/assets/assets.go`.
- Pass pointers for richer domain objects when mutation or optional data matters: `*config.Binary`, `*providers.ReleaseInfo`, `*providers.File`.

**Return Values:**
- Multi-value returns are common for parse/resolve helpers: `parseInstallTargets()` in `cmd/install.go`, `resolveSpinnerCommand()` in `cmd/root.go`, provider constructors in `pkg/providers/`.
- Prefer explicit structs for richer results instead of large tuples: `InstallResult` in `cmd/installer.go`, `ReleaseInfo` in `pkg/providers/providers.go`.

## Module Design

**Exports:**
- Export identifiers only when they are consumed across packages. Internal command wiring stays package-private in `cmd/`; shared domain APIs are exported from `pkg/config/`, `pkg/providers/`, and `pkg/assets/`.
- Package state is allowed where it simplifies CLI runtime behavior, but it is kept scoped to a package and wrapped by getters/setters or command entrypoints: `cfg` in `pkg/config/config.go`, `stdin` in `pkg/prompt/prompt.go`, `resolver` in `pkg/assets/assets.go`.

**Barrel Files:**
- Barrel files are not used.
- Each package exposes functionality through normal Go package exports from concrete files such as `pkg/config/config.go`, `pkg/providers/providers.go`, and `pkg/assets/assets.go`.

---

*Convention analysis: 2026-04-10*
