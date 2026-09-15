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

## Release checksum manifests

Providers accept only GNU (`<sha256>  <path>`), BSD (`SHA256 (<path>) = <sha256>`), and declared column/hash-order manifest records. GNU records require a SHA-256-declaring sidecar or manifest name; digest length never supplies the algorithm. Generic manifests are only candidates: unavailable or unrelated content is ignored, while a record that names the selected target is authoritative. A hash-order declaration applies only to its exact case-sensitive matching manifest stem, and multiple declarations fail. A named record binds to the exact, case-sensitive path of the selected download; an unambiguous basename fallback is allowed only when the release candidate set contains exactly one matching basename. Case or path collisions and duplicate, malformed, or unsupported-algorithm target records fail rather than selecting a digest.

A bare SHA-256 applies only when its sidecar is named exactly `<download>.sha256` or `<download>.sha256sum`, including case and path; it verifies the downloaded bytes, not an inferred installed executable. Manifests are limited to 2 MiB and individual lines to 64 KiB; truncated input and read/scanner failures fail.

Integrity outcomes distinguish not supplied, verified, and failed. Parsing produces a separate, unverified expected digest; the existing download and installed-byte checks consume it. Failures are categorized as retrieval, parsing, unsupported algorithm, or mismatch. Scoped integrity persistence and checksum-source priority are ticket 03 work; do not add them here.
