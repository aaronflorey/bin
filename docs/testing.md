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
just lint
just verify
just coverage
```

`just lint` runs `go fmt ./...` and `go vet ./...`. `just verify` adds dependency download, `gofmt -w -s`, and `golangci-lint run`. `just coverage` writes `coverage.out` in the repo root.

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
