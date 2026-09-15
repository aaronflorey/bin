# 03: Enforce the Go suite on Windows

**What to build:** Make the complete Go unit suite portable to Windows and run it for every pull request and master update on a native hosted Windows runner. The job should catch runtime path, executable-format, and process behavior that Linux cross-compilation cannot exercise, without adding live release installs or privileged package operations.

**Blocked by:** 01 / Resolve managed Windows names and destinations; 02 / Preserve Go import-path separators on Windows; reliability-managed-completions/08 / Resolve release products before packaging; reliability-managed-completions/09 / Persist stable selection intent.

**Status:** ready-for-agent

- [ ] The archive-checksum test uses a payload and filename that pass the existing runnable-payload gate on the current test platform without weakening that gate.
- [ ] The ordinary Go test suite passes on a native Windows runner, including managed-name, path, Go-install, release-selection, and persisted-selection regressions.
- [ ] CI provisions the Go version declared by the repository and runs `go test ./...` on `windows-latest`.
- [ ] Existing Linux lint, race, coverage, Action, installer, and live-install jobs remain unchanged.
- [ ] Windows CI performs no live provider installs, release publication, privileged package installation, or mutation of developer state.
- [ ] Run `mise run test`, `mise run test-race`, `mise run verify`, and `mise run build` before handoff.
