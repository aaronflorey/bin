# Testing

## Primary test command

```bash
go test ./...
```

This is also the command used by CI (`.github/workflows/build.yml`).

Additional CI verification runs:

```bash
go test -race ./...
go test -coverprofile=coverage.out ./...
```

## Related checks

```bash
mise run fmt
mise run tidy
mise run lint
mise run verify
mise run coverage
```

`mise run fmt` runs `gofmt -w -s ./.` and `mise run tidy` runs `go mod tidy` when you need to fix local drift. `mise run lint` is non-mutating and runs `gofmt -l -s ./.` plus `go vet ./...`. `mise run verify` is also non-mutating: it adds `go mod download`, `go mod tidy -diff`, and `golangci-lint run`. `mise run coverage` writes `coverage.out` in the repo root.

## What the test suite covers

- Command behavior in `cmd/*_test.go`
- Config path resolution and hook execution in `pkg/config/*_test.go`
- Provider normalization and asset selection in `pkg/providers/*_test.go`
- System package support in `pkg/systempackage/*_test.go`

## CI smoke coverage

The GitHub Actions workflow also runs install and action smoke tests for:

- the installer script (`install.sh`)
- provider installs from GitHub release assets
- system-package installs
- pinned installs
- the composite GitHub Action (`action.yml`)
