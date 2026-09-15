# 02: Preserve Go import-path separators on Windows

**What to build:** Keep `goinstall://` module names and subpackage paths in Go's slash-delimited import-path form on every operating system while continuing to use native paths for the isolated local build output. Users must be able to install module roots and nested commands by version or `latest` on Windows.

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] Parsing a versioned Go-install target never rewrites `/` separators to `\` on Windows.
- [ ] Base-module discovery retains a nested command path and constructs a valid `module/subpackage@version` argument.
- [ ] Proxy metadata requests continue to address the base module while build commands include the selected subpackage.
- [ ] Local `GOBIN` output discovery continues using native filesystem paths and the Windows `.exe` suffix.
- [ ] Add regressions for module-root and nested-command targets that run in the native Windows suite.
- [ ] Run the focused provider tests, `mise run lint`, and the repository Go test suite available on the development platform.
