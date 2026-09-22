# 03: Enforce the Go suite on Windows

**What to build:** Make the complete Go unit suite portable to Windows and run it for every pull request and master update on a native hosted Windows runner. The job should catch runtime path, executable-format, and process behavior that Linux cross-compilation cannot exercise, without adding live release installs or privileged package operations.

**Blocked by:** 01 / Resolve managed Windows names and destinations; 02 / Preserve Go import-path separators on Windows.

**Status:** resolved

**Implementation boundary:** Add one `windows-latest` job to `.github/workflows/build.yml`, reusing checkout/setup-go and `go test ./...`. Repair only fixtures or assumptions exposed by the native suite. Release-selection tickets 08/09 are already complete; later completion coverage uses this job instead of another matrix or test harness.

- [x] The archive-checksum test uses a payload and filename that pass the existing runnable-payload gate on the current test platform without weakening that gate.
- [x] The ordinary Go test suite passes on a native Windows runner, including managed-name, path, Go-install, release-selection, and persisted-selection regressions.
- [x] CI provisions the Go version declared by the repository and runs `go test ./...` on `windows-latest`.
- [x] Existing Linux lint, race, coverage, Action, installer, and live-install jobs remain unchanged.
- [x] Windows CI performs no live provider installs, release publication, privileged package installation, or mutation of developer state.
- [x] Run `mise run test`, `mise run test-race`, `mise run verify`, and `mise run build` before handoff.

## Implementation progress

- [x] Slice 1: Make the archive-checksum fixture pass the existing runnable-payload gate on each platform.
- [x] Slice 2: Add the bounded native Windows Go-test CI job without changing existing jobs.
- [x] Slice 3: Run required repository verification and resolve the ticket after review approval.
- [x] Slice 4: Repair additional runnable-payload fixtures identified by review as native-Windows failures.
- [x] Slice 5: Update testing guidance to include the native Windows Go suite.
- [x] Slice 6: Re-run verification and obtain review approval before resolving.

Verification completed on Linux: all required repository commands pass, and every Go test package cross-compiles for Windows. Native runtime execution is delegated to the new `windows-latest` CI job.

Initial review requested changes because additional successful processing tests still supplied Unix-only script payloads when using the native resolver, and the testing guide still described only Linux and macOS native suites.

Post-correction verification passed all required repository commands and Windows cross-compilation before final review.

Final spec review found and corrected one target-filter mismatch: Windows runnable fixtures now use Windows-labelled artifact names rather than retaining a Linux target token.

Final verification after that correction passes the full required command set, Windows test cross-compilation, and diff checks.

Follow-up spec review identified and corrected the remaining Linux-targeted successful fixtures in byte-transformation, persisted-selection, and HashiCorp checksum tests.

The final static scan also aligned the checksum test's outer archive name with the native Windows target; its focused test and Windows package cross-compilation pass.

## Answer

CI now runs the complete Go suite in an isolated `windows-latest` job using the Go version from `go.mod`. Successful asset and provider tests use platform-runnable payloads and target-compatible names on Windows without weakening executable validation, including GNU/MSVC selection-intent coverage. Testing guidance now records the native Windows suite and its no-live-install boundary.

Final Standards and Spec reviews approved the change after `mise run test`, `mise run test-race`, `mise run verify`, `mise run build`, Windows test cross-compilation, focused regressions, and diff checks passed. Native execution is performed by the new hosted Windows CI job.
