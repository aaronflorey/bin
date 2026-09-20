# 01: Resolve managed Windows names and destinations

**What to build:** Make commands that address an existing managed executable accept the bare Windows command name even when the stored path has an executable suffix and its directory is absent from `PATH`. Interpret Windows filesystem paths as paths throughout install orchestration so an absolute or relative destination is never prefixed with the configured binary directory or routed through the wrong install mode.

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

**Implementation boundary:** Fix `getBinPath` and the existing alias/destination checks in `cmd/root.go`, `cmd/installer.go`, `cmd/install.go`, and `cmd/remove.go`. Reuse one shared rule wherever those callers need the same interpretation. Use stdlib/native path handling and the current lifecycle registry; do not add a parallel Windows resolver or rename unrelated APIs.

- [ ] A managed `rg.exe` can be addressed as either `rg` or `rg.exe`, case-insensitively, when its install directory is not on `PATH`.
- [ ] Bare-name resolution works through the shared targeting path used by pin, unpin, update, outdated, ensure, remove, and uninstall without changing explicit-path behavior.
- [ ] Recognized Windows executable suffixes remain distinct from unrelated filename extensions.
- [ ] Absolute, drive-relative, and separator-containing Windows destinations are treated as filesystem paths rather than command aliases.
- [ ] Reinstalling an existing managed system-package record on Windows preserves its destination and lifecycle strategy instead of entering direct-binary installation or panicking.
- [ ] Duplicate or ambiguous managed entries are not selected nondeterministically.
- [ ] Add native Windows regressions for bare-name lookup, an install destination outside the configured binary directory, and the existing release-lane/system-package scenario.
- [ ] Run the focused command tests, `mise run lint`, and the repository Go test suite available on the development platform.
