# 02: Preserve Go import-path separators on Windows

**What to build:** Keep `goinstall://` module names and subpackage paths in Go's slash-delimited import-path form on every operating system while continuing to use native paths for the isolated local build output. Users must be able to install module roots and nested commands by version or `latest` on Windows.

**Blocked by:** None (can start immediately).

**Status:** resolved

**Implementation boundary:** Start with `parseRepo` in `pkg/providers/goinstall.go`, which currently applies `filepath.Clean` to an import path. Preserve the slash-delimited input when separating its version; retain `filepath` for local output paths. Fix the existing parsing/build flow and extend its table tests. No new import-path parser, provider, or configuration is needed.

- [x] Parsing a versioned Go-install target never rewrites `/` separators to `\` on Windows.
- [x] Base-module discovery retains a nested command path and constructs a valid `module/subpackage@version` argument.
- [x] Proxy metadata requests continue to address the base module while build commands include the selected subpackage.
- [x] Local `GOBIN` output discovery continues using native filesystem paths and the Windows `.exe` suffix.
- [x] Add regressions for module-root and nested-command targets that run in the native Windows suite.
- [x] Run the focused provider tests, `mise run lint`, and the repository Go test suite available on the development platform.

## Implementation progress

- [x] Slice 1: Preserve slash-delimited module roots and nested command paths during target parsing; extend parsing regressions.
- [x] Slice 2: Cover the resolved build/metadata/output flow, including native output paths and Windows executable naming.
- [x] Slice 3: Run required focused and repository verification, then resolve the ticket after review approval.

## Answer

Go-install target parsing now uses slash-aware import-path cleaning instead of filesystem-path cleaning. Fetch-time module discovery remains injectable for focused tests, and regressions verify that root and nested targets send metadata requests to the base module while invoking `go install` with the full slash-delimited package target. Native `GOBIN` path handling and Windows `.exe` output naming remain unchanged.

Review approved after focused provider tests, Windows test-binary compilation, lint, vet, the full Go test suite, repository verification, and diff checks passed.
