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
| `install` | Install a binary from a release, image, `goinstall://`, or URL | Alias: `i`. Supports `--force`, `--provider`, `--select`, `--all`, `--min-age-days`, `--pin`, `--system-package`, `--prefer-system-package`, `--package-type`, `--non-interactive`, and `--completions=bash|zsh|fish|off`. |
| `run` | Download a binary into the user cache and execute it | Supports passthrough args after `--`. Cached files live under `os.UserCacheDir()/bin`; provider versions remain cache identities even when encoded for safe filenames. |
| `ensure` | Reinstall tracked binaries when they are missing or mismatched | Alias: `e`. |
| `outdated` | Show tracked binaries with newer versions available | `--format=text|json` (default `text`). |
| `update` | Update one or more tracked binaries | Alias: `u`. Supports `--yes`, `--dry-run`, `--all`, `--select`, `--parallelism`, `--skip-path-check`, `--continue-on-error`. `--select` requires exactly one update target and explicitly chooses its release asset; persisted member and payload validation still apply. Defaults to `--continue-on-error=true`: later binaries still run after a per-binary failure, but the command exits with code `4` if any update failed. Use `--continue-on-error=false` to stop on the first per-binary failure. |
| `set-config` | Update supported config keys | Only `default_path` and `use_gh_for_github_token`. |
| `export` | Write managed binaries as JSON | Writes to stdout unless a file is passed. |
| `import` | Read managed binaries from JSON | `--skip-ensure` skips the post-import ensure step. |
| `pin` / `unpin` | Toggle update pinning | Accept binary names or paths. |
| `remove` | Uninstall tracked binaries | Aliases: `rm`, `r`, `uninstall`. With no args, opens an interactive picker. `--yes` is required for system-package removals in non-interactive mode. |
| `list` | List tracked binaries | Alias: `ls`. `--format=table|json`. |
| `prune` | Remove config entries whose binaries no longer exist | `--force` skips the confirmation prompt. |
| `completions sync <binary> <shell> [-- <generator-args...>]` | Install or refresh a completion for an existing managed direct binary | `<shell>` is exactly `bash`, `zsh`, or `fish`. |
| `completion <shell>` | Generate a completion script for `bin` itself | This is Cobra's built-in singular command, not managed-tool completion sync. |
| `inspect` | Resolve a supported release or generic-URL source read-only and report artifact decisions | Requires `--json`, which is the only supported output and writes the stable automation report to stdout. Never prompts, runs no hooks, package managers, or completion work, and rejects effectful Docker, Go-install, and forced non-release providers without creating config, cache, log, or install state. |
| `version` | Print the `bin` version | Useful for installation checks. |

## Notes

- `install` rejects multiple binaries plus custom paths; custom paths are only valid for a single target (`cmd/install.go`).
- Environment variables expand in explicit install destinations only; provider-derived names remain literal portable filename components.
- `install --all` shows all compatible current-platform candidates but still excludes unsupported artifact shapes and does not bypass payload validation.
- `install --select <token>` chooses an exact compatible asset before product ranking. It does not bypass platform or executable validation.
- Ambiguous products prompt interactively; ambiguous release lanes prompt for one lane. Both fail in non-interactive mode rather than selecting alphabetically.
- On macOS, GUI app releases that ship a `.dmg` should be installed with `--system-package --package-type dmg`, for example `bin install --system-package --package-type dmg github.com/getpaseo/paseo Paseo`; binary mode intentionally ignores Windows `.exe` installers on non-Windows platforms.
- `update` requires `--yes` or `--dry-run` in non-interactive mode when updates are available (`cmd/update.go`).
- `update` pre/post hooks are global blockers: if either hook fails, the command stops instead of continuing per binary (`cmd/update.go`).
- `remove` without arguments requires an interactive terminal (`cmd/remove.go`).
- `--package-type flatpack` is normalized to `flatpak` (`pkg/systempackage/systempackage.go`).

## Managed tool completions

`bin completions sync` manages a completion for one already-installed direct
binary. It does not install the binary and does not support system-package
records. For example, after installing a managed command named `tool`:

```bash
bin completions sync tool bash
```

With no generator arguments, `bin` first uses one exact bundled completion when
available, then invokes the installed executable as `tool completion bash`.
The success output names the file and the relevant setup direction, for example:

