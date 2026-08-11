# Architecture

`bin` is organized around a small command layer and provider-specific fetchers.

## Package map

| Path | Responsibility |
| --- | --- |
| `main.go` | Program entry point and version metadata. |
| `cmd/` | Cobra commands, command orchestration, lifecycle hooks, cache handling, and install/update/remove flows. |
| `pkg/config/` | Persistent config file loading, path selection, hooks, and tracked binary metadata. |
| `pkg/providers/` | Provider detection and fetch/update logic for GitHub, GitLab, Codeberg, Docker, HashiCorp, `go install`, and generic URLs. |
| `pkg/assets/` | Target-aware asset resolution, executable validation, archive selection, naming, and checksums. |
| `pkg/systempackage/` | Package-artifact detection and normalization. |
| `pkg/prompt/` | Interactive prompts and multi-select helpers. |
| `pkg/spinner/` | Terminal spinner UI. |

## Data flow

1. `cmd/root.go` loads config and configures logging.
2. The command resolves a provider in `pkg/providers`.
3. The provider resolves compatible products through `pkg/assets`. Metadata, package artifacts, delta patches, and wrong-platform assets are excluded before scoring; ambiguity prompts interactively or fails without a TTY or in non-interactive mode.
4. Release downloads are unpacked and validated as compatible native executables, shebang scripts, or Windows batch scripts without being run. The final write path validates bytes again before chmod or replacement.
5. The command installs, updates, removes, or runs the binary.
6. The config file is updated with the stable logical command name, the raw outer source asset, and the resolved inner archive path when applicable.

## Behavior worth knowing

- Hooks are part of persisted config and are executed around install/update/remove operations.
- `install` and `update` preserve provider metadata so later runs can use the same provider and artifact selection. `remote_name` is the stable command name; `source_asset` is informational and is never an exact update constraint.
- System-package installs can carry extra metadata such as package type and macOS app bundle name so later lifecycle commands still work.
