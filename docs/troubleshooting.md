# Troubleshooting

## `error loading config file`

**Symptoms**

- Commands fail before doing any work.

**Likely causes**

- `BIN_CONFIG` points to unreadable data.
- The config file contains invalid JSON.

**Fix**

```bash
bin --debug list
```

Check the config path in [configuration](configuration.md) and repair or remove the file if needed.

## `unsupported config key ...`

**Cause**

Only `default_path` and `use_gh_for_github_token` are supported by `set-config` (`cmd/set_config.go`).

**Fix**

```bash
bin set-config default_path /your/bin/dir
bin set-config use_gh_for_github_token true
```

## Install path not found automatically

**Symptoms**

- `bin` prompts for a download directory.

**Cause**

- No writable `PATH` directory was discovered.

**Fix**

Set `BIN_EXE_DIR` or update `default_path` with `bin set-config default_path ...`.

## `--min-age-days must be a positive integer`

**Cause**

- `install --min-age-days` was set to `0` or a negative value.

**Fix**

Use a positive integer only.

## `unsupported --package-type ...`

**Cause**

- The package type is not one of `apk`, `deb`, `dmg`, `flatpak`, `flatpack`, or `rpm` (`pkg/systempackage/systempackage.go`).

**Fix**

Use one of the supported values.

For macOS GUI app releases that ship `.dmg` assets, use system-package mode explicitly:

```bash
bin install --system-package --package-type dmg github.com/getpaseo/paseo Paseo
```

`bin` intentionally rejects Windows `.exe` installers when you are installing on macOS or Linux.

## `multiple compatible products found`

**Cause**

- The release contains several current-platform tools and `bin` will not choose one alphabetically.

**Fix**

Run with `--all` to inspect compatible choices, then pass the exact asset with `--select`. In CI, always provide `--select` for an ambiguous multi-product release.

## `selected payload ... is not executable`

**Cause**

- The selected release asset resolved to a library, package metadata, source file, or another non-executable payload.

**Fix**

Choose a native binary or binary archive from the release. `--select` does not bypass this validation.

## `update requires --yes or --dry-run in non-interactive mode`

**Cause**

- `bin update` found upgrades, but the terminal is non-interactive.

**Fix**

```bash
bin update --yes
bin update --dry-run
```

## `bin update` finished some updates but still exited non-zero

**Cause**

- `bin update` defaults to `--continue-on-error=true`, so it keeps updating later binaries after a per-binary failure.
- If any update fails during discovery or install, the command still exits with code `4` after finishing the rest.
- Pre-update and post-update hooks are global blockers and can stop the command before or after the per-binary loop.

**Fix**

- Review the warning lines above the final error to identify the specific failing binary.
- Re-run with `--continue-on-error=false` if you want the command to stop at the first per-binary failure.

## `remove without arguments requires an interactive terminal`

**Cause**

- `bin remove` was called with no args in a non-interactive session.

**Fix**

Pass explicit targets or run in an interactive terminal.

## `binary ... is not managed by bin`

**Cause**

- The name/path is present on disk but not in the config map.

**Fix**

Use `bin list` to confirm the managed path, then pass that exact name or path.

## Generic URL version inference fails

**Cause**

- The URL does not expose a semver-like version in `Content-Disposition`, the redirect URL, or the original URL.

**Fix**

Use a filename or redirect target that contains a version token such as `1.2.3`.

## `goinstall://` fails

**Cause**

- `go` is not installed or not on `PATH`.
- `go env GOPATH` fails.
- The module proxy does not return version metadata.

**Fix**

Ensure `go` is installed and try again.

## When to look at source

- `cmd/root.go` for startup and logging errors
- `pkg/config/*` for config path and default path selection
- `pkg/providers/*` for provider-specific fetch failures
- `cmd/install.go`, `cmd/update.go`, `cmd/remove.go` for command validation
