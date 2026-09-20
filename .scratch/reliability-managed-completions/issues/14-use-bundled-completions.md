# 14: Prefer exact bundled completion files

**Status:** ready-for-agent

**Blocked by:** 13 / Sync completions for one managed binary.

## What to build

Extend the working sync command from 13 to prefer a bundled script when no explicit generator argv was supplied. Follow the exact filename/archive-scope rules in the [spec](../spec.md). Reuse `artifactInventory`, the existing completion-entry classification, bounded readers, and the current artifact cleanup owner in `pkg/assets/artifact_processing.go`.

Select one exact script for the resolved command and requested shell. Carry only the bounded selected bytes and a diagnostic name through the existing asset/provider result. A small request flag or shell field is sufficient to avoid completion reads for unrelated operations. Do not add a public inventory API, alternate resolver, discovery cache, evidence hashes, registration parser, or provider interface.

Sync reuses the stored version, selection intent, and normal HTTP provider policy. Only apply a bundled script when the fetched executable matches the current installed bytes. Otherwise use the installed generator from 13; absent bundled content also permits that fallback. Ambiguous or malformed matching bundled content warns/skips rather than guessing. Explicit generator argv bypasses this fetch. Docker and Go-install sources use native generation directly. Preserve fetch/security errors rather than treating them as proof that no bundled file exists; the user can explicitly request generator argv to avoid a fetch.

Keep the selected script available until the ordinary caller can publish it, without retaining archive paths after cleanup. Ticket 17 will use the same result from the artifact already fetched during install/update. Bundled-selection failures must not become binary-fetch failures; existing archive security failures remain errors.

## Acceptance

- [ ] Synthetic TAR/ZIP fixtures select an exact Bash, Zsh, or Fish script belonging to the selected executable's archive scope; a bundled success does not invoke the generator.
- [ ] Duplicate matching scripts, other commands, renamed commands without exact matches, malformed text, and oversized scripts are skipped without changing executable selection.
- [ ] An absent bundled candidate falls back to the native generator; explicit argv skips discovery; effectful providers are not re-executed for sync.
- [ ] Sync preserves stored release/member/variant choices, checks fetched bytes against the current installed executable, and closes every fetched result on success and failure.
- [ ] Default artifact operations and inspect do not request completion work. Existing containment, integrity, budgets, and cleanup behavior remain intact.

## Verification

Extend existing asset/provider fixtures and the sync command test from 13. Test observable selection and fallback, not a new internal discovery schema. Run focused asset/provider/command checks followed by the applicable repository checks in the spec.
