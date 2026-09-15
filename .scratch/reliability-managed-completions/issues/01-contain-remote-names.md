# 01: Contain remote names and clean failed fetches

**What to build:**

Treat provider-derived executable names as untrusted single filename components. Reuse the established portable-name policy without weakening import compatibility: reject empty names, dot components, NUL, either path separator, absolute, drive, and UNC forms, plus Windows-reserved and otherwise invalid Windows filename forms. Validate raw remote names before any transformation and validate the resulting final component after every transformation. Archive members remain validated relative identities; only a validated leaf may become an executable name. Do not make unsafe input acceptable by cleaning, taking a basename, or repeatedly sanitizing it.

Keep intentional user destinations separate from remote names. Continue expanding environment variables in explicit user destinations before they are joined, while never expanding provider-derived components in destination, tracked-path, or lifecycle handling. Contain cache entries derived from provider names and versions without changing stored version identity; release-lane separators must be encoded into a stable cache component rather than interpreted as directories.

Establish ownership and cleanup for a fetched stream immediately after a successful fetch. All early failures—including minimum-age, destination-resolution, pre-staging, copy, close, and cancellation failures—must close streams and remove temporary package artifacts as applicable. Keep a single clear owner (or idempotent close) and preserve provider cleanup responsibilities.

Keep nearby architecture and testing documentation accurate if this stricter remote-name and resource-ownership behavior changes their descriptions.

**Blocked by:** None (can start immediately)

**Status:** resolved

## Implementation progress

- [x] Slice 1: Validate untrusted remote executable components before and after transformation while preserving trusted destination handling.
- [x] Slice 2: Encode provider/version run-cache components without changing version identity.
- [x] Slice 3: Close fetched streams and clean temporary package artifacts on every post-fetch failure.
- [x] Slice 4: Update nearby documentation and run ticket-level verification.

- [x] Remotely derived names with a three-dot one-pass case or seven-dot two-pass case are rejected rather than converted into a safe-looking name.
- [x] Empty names, `.`, `..`, NUL, both separator styles, absolute paths, drive-qualified paths, UNC paths, and Windows-reserved or invalid names are rejected on the applicable platform.
- [x] Raw remote names are rejected before transformation, and final derived executable components are revalidated after transformation; archive member validation and executable-leaf derivation preserve their distinct roles.
- [x] Explicit absolute and relative user destinations, aliases, and supported environment-variable expansion continue to work.
- [x] A remote name containing environment-variable syntax remains literal after validation and cannot introduce path separators through expansion.
- [x] Provider/version cache names cannot escape their cache directory, including versions containing release-lane separators, while the provider's stored version identity remains unchanged.
- [x] Fetch streams are closed on success and every early command failure after fetch; failures before staging for system packages also release their resources.
- [x] Temporary package files are removed when copy or close fails, without conflicting duplicate cleanup or changes to provider-owned cleanup behavior.
- [x] Existing payload validation, provider interfaces, lifecycle behavior, portable import restrictions, release lanes, aliases, and non-interactive safety remain intact.


- [x] Add behavioral regressions at installer, provider, asset, cache, and system-package seams for unsafe names, trusted destinations, environment expansion, cache containment, and early-error cleanup.
- [x] Run `mise exec -- go test ./cmd ./pkg/assets ./pkg/providers`.
- [x] Run `mise run lint`.
- [x] Run `mise run test-race` because this work changes filesystem/resource ownership behavior.
