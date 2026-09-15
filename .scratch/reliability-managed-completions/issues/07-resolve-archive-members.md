# 07: Resolve archive members by identity

**What to build:**

Add a pure, reusable archive-member resolver over the owned executable inventory. Make its decision order deterministic and independent of archive traversal or input ordering: first apply target eligibility and any explicit member selection, then exact stored and logical identity, narrowly normalized wrapper identity, and finally an unambiguous basename fallback. Apply archive depth only to otherwise equivalent identities; it must not cause a helper or incidental member to outrank the requested product.

Normalize wrapper paths only where the wrapper convention is demonstrably non-identity-bearing. Preserve meaningful internal directories and do not turn broad path cleaning or basename extraction into an implicit selection rule. A basename match is eligible only when it identifies exactly one candidate after the earlier identity and compatibility rules; multiple matches remain ambiguous.

Validate explicit member choices against the same containment, normalized-identity, target-compatibility, executable-class, and final runnable-payload requirements as automatic selection. Return stable typed resolution errors and reasons for no eligible member, ambiguity, invalid explicit selection, and incompatible selection so callers, non-interactive behavior, and JSON diagnostics can distinguish failures without guessing.

Keep existing compatibility filtering, interactive ambiguity handling, non-interactive safety, final executable validation, integrity evidence, release lanes, aliases, pinning, and system-package behavior intact. Leave release-product and variant ranking to its dedicated follow-up work.

**Blocked by:** 06 / Process artifacts through a bounded owned inventory

**Status:** resolved

- [x] Archive-member resolution is a pure reusable operation over owned inventory metadata and produces the same result and rejection reason regardless of input order.
- [x] Resolution applies target eligibility and explicit selection before stored or logical identity, wrapper normalization, basename fallback, or archive depth.
- [x] Exact stored and logical member identities are preferred deterministically, while archive depth ranks only candidates with equivalent resolved identities.
- [x] Wrapper normalization is narrow and preserves meaningful directory identity; it cannot broadly clean paths or make an arbitrary nested helper match a requested member.
- [x] Basename fallback selects only a unique eligible candidate; duplicate basenames produce an ambiguity outcome rather than selecting by depth or iteration order.
- [x] Explicit member selection is validated for containment, normalized identity, executable eligibility, target compatibility, and final runnable payload validation; it cannot bypass these gates.
- [x] Missing, ambiguous, invalid explicit, and incompatible member outcomes use stable typed errors or reasons suitable for caller branching and JSON diagnostics.
- [x] Existing interactive and non-interactive selection safety, compatibility filtering, final executable validation, integrity semantics, aliases, pinning, release lanes, and system-package behavior remain unchanged.


- [x] Add resolver regressions showing stable selection and typed rejection reasons across reordered inventories.
- [x] Add identity-order regressions for requested members, stored identities, logical identities, narrowly wrapped members, unique basename fallback, and duplicate basename ambiguity.
- [x] Add depth regressions proving depth breaks ties only among equivalent identities and cannot promote an unrelated helper over an identity match.
- [x] Add explicit-selection regressions for valid selection and containment, normalization, target, executable-class, and runnable-payload rejection paths.
- [x] Exercise command-level interactive and non-interactive ambiguity behavior to confirm callers preserve selection safety and surface typed resolver failures accurately.
- [x] Run the focused command, artifact-processing, provider, and configuration test suites.
- [x] Run `mise run lint`.
- [x] Run `mise run test-race` because archive inventory and selection behavior cross shared artifact state and filesystem-backed processing.
