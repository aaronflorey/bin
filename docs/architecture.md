# Architecture

`bin` is organized around a small command layer and provider-specific fetchers.

## Package map

| Path | Responsibility |
| --- | --- |
| `main.go` | Program entry point and version metadata. |
| `cmd/` | Cobra commands, command orchestration, lifecycle hooks, cache handling, and install/update/remove flows. |
| `pkg/config/` | Persistent config file loading, path selection, hooks, and tracked binary metadata. |
| `pkg/providers/` | Provider detection and fetch/update logic for GitHub, GitLab, Codeberg, Docker, HashiCorp, `go install`, and generic URLs. |
| `pkg/assets/` | Asset scoring, filtering, naming, and checksum selection. |
| `pkg/systempackage/` | Package-artifact detection and normalization. |
| `pkg/prompt/` | Interactive prompts and multi-select helpers. |
| `pkg/spinner/` | Terminal spinner UI. |

## Data flow

1. `cmd/root.go` loads config and configures logging.
2. The command resolves a provider in `pkg/providers`.
3. The provider selects an asset or package artifact through `pkg/assets`.
4. The command installs, updates, removes, or runs the binary.
5. The config file is updated with the resolved binary metadata.

## Behavior worth knowing

- Hooks are part of persisted config and are executed around install/update/remove operations.
- `install` and `update` preserve provider metadata so later runs can use the same provider and artifact selection.
- System-package installs can carry extra metadata such as package type and macOS app bundle name so later lifecycle commands still work.
