# 18: Document completion usage and verify native behavior

**Status:** ready-for-agent

**Blocked by:** 17 / Refresh opted-in completions and clean up owned files.

## What to build

Update the existing `docs/cli.md`, `docs/configuration.md`, and `docs/testing.md` for the completion behavior actually delivered by 13, 14, and 17. Include explicit sync and argv examples, per-binary opt-in/off, the bin-owned destinations, manual Bash/Zsh/Fish setup, bundled matching limits, warning-only lifecycle failures, and manual cleanup after ownership-save or removal failure.

Explain that completion code comes from the authorized installed tool/release. Generation has a command deadline and output cap, not a sandbox or a descendant-containment guarantee. Document unsupported/renamed tools honestly and preserve the distinction from `bin completion <shell>`. Correct nearby stale descriptions of completed reliability work only where necessary; do not reopen tickets 01–12 or write another contract catalogue.

Reuse native Go tests and add only missing integration coverage: a controlled helper through sync and one isolated loading smoke case per supported shell, including Zsh's first-line registration convention. Use a disposable HOME/config and explicit test shell setup, never the developer's startup files or tools. Cover Linux/macOS using the existing CI style; add a simple native macOS Go-test job if absent. Windows compatibility ticket 03 owns the Windows job. Share it, and report unavailable local platforms/shells as untested. Do not build an acceptance harness, duplicate CI matrices, or tests that assert documentation wording.

## Acceptance

- [ ] Docs give runnable sync, opt-in/off, shell setup, and manual repair examples that match implemented flags and paths.
- [ ] Existing native tests plus small isolated shell smoke cases demonstrate the supported behavior; CI/local evidence names the platform and any unavailable coverage.
- [ ] Tests require no live release, privileged package operation, developer config, or persistent shell changes.
- [ ] New failures are corrected and the applicable repository checks pass. No additional ticket-by-ticket approval or evidence ledger is required.

## Verification

Run the new focused checks, then `mise run test`, `mise run test-race`, `mise run verify`, and `mise run build` once on the final implementation. Run Action tests only if Action files changed. Check documentation links and examples directly; do not add prose snapshot tests.
