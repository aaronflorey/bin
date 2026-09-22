# Local Issue State

Generated: 2026-09-22T00:55:24Z

NOTE: This file is auto-generated. Do not edit manually.

Tickets: 0 active, 19 resolved

## Active Tickets

| Effort | Ticket | Type | Mode | Status | Blocked by |
| --- | --- | --- | --- | --- | --- |

_No active tickets._

## Resolved Tickets

| Effort | Ticket | Type | Mode | Status | Blocked by |
| --- | --- | --- | --- | --- | --- |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [01-contain-remote-names](reliability-managed-completions/issues/01-contain-remote-names.md) | — | — | resolved | None (can start immediately) |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [02-bind-checksums](reliability-managed-completions/issues/02-bind-checksums.md) | — | — | resolved | None (can start immediately) |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [03-verify-integrity](reliability-managed-completions/issues/03-verify-integrity.md) | — | — | resolved | 02 / Bind checksum records to exact artifacts |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [04-safe-binary-replacement](reliability-managed-completions/issues/04-safe-binary-replacement.md) | — | — | resolved | 01 / Contain remote names and clean failed fetches; 03 / Verify download and installed-byte integrity |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [05-recoverable-install-commit](reliability-managed-completions/issues/05-recoverable-install-commit.md) | — | — | resolved | 04 / Replace direct binaries without deleting the working version |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [06-bounded-artifact-inventory](reliability-managed-completions/issues/06-bounded-artifact-inventory.md) | — | — | resolved | 05 / Commit executable and config state recoverably |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [07-resolve-archive-members](reliability-managed-completions/issues/07-resolve-archive-members.md) | — | — | resolved | 06 / Process artifacts through a bounded owned inventory |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [08-resolve-release-products](reliability-managed-completions/issues/08-resolve-release-products.md) | — | — | resolved | 07 / Resolve archive members by identity |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [09-persist-selection-intent](reliability-managed-completions/issues/09-persist-selection-intent.md) | — | — | resolved | 08 / Resolve release products before packaging |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [10-select-dmg-bundle](reliability-managed-completions/issues/10-select-dmg-bundle.md) | — | — | resolved | 01 / Contain remote names and clean failed fetches; 09 / Persist stable selection intent |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [11-process-generic-urls](reliability-managed-completions/issues/11-process-generic-urls.md) | — | — | resolved | 09 / Persist stable selection intent |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [12-inspect-artifacts](reliability-managed-completions/issues/12-inspect-artifacts.md) | — | — | resolved | 11 / Process generic URLs through the shared resolver |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [13-sync-managed-completions](reliability-managed-completions/issues/13-sync-managed-completions.md) | — | — | resolved | 12 / Inspect artifact decisions without side effects (resolved). |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [14-use-bundled-completions](reliability-managed-completions/issues/14-use-bundled-completions.md) | — | — | resolved | 13 / Sync completions for one managed binary. |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [17-integrate-completion-lifecycle](reliability-managed-completions/issues/17-integrate-completion-lifecycle.md) | — | — | resolved | 14 / Prefer exact bundled completion files. |
| [reliability-managed-completions](reliability-managed-completions/spec.md) | [18-document-and-verify](reliability-managed-completions/issues/18-document-and-verify.md) | — | — | resolved | 17 / Refresh opted-in completions and clean up owned files. |
| [windows-compatibility](windows-compatibility/spec.md) | [01-resolve-managed-windows-targets](windows-compatibility/issues/01-resolve-managed-windows-targets.md) | — | — | resolved | None (can start immediately). |
| [windows-compatibility](windows-compatibility/spec.md) | [02-preserve-go-import-paths](windows-compatibility/issues/02-preserve-go-import-paths.md) | — | — | resolved | None (can start immediately). |
| [windows-compatibility](windows-compatibility/spec.md) | [03-enforce-windows-go-tests](windows-compatibility/issues/03-enforce-windows-go-tests.md) | — | — | resolved | 01 / Resolve managed Windows names and destinations; 02 / Preserve Go import-path separators on Windows. |
