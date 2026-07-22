# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in `bin`, please report it responsibly.

- **Do not** open a public GitHub issue for security vulnerabilities.
- Email the maintainer at **security@aaronflorey.com** with a description of the
  issue, steps to reproduce, and any relevant proof of concept.
- If you do not receive an acknowledgment within 72 hours, please follow up.

## Scope

This policy covers the `bin` CLI tool distributed from this repository,
including the Go source in `cmd/` and `pkg/`, the install script
(`install.sh`), and the composite GitHub Action (`action.yml` / `action/`).

Out of scope:

- Vulnerabilities in third-party tools that `bin` installs on your behalf.
- Issues in dependencies tracked in `go.mod`; report those upstream.
- Self-compiled builds from non-release commits.

## Disclosure

Once a fix is released, a GitHub Security Advisory will be published with a
CVE when applicable. Please do not publicly disclose a vulnerability until a
fix has been released and you have been notified.
