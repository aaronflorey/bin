# Getting started

## Prerequisites

- Go `1.24.4` if you plan to build from source (`go.mod`).
- `curl` or `wget` if you plan to use `install.sh` (`install.sh`).
- Optional: `gh` if you enable GitHub CLI token lookup through config.

## Install

### Quick install

```bash
curl -fsSL https://raw.githubusercontent.com/aaronflorey/bin/master/install.sh | sh
```

To install into `/usr/local/bin` with elevated privileges:

```bash
curl -fsSL https://raw.githubusercontent.com/aaronflorey/bin/master/install.sh | sudo sh
```

### Build from source

```bash
make download
make build
```

The binary is built from the repository root (`Makefile`, `main.go`).

## First run

`bin` runs `list` when you invoke it with no command arguments (`cmd/root.go`).

```bash
bin
bin version
```

If you want to install a tool immediately, try:

```bash
bin install github.com/cli/cli
```

## Verify the setup

```bash
bin version
bin list
make test
```

If you built from source, `go test ./...` should also pass.

## Common next steps

- Review [configuration](configuration.md) if the install directory is wrong.
- Read [cli](cli.md) for `install`, `update`, `remove`, and `run`.
- Read [integrations](integrations.md) before using provider-specific URLs or tokens.
