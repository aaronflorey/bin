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
4. Provider-derived executable names are untrusted portable single components: raw names are validated before derivation and final names after it. Archive paths stay relative archive identities until a validated leaf is selected. Release downloads are then validated as compatible native executables, shebang scripts, or Windows batch scripts without being run; the final write path validates bytes again before chmod or replacement.
5. The command installs, updates, removes, or runs the binary.
6. The config file is updated with the stable logical command name, raw outer source provenance, and resolved inner archive path when applicable. It also stores optional portable selection intent (logical product, target, and member) so install, update, and ensure retain deliberate choices without treating versioned filenames as constraints. Legacy records without that intent retain compatible behavior; it is derived only from unambiguous artifact facts.

## Behavior worth knowing

- Hooks are part of persisted config and are executed around install/update/remove operations.
- `install` and `update` preserve provider metadata so later runs can use the same provider and artifact selection. `remote_name` is the stable command name; `source_asset` is informational and is never an exact update constraint.
- `selection_intent` is distinct from source provenance. A member identity is recorded only after tar/ZIP traversal; raw filenames and gzip/xz/bzip2 stream names are not archive members. Its identity ignores only a narrow versioned top-level wrapper (`tool-v1/bin/tool` becomes `bin/tool`) whose remaining suffix is made entirely of recognized target tokens, preserving meaningful internal directories while allowing releases to change wrapper versions.
- Only explicit user destinations receive environment-variable expansion. `run` encodes unsafe version characters in cache filenames while retaining the original version as its cache identity; command code deterministically closes fetched streams and removes failed temporary package artifacts.
- System-package installs can carry extra metadata such as package type and macOS app bundle name so later lifecycle commands still work.
