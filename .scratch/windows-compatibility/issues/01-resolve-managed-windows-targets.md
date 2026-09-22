# 01: Resolve managed Windows names and destinations

**What to build:** Make commands that address an existing managed executable accept the bare Windows command name even when the stored path has an executable suffix and its directory is absent from `PATH`. Interpret Windows filesystem paths as paths throughout install orchestration so an absolute or relative destination is never prefixed with the configured binary directory or routed through the wrong install mode.

**Blocked by:** None (can start immediately).

**Status:** resolved

**Implementation boundary:** Fix `getBinPath` and the existing alias/destination checks in `cmd/root.go`, `cmd/installer.go`, `cmd/install.go`, and `cmd/remove.go`. Reuse one shared rule wherever those callers need the same interpretation. Use stdlib/native path handling and the current lifecycle registry; do not add a parallel Windows resolver or rename unrelated APIs.

- [x] A managed `rg.exe` can be addressed as either `rg` or `rg.exe`, case-insensitively, when its install directory is not on `PATH`.
- [x] Bare-name resolution works through the shared targeting path used by pin, unpin, update, outdated, ensure, remove, and uninstall without changing explicit-path behavior.
- [x] Recognized Windows executable suffixes remain distinct from unrelated filename extensions.
- [x] Absolute, drive-relative, and separator-containing Windows destinations are treated as filesystem paths rather than command aliases.
- [x] Reinstalling an existing managed system-package record on Windows preserves its destination and lifecycle strategy instead of entering direct-binary installation or panicking.
- [x] Duplicate or ambiguous managed entries are not selected nondeterministically.
- [x] Add native Windows regressions for bare-name lookup, an install destination outside the configured binary directory, and the existing release-lane/system-package scenario.
- [x] Run the focused command tests, `mise run lint`, and the repository Go test suite available on the development platform.

## Implementation progress

- [x] Slice 1: Add one shared deterministic managed-name rule and cover bare Windows executable lookup.
- [x] Slice 2: Apply shared Windows path interpretation to install/remove orchestration and cover destination/lifecycle regressions.
- [x] Slice 3: Run focused and repository verification, fix only ticket-related failures, and record completion.

## Comments

- 2026-09-21: Slice 1 complete. Added deterministic shared managed-name lookup so native Windows command names match `.exe` case-insensitively outside `PATH`, while unrelated extensions and ambiguous entries remain distinct.
- 2026-09-21: Slice 2 complete. Applied native path interpretation at existing target and destination seams, preserved Unix behavior, and retained the stored system-package lifecycle and destination during reinstall.
- 2026-09-21: Slice 3 complete. Added native Windows regressions for bare names, explicit destinations, and system-package/release-lane reuse. Focused command tests, lint, the repository Go suite, verification, and Windows command-test cross-compilation passed.
- 2026-09-21: Two-axis code review approved the final diff with no Standards or Spec findings after deterministic alias resolution and behavior-preservation corrections.

## Answer

Managed Windows executables now resolve by bare or `.exe` name without relying on `PATH`, and ambiguous managed names fail deterministically. Windows filesystem destinations retain their explicit paths through install and removal orchestration, including existing system-package lifecycle records and release-lane metadata. Native Windows regressions cover the reported scenarios; all required local checks passed.
