# Testing

## Primary test command

```bash
go test ./...
```

This is also the command used by CI (`.github/workflows/build.yml`).

## Related checks

```bash
make lint
make verify
```

`make lint` runs `go fmt ./...` and `go vet ./...`. `make verify` adds dependency download, `gofmt -w -s`, and `golangci-lint run` (`Makefile`).

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
