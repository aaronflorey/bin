# 08: Resolve release products before packaging

**What to build:**

Introduce a pure, reusable release-candidate resolver that operates on candidate descriptions and returns a selected result or stable rejection reason without depending on provider ordering, prompts, or filesystem state. Describe each candidate with separately tokenized logical product, target facts, ABI, CPU variant, and package format so callers can explain both selection and rejection consistently.

Resolve the intended logical product and compatible target before package-format preference. Treat operating system, architecture, ABI, CPU-feature variant, and format-implied targets as distinct compatibility facts rather than accumulated scoring hints. Infer target restrictions imposed by recognized formats, and reject candidates whose explicit or implied target conflicts with the requested platform. Do not let aliases or repeated tokens add preference, and do not permit a CPU-specific variant to stand in for a baseline or incompatible CPU target without an explicit safe compatibility rule.

Group equivalent candidates by product before comparing packaging, so standalone helpers, metadata-adjacent files, or an attractive package format cannot outrank the intended archive product. Keep package format as a deterministic tie-breaker only after product, target, ABI, and CPU-variant eligibility have been established. Return stable reasons for no eligible candidate, ambiguous product or variant, incompatible target, and invalid explicit choice so non-interactive callers and JSON diagnostics can branch without parsing presentation text.

Apply the same known-target and final runnable-payload validation gates to automatic selection, `--select`, and `--all`. Explicit selection may identify a candidate but cannot bypass compatibility, CPU-variant safety, artifact classification, archive-member resolution, or payload validation. Preserve existing provider transport and integrity behavior, interactive ambiguity handling, non-interactive safety, release lanes, aliases, pinning, system-package behavior, and final executable validation. Leave persistence of deliberate selection intent to its dedicated follow-up work.

**Blocked by:** 07 / Resolve archive members by identity

**Status:** resolved

- [x] Release-candidate resolution is pure, reusable, deterministic across reordered input, and returns candidate descriptions or results with stable typed reasons suitable for caller branching and JSON diagnostics.
- [x] Candidate descriptions distinguish logical product, OS, architecture, ABI, CPU-feature variant, package format, and format-implied target constraints without conflating them into a cumulative name score.
- [x] Product grouping and product/target/ABI/variant eligibility occur before package-format preference; packaging cannot cause a helper or unrelated standalone artifact to outrank the intended product.
- [x] Recognized formats contribute explicit implied target constraints, and candidates with conflicting explicit or implied targets are rejected rather than ranked for an incompatible platform.
- [x] Aliases and duplicate tokens do not increase preference, and CPU-specific variants are selected only when their compatibility with the requested CPU target is explicitly safe.
- [x] No eligible candidate, ambiguous product or variant, incompatible target, and invalid explicit choice produce stable distinguishable rejection reasons.
- [x] Automatic selection, `--select`, and `--all` retain known-target, artifact-classification, archive-member, and final runnable-payload validation gates.
- [x] Existing transport and integrity semantics, interactive and non-interactive selection safety, release lanes, aliases, pinning, system-package behavior, and final executable validation remain unchanged.


- [x] Add pure resolver regressions proving stable product, target, ABI, and CPU-variant outcomes and rejection reasons across reordered candidate descriptions.
- [x] Add grouping and ranking regressions showing logical product resolution precedes package format and prevents helpers or standalone artifacts from outranking the intended archive product.
- [x] Add compatibility regressions for token-aware OS, architecture, ABI, and CPU-feature variants, including aliases that do not add preference and unsafe CPU-variant fallback rejection.
- [x] Add format-implied target regressions, including AppImage rejection for Darwin and Windows targets.
- [x] Add an AgentsView Darwin ARM64 regression proving intended product and target resolution before packaging preference.
- [x] Add `--select` and `--all` regressions confirming explicit requests still reject unknown or incompatible targets and invalid runnable payloads.
- [x] Exercise command-level interactive and non-interactive ambiguity behavior to confirm callers preserve selection safety and surface stable resolver failures accurately.
- [x] Run the focused command, asset-processing, provider, and configuration test suites.
- [x] Run `mise run lint`.
- [x] Run `mise run test-race` because candidate selection crosses shared artifact state and filesystem-backed processing.
