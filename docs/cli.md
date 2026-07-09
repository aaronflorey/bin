# CLI reference

`bin` uses Cobra. If you run it with no arguments, it behaves like `bin list` (`cmd/root.go`).

## Global flags

| Flag | Effect |
| --- | --- |
| `--verbose` / `--debug` | Enable verbose logging. |
| `--log-file <path>` | Write logs to a file. |

## Commands

| Command | Purpose | Notes |
| --- | --- | --- |
| `install` | Install a binary from a release, image, `goinstall://`, or URL | Alias: `i`. Supports `--force`, `--provider`, `--select`, `--all`, `--min-age-days`, `--pin`, `--system-package`, `--prefer-system-package`, `--package-type`, `--non-interactive`. |
| `run` | Download a binary into the user cache and execute it | Supports passthrough args after `--`. Cached files live under `os.UserCacheDir()/bin`. |
| `ensure` | Reinstall tracked binaries when they are missing or mismatched | Alias: `e`. |
| `outdated` | Show tracked binaries with newer versions available | `--format=text|json` (default `text`). |
| `update` | Update one or more tracked binaries | Alias: `u`. Supports `--yes`, `--dry-run`, `--all`, `--parallelism`, `--skip-path-check`, `--continue-on-error`. Defaults to `--continue-on-error=true`: later binaries still run after a per-binary failure, but the command exits with code `4` if any update failed. Use `--continue-on-error=false` to stop on the first per-binary failure. |
| `set-config` | Update supported config keys | Only `default_path` and `use_gh_for_github_token`. |
| `export` | Write managed binaries as JSON | Writes to stdout unless a file is passed. |
| `import` | Read managed binaries from JSON | `--skip-ensure` skips the post-import ensure step. |
| `pin` / `unpin` | Toggle update pinning | Accept binary names or paths. |
| `remove` | Uninstall tracked binaries | Aliases: `rm`, `r`, `uninstall`. With no args, opens an interactive picker. `--yes` is required for system-package removals in non-interactive mode. |
| `list` | List tracked binaries | Alias: `ls`. `--format=table|json`. |
| `prune` | Remove config entries whose binaries no longer exist | `--force` skips the confirmation prompt. |
| `version` | Print the `bin` version | Useful for installation checks. |

## Notes

- `install` rejects multiple binaries plus custom paths; custom paths are only valid for a single target (`cmd/install.go`).
- On macOS, GUI app releases that ship a `.dmg` should be installed with `--system-package --package-type dmg`, for example `bin install --system-package --package-type dmg github.com/getpaseo/paseo Paseo`; binary mode intentionally ignores Windows `.exe` installers on non-Windows platforms.
- `update` requires `--yes` or `--dry-run` in non-interactive mode when updates are available (`cmd/update.go`).
- `update` pre/post hooks are global blockers: if either hook fails, the command stops instead of continuing per binary (`cmd/update.go`).
- `remove` without arguments requires an interactive terminal (`cmd/remove.go`).
- `--package-type flatpack` is normalized to `flatpak` (`pkg/systempackage/systempackage.go`).
