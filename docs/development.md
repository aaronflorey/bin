# Development

## Local workflow

The repository uses [`mise`](https://mise.jdx.dev/) as the source of truth for tool versions and project tasks. Install it and run `mise install` to pick up Go and `hk` from `mise.toml`.

```bash
mise install        # install Go and hk from mise.toml
mise run download
mise run fmt        # fix formatting when needed
mise run tidy       # fix module metadata when needed
mise run lint
mise run verify
mise run test
mise run build
```

`mise.toml` defines these tasks:

| Target | Effect |
| --- | --- |
| `build` | `go build .` |
| `fmt` | `gofmt -w -s ./.` |
| `fmt-check` | `gofmt -l -s ./.` (non-mutating) |
| `tidy` | `go mod tidy` |
| `mod-check` | `go mod tidy -diff` (non-mutating) |
| `lint` | `fmt-check` then `go vet ./...` |
| `test` | `go test ./...` |
| `test-race` | `go test -race ./...` |
| `coverage` | `go test -coverprofile=coverage.out ./...` |
| `download` | `go mod download` |
| `verify` | `download`, `fmt-check`, `mod-check`, `golangci-lint run` |
| `check` | `fmt-check`, `lint`, `test` |
| `fix` | `fmt` then `tidy` |
| `hooks` | `hk install --mise` |

`mise run lint` and `mise run verify` are check-only commands. Use `mise run fmt` to apply formatting fixes and `mise run tidy` to rewrite module metadata when the checks fail.

## Git hooks

Hooks are managed by [`hk`](https://github.com/jdx/hk) and configured in `hk.pkl`:

- `pre-commit` runs `go_fmt` with auto-fix on staged files.
- `pre-push` runs `go test ./...`.
- `commit-msg` enforces [Conventional Commits](https://www.conventionalcommits.org/) via `hk util check-conventional-commit`.

Install hooks once per clone:

```bash
mise run hooks    # runs: hk install --mise
```

Validate the hook config with `hk validate`.

## Repository layout

- `cmd/` contains the CLI surface.
- `pkg/` contains reusable logic for config, providers, prompts, assets, and system packages.
- `.github/workflows/` contains CI and release automation.
- `action.yml` defines the composite GitHub Action wrapper.

## Release automation

- CI runs lint, `go test ./...`, `go test -race ./...`, and a coverage pass (`.github/workflows/build.yml`).
- Releases are created by `release-please` and published with GoReleaser (`.github/workflows/release.yaml`, `.goreleaser.yml`).
- Conventional commit messages (`feat:`, `fix:`, etc.) drive the release-please changelog and version bumps.

## Notes

- `main.go` contains `//go:generate` directives for lint tooling, but the documented everyday workflow uses `mise` tasks instead.
