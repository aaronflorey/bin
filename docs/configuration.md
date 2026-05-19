# Configuration

`bin` stores state in a JSON config file and also reads a small set of environment variables at runtime.

## Config file path

The config path is resolved in this order:

1. `BIN_CONFIG` if it is set (`pkg/config/config.go`).
2. Legacy `~/.bin/config.json` if it already exists (`pkg/config/config_unix.go`, `pkg/config/config_windows.go`).
3. Unix: `XDG_CONFIG_HOME/bin/config.json` when `XDG_CONFIG_HOME` points to an existing path, otherwise `~/.config/bin/config.json`, otherwise `~/.bin/config.json` (`pkg/config/config_unix.go`).
4. Windows: `%APPDATA%/bin/config.json`, otherwise `~/.bin/config.json` (`pkg/config/config_windows.go`).

The file is created automatically if missing when `bin` starts (`CheckAndLoad`). Invalid JSON aborts startup with `error loading config file: ...` (`cmd/root.go`, `pkg/config/config.go`).

## Config schema

Defined in `pkg/config/config.go`:

| Key | Type | Notes |
| --- | --- | --- |
| `default_path` | string | Default install directory. |
| `default_chmod` | string | Unix default is `0755` when unset (`pkg/config/config.go`). |
| `use_gh_for_github_token` | bool | Enables `gh auth token` fallback for GitHub.com requests when no GitHub token env var is set. |
| `bins` | object map | Managed binaries keyed by config path. |
| `hooks` | array | Lifecycle hooks (`pre-install`, `post-install`, `pre-update`, `post-update`, `pre-remove`, `post-remove`). |

Hooks are executed directly with `exec.Command`, so shell features are not expanded unless you invoke a shell yourself (`pkg/config/config.go`).

### Example

```json
{
  "default_path": "/home/me/.local/bin",
  "default_chmod": "0755",
  "use_gh_for_github_token": true,
  "bins": {
    "/home/me/.local/bin/jq": {
      "path": "/home/me/.local/bin/jq",
      "remote_name": "jq",
      "version": "v1.7.1",
      "hash": "...",
      "url": "https://github.com/jqlang/jq",
      "provider": "github"
    }
  },
  "hooks": [
    {
      "type": "pre-install",
      "command": "sh",
      "args": ["-c", "echo installing"]
    }
  ]
}
```

## Environment variables

### Core runtime variables

| Variable | Effect |
| --- | --- |
| `BIN_CONFIG` | Overrides the config file path. |
| `BIN_EXE_DIR` | Forces the install directory and creates it if needed. If creation fails, `bin` falls back to normal path selection. |
| `XDG_CONFIG_HOME` | Unix config path base. |
| `APPDATA` | Windows config path base. |
| `PATH` | Used to discover a writable default install directory when `default_path` is not set. |

### Configurable behavior

`bin set-config` only accepts:

- `default_path`
- `use_gh_for_github_token`

Invalid keys fail with `unsupported config key ...` (`cmd/set_config.go`, `pkg/config/config.go`). Boolean parsing for `use_gh_for_github_token` uses Go boolean parsing and rejects invalid values.

## Precedence

When `default_path` is missing, `CheckAndLoad` uses:

1. `BIN_EXE_DIR`
2. Unix: `~/.local/bin` when it can be prepared; otherwise the first writable directory under `PATH`. Windows: the first writable directory under `PATH`.
3. an interactive prompt to choose a writable PATH directory if automatic discovery fails

## Related source

- `pkg/config/config.go`
- `pkg/config/config_unix.go`
- `pkg/config/config_windows.go`
- `cmd/set_config.go`
