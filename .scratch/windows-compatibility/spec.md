# Windows compatibility

Status: ready-for-agent

## Problem Statement

Native Windows testing found that managed executables cannot reliably be addressed by bare command names, Windows paths can be mistaken for aliases and joined beneath the configured install directory, Go-install module paths acquire backslashes, and one archive test fixture assumes a Unix executable. The existing release-selection work must also preserve a deliberate GNU or MSVC choice across update and ensure.

## Solution

Reuse the deterministic release resolver and persisted-selection work in the reliability-managed-completions effort for Windows ABI continuity. Independently fix managed-name and path interpretation at their shared command seams, keep Go import paths slash-delimited, make the affected fixture platform-runnable, and run the complete Go suite on a native Windows CI runner.

## Dependencies

- Release-product resolution remains owned by `reliability-managed-completions/08`.
- Persisted product, ABI, CPU-variant, and archive-member intent remains owned by `reliability-managed-completions/09`.
- No duplicate Windows-only selection mechanism or update-time `--select` flag is introduced.

## Implementation size

Use the existing shared command targeting and install-mode helpers, stdlib path handling, and the current Go test workflow. Fix the demonstrated Windows assumptions in place; do not add a platform abstraction, replacement resolver, import-path parser, or new test harness. Tickets 08 and 09 above are already complete. These Windows fixes do not wait for managed completions; their single native CI job also runs later completion tests where applicable.

## Out of Scope

- Changing non-interactive `prune` confirmation behavior.
- PowerShell completion support.
- Live provider installation tests in routine Windows CI.
