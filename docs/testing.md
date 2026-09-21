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

- Command behavior, fetched-stream cleanup, and run-cache containment in `cmd/*_test.go`
- Config path resolution and hook execution in `pkg/config/*_test.go`
- Provider normalization, untrusted remote-name rejection, and asset selection in `pkg/providers/*_test.go`
- Portable executable-name and archive-member validation in `pkg/assets/*_test.go`
- System package support in `pkg/systempackage/*_test.go`
- Managed completion sync, bounded native generation, ownership cleanup, and
  automatic-refresh policy in `cmd/*_test.go` and `pkg/assets/*_test.go`

## Managed-completion native smoke expectations

Completion tests use a controlled helper executable and disposable config,
install, and temporary directories. They must not fetch a live release, use a
privileged package operation, read a developer config, or alter persistent
shell startup files.

When exercising shell loading outside the Go suite, use a disposable `HOME` and
`BIN_CONFIG` and configure the shell explicitly. The isolated smoke expectation
is that Bash can source the owned Bash file, Zsh has the owned Zsh directory in
`fpath` before `compinit` and accepts a file whose first line is `#compdef
<command>`, and Fish has the owned Fish directory in `fish_complete_path`. Do
not rely on a developer's shell initialization or completion directories.

The workflow runs the Go suite on `ubuntu-latest` and `macos-latest`; its race
and coverage suites run on `ubuntu-latest`. This provides native Linux and
macOS Go-suite evidence. The isolated shell-loading subtests skip and report
unavailable shells as untested; unavailable platforms are likewise untested.
Windows compatibility coverage is owned by its separate platform work rather
than a second completion matrix.

## CI smoke coverage

The GitHub Actions workflow also runs install and action smoke tests for:

- the installer script (`install.sh`)
- provider installs from GitHub release assets
- system-package installs
- pinned installs
- the composite GitHub Action (`action.yml`)
