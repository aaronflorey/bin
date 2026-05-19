# bin documentation

`bin` is a Go CLI for installing, tracking, updating, and removing binaries from release artifacts, container images, generic URLs, and `go install` sources. It also ships as a GitHub Action and as a shell installer script.

## What this docs set covers

- [Getting started](getting-started.md)
- [Configuration](configuration.md)
- [CLI reference](cli.md)
- [Integrations and providers](integrations.md)
- [Architecture](architecture.md)
- [Development workflow](development.md)
- [Testing](testing.md)
- [Troubleshooting](troubleshooting.md)

## Recommended reading order

1. Read [getting started](getting-started.md) to install and verify `bin`.
2. Read [configuration](configuration.md) to understand config file paths and environment variables.
3. Read [cli](cli.md) for command behavior and flags.
4. Use [integrations](integrations.md) when you need provider-specific auth or URL rules.
5. Refer to [troubleshooting](troubleshooting.md) when commands fail.

## Known limitations

- There is no separate schema file for `config.json`; the shape is defined in Go structs under `pkg/config`.
- Provider support and asset matching are intentionally code-driven; if a detail is missing here, check the linked source files.
