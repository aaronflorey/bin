# 18: Document completion usage and verify native behavior

**Status:** resolved

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

## Comments

- 2026-09-21: Claimed for implementation. Plan: (1) document delivered completion behavior and manual repair, (2) add only missing isolated sync/shell-loading integration coverage, (3) add native macOS Go-test CI coverage only if absent, then run the ticket's final verification. Preserve `bin completion <shell>`, existing completion contracts, and Windows ticket 03 ownership; exclude acceptance harnesses, wording snapshots, and unrelated reliability changes.
- 2026-09-21: Slice 1 complete. Updated CLI, configuration, and testing docs with runnable sync/argv and opt-in/off examples, owned destinations and shell setup, discovery/trust limits, warning-only lifecycle behavior, and manual repair guidance. Examples and shell snippets were checked against the implemented commands.
- 2026-09-21: Slice 2 complete. Added one isolated native loading smoke per Bash, Zsh, and Fish through the existing controlled sync helper, with disposable environment/startup state and an explicit Zsh first-line `#compdef` check. Focused `cmd` tests and lint passed locally; all three shells were available.
- 2026-09-21: Slice 3 complete. Added a single `macos-latest` native Go-suite CI job without duplicating Linux race/coverage or creating a Windows matrix, and updated testing docs to describe Linux/macOS evidence and skipped unavailable shells. Workflow YAML parsing and diff checks passed.
- 2026-09-21: Final local verification complete on Linux 6.17.0-41-generic x86_64. Bash, Zsh, and Fish loading smokes all ran and passed; `mise run test`, `mise run test-race`, `mise run verify`, `mise run build`, and `git diff --check` passed. Native macOS execution remains CI-only evidence pending the new job.

## Answer

Documented managed completion sync, per-binary lifecycle choices, owned paths, manual shell setup and repair, discovery limits, and the execution trust boundary. Added isolated Bash, Zsh, and Fish loading smokes through the existing controlled sync flow, plus a focused native macOS Go-test CI job. All required local checks passed on Linux; the macOS run is delegated to CI and Windows remains owned by compatibility ticket 03.
