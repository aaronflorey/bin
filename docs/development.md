# Development

## Local workflow

The repository’s documented workflow is:

```bash
just clean
just download
just lint
just verify
just test
just build
```

`justfile` defines these recipes:

| Target | Effect |
| --- | --- |
| `build` | `go build .` |
| `lint` | `go fmt ./...` and `go vet ./...` |
| `test` | `go test ./...` |
| `test-race` | `go test -race ./...` |
| `coverage` | `go test -coverprofile=coverage.out ./...` |
| `download` | `go mod download` and `go mod tidy` |
| `verify` | `download`, `gofmt -w -s ./.`, `golangci-lint run` |
| `hooks` | `lefthook install` |

## Repository layout

- `cmd/` contains the CLI surface.
- `pkg/` contains reusable logic for config, providers, prompts, assets, and system packages.
- `.github/workflows/` contains CI and release automation.
- `action.yml` defines the composite GitHub Action wrapper.

## Release automation

- CI runs lint, `go test ./...`, `go test -race ./...`, and a coverage pass (`.github/workflows/build.yml`).
- Releases are created by `release-please` and published with GoReleaser (`.github/workflows/release-please.yml`, `.goreleaser.yml`).

## Notes

- `main.go` contains `//go:generate` directives for lint tooling, but the documented everyday workflow uses `just` recipes instead.
