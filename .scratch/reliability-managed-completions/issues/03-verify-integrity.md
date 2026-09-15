# 03: Verify download and installed-byte integrity

**What to build:**

Integrate the strict checksum outcomes into release-provider fetching and artifact processing without weakening existing provider contracts. Prefer a supported GitHub release-asset digest; only when it is absent, use a strict applicable manifest. Define and document source priority. Unsupported advertised digest formats must produce an explicit policy outcome rather than being guessed or reported as verified, and an authoritative mismatch must not fall back to another checksum source.

Treat an applicable authoritative sidecar as required evidence: retrieval or parsing failure fails the fetch. A failing unrelated sidecar must not affect the selected artifact. Preserve distinct structured errors and integrity scope through the shared integrations for GitHub, GitLab, Codeberg, HashiCorp, and other release providers that use the checksum helper.

Carry actual transformation evidence from the download through decompression and member extraction. Verify known outer-download bytes before transformation. Establish installed-byte verification only from an independently bound final-file digest, or from processing evidence that proves an unchanged-byte transfer; filename equality alone is not proof. Keep download and installed-byte records separate, including algorithm, expected and observed values, scope, source, and result.

Persist optional scoped integrity records with managed binary metadata while preserving `hash` as the SHA-256 of installed bytes and loading older records without migration. Ensure cloning, config mutation, export, and import retain compatible optional metadata without turning imported provenance into a claim that local bytes were verified. If export recalculates an installed hash, invalidate any verification assertion that no longer matches those bytes. A failed installation must preserve the prior executable and its verified metadata rather than overwriting either.

Keep nearby integrity/source-priority documentation accurate.

**Blocked by:** 02 / Bind checksum records to exact artifacts

**Status:** resolved

- [x] A supported GitHub release-asset digest takes priority over an applicable manifest; absent digest uses the strict manifest path, while malformed or unsupported advertised digests have an explicit non-success outcome.
- [x] An authoritative digest mismatch cannot fall back to another source, and applicable sidecar retrieval or parse failures fail fetching with their structured integrity reason.
- [x] Unrelated failing sidecars do not fail the selected artifact or become checksum evidence for it.
- [x] Provider integrations preserve integrity errors and download-versus-installed scope for GitHub, GitLab, Codeberg, HashiCorp, and every shared checksum-helper caller.
- [x] Outer download bytes are verified before extraction when known; archive extraction or other transformations cannot claim outer verification as installed-byte verification.
- [x] Installed-byte verification is recorded only for an independently bound final-file digest or an evidenced unchanged-byte transfer, including renamed unchanged payloads; same-name transformed payloads do not receive that assertion.
- [x] Download and installed-byte integrity records retain algorithm, expected and observed digest, source, scope, and result separately.
- [x] Existing `hash` remains the installed-byte SHA-256, older configuration loads without migration, and optional integrity records round-trip compatibly through config cloning, export, and import.
- [x] Imported provenance is not represented as newly verified local bytes; exporting a recalculated installed hash invalidates a stale verification assertion.
- [x] An integrity failure leaves a prior installation and its verified record intact.
- [x] Existing payload validation, release lanes, pinning, aliases, provider behavior, and non-interactive safety remain intact.


- [x] Add behavioral provider regressions for missing, supported, malformed, and unsupported GitHub digests; mismatches; applicable sidecar HTTP and parsing failures; and unrelated failing sidecars across representative shared-helper providers.
- [x] Add processing and persistence regressions for unchanged plain and renamed payloads, archive/final digests that differ, transformed same-name payloads, old-record loading, export/import provenance isolation, recalculated-hash invalidation, and preservation after failed installation.
- [x] Run the focused provider, artifact-processing, configuration, and command test suites.
- [x] Run `mise run lint`.
- [x] Run `mise run test-race` because integrity metadata and failed-install preservation cross filesystem and persisted-state boundaries.

## Implementation progress

- [x] Slice 1: Integrate authoritative provider digest/checksum outcomes and source priority. (`pkg/providers`; focused tests and lint passed)
- [x] Slice 2: Carry scoped download/installed-byte integrity through artifact processing and provider results. (`pkg/assets`, `pkg/providers`; focused tests and lint passed)
- [x] Slice 3: Persist integrity metadata through config/export/import while preserving compatibility and failure semantics. (`pkg/config`, `cmd`; focused tests, lint, and race tests passed)
- [x] Slice 4: Complete focused regressions, nearby documentation, and full verification.
- [x] Review corrections: Preserve download evidence for system-package installs and cover shared provider boundaries.
- [x] Independent code review and approval.

## Answer

Implemented authoritative GitHub digest priority and strict no-fallback failures, scoped download and installed-byte integrity evidence across shared release providers and artifact transformations, and backward-compatible persistence/export/import behavior. System-package installs retain download evidence without claiming package-manager output verification. Added provider, processing, configuration, command, and preservation regressions; full tests, race tests, lint, and verification pass. Independent code review approved the final diff.
