# 02: Bind checksum records to exact artifacts

**What to build:**

Replace heuristic checksum handling with separate applicability, parsing, binding, and verification steps. A checksum asset must first be established as applicable to a specific download or final installed file; parsing an applicable asset must not let a named record for one artifact become a digest for another.

Support only GNU, BSD, and explicitly declared column/hash-order manifest forms. Preserve filename case and relative-path identity. A bare SHA-256 is permitted only when an exact sidecar association establishes both its target and whether it applies to download or final-file bytes. Remove assumptions that derive an installed executable association from an archive stem. Permit basename fallback only when the format and candidate set establish one unambiguous target.

Reject conflicting records, ambiguous basename matches, malformed applicable records, truncation, and unsupported algorithms. Bound manifest input and individual scanner tokens, and surface read/scanner errors rather than accepting incomplete input. Do not infer an algorithm from digest length.

Represent integrity results explicitly as not supplied, verified, or failed. Failed outcomes must retain structured reasons that distinguish retrieval, parsing, unsupported-algorithm, and mismatch failures. Parsing may produce an expected digest awaiting verification, but that intermediate state must never be represented or persisted as verified. Preserve existing provider contracts and the installed-byte hash meaning; broader provider integration and persisted scoped integrity records remain follow-on work.

Keep nearby checksum behavior documentation accurate when this stricter applicability and outcome model changes it.

**Blocked by:** None (can start immediately)

**Status:** resolved

**Implementation slices:**

- [x] Define strict checksum applicability, parsing/binding, limits, and explicit outcome types with focused regressions.
- [x] Integrate strict outcomes into provider fetch flows while preserving provider and installed-hash contracts.
- [x] Update nearby documentation and run targeted/full verification.
- [x] Complete independent code review and address all findings.

- [x] GNU, BSD, and explicitly declared column/hash-order manifests parse only their supported forms while retaining exact case-sensitive filename and relative-path identity.
- [x] A named digest binds only to its exact artifact; a record for another artifact cannot be reinterpreted as a bare digest or associated through an archive-stem guess.
- [x] A bare SHA-256 verifies only when an exact sidecar association establishes the target and download/final-file scope.
- [x] Unambiguous basename fallback works only where the manifest form and candidate set prove a single target; case/path collisions and ambiguous basenames fail.
- [x] Duplicate or conflicting records, malformed applicable records, unsupported algorithms, oversized manifests or lines, truncated input, and scanner/read errors produce explicit failed outcomes.
- [x] Not-supplied, verified, and failed outcomes remain distinct, with structured reasons for retrieval, parsing, unsupported algorithm, and mismatch; an unverified expected digest is never reported as verified.
- [x] Existing payload validation, provider interfaces, and the installed-byte SHA-256 `hash` contract remain intact.


- [x] Add behavioral checksum regressions for supported manifest forms, correctly bound bare digests, wrong named targets, unrelated sidecars, path/case collisions, duplicate/conflicting records, unsupported algorithms, oversized content/lines, and scanner errors.
- [x] Run `mise exec -- go test ./pkg/providers -run 'Checksum|SHA256'`.
- [x] Run `mise run lint`.