```text
Installed completion: /absolute/path/to/completions/bash/tool
Bash: source this file, or add /absolute/path/to/completions/bash to your existing completion setup.
```

To use a command shape the tool supports instead, put the arguments after `--`:

```bash
bin completions sync tool fish -- completion fish
bin completions sync tool zsh -- generate-completion zsh
```

Those trailing values are passed as the installed executable's argv, not parsed
as a shell command, and are not saved. An explicit argv invocation bypasses
bundled discovery. This is also the way to sync a renamed installed command:

```bash
bin completions sync my-tool fish -- completion fish
```

The managed files always live beside the active config file, not in a
shell-managed or package-manager directory. If the active config is
`/home/me/.config/bin/config.json`, the destinations for a command named
`tool` are:

```text
/home/me/.config/bin/completions/bash/tool
/home/me/.config/bin/completions/zsh/_tool
/home/me/.config/bin/completions/fish/tool.fish
```

The command prints the actual destination, which is the path to use below. It
does not edit startup files or load the completion into the current shell.

### Shell startup setup

Add one of these to your own shell startup setup, substituting the directory
printed by `bin completions sync`:

```bash
# Bash: after any bash-completion initialization, or source the printed file directly.
source "/absolute/path/to/completions/bash/tool"
```

```zsh
# Zsh: this must happen before compinit.
fpath=("/absolute/path/to/completions/zsh" $fpath)
autoload -Uz compinit
compinit
```

```fish
# Fish
set -gx fish_complete_path "/absolute/path/to/completions/fish" $fish_complete_path
```

Zsh completion files use the normal first-line registration convention. A Zsh
script intended for `_tool` must begin with `#compdef tool`; `bin` preserves
bundled or generated text and never adds or rewrites that line.

### Automatic refresh and supported discovery

Automatic refresh is off for a new install. Opt in per direct binary with the
shell to refresh after successful installs and updates:

```bash
bin install --completions=bash github.com/example/tool
```

On a later reinstall, omitting `--completions` preserves that local choice.
Disable future automatic refresh explicitly with:

```bash
bin install --completions=off github.com/example/tool
```

An explicit `completions sync` does not change the automatic choice. An actual
reinstall by `ensure` follows the same opt-in behavior; an unchanged binary
does no completion work. Automatic refresh failures are warnings only: the
binary install or update remains successful and keeps its normal exit status.

Bundled discovery is intentionally narrow. It considers only regular entries
in the selected executable's archive scope and accepts one exact match beneath
a `completions`, `autocomplete`, or `complete` directory:

| Shell | Accepted bundled path after that directory |
| --- | --- |
| Bash | `<command>.bash` or `bash/<command>` |
| Zsh | `_<command>` |
| Fish | `<command>.fish` |

Duplicate, malformed, or approximate names are skipped rather than guessed.
`bin` does not rewrite registrations, probe alternate generator commands, or
infer a completion framework. Default discovery and default native generation
are unavailable for a renamed command; automatic refresh skips it. Use explicit
generator argv when the renamed executable supports it. A nonzero generator,
empty/non-text output, or an unsupported tool leaves the existing managed
completion unchanged and reports an error for explicit sync (or a warning for
automatic refresh).

### Trust and repair

Sync authorizes completion code from that installed, managed tool and, when an
exact bundled match is used, from its authorized installed release. Treat that
code as shell code you choose to load. Generation runs the verified installed
executable by absolute path with direct argv, a five-second deadline, and caps
of 1 MiB stdout and 64 KiB stderr. These limits are not a sandbox or a promise
that every descendant process is contained or killed.

`bin` replaces only completion files it recorded as its own and whose contents
still match the recorded digest. If saving that ownership record fails after a
file was published, `bin` reports the exact path; remove that published,
unrecorded file manually before retrying:

```bash
rm -- "/absolute/path/reported-by-bin"
bin completions sync tool bash
```

`remove` and `prune` make a best-effort cleanup of unchanged, recorded files.
They still remove the binary/config record if completion cleanup fails and warn
with every leftover path. Inspect the reported path and remove it manually when
appropriate; `bin` will not delete a modified, unrecorded, conflicting, or
unsafe destination.
