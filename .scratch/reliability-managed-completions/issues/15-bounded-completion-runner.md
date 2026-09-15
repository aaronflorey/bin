# 15: Generate completions through a bounded native runner

**What to build:**

Generate Bash, Zsh, and Fish completions only after managed-completion policy authorizes the work and applicable bundled completion evidence has been considered. Support only recognized advertised completion protocols and explicitly configured structured argument-vector overrides for the installed command identity; do not probe arbitrary subcommands, accept shell fragments, or infer a generator from help text.

Execute generation through a native, bounded runner using the absolute installed executable. Run with closed standard input, a temporary working directory, and a minimal allowlisted environment that excludes credentials and inherited shell configuration. Apply native-platform time, output-size, and process-tree limits; explain unavailable platform guarantees rather than weakening the boundary. Skip generation when elevated rather than executing with elevation.

Treat generated text solely as untrusted output. Validate shell-specific syntax and command registration without sourcing it, and reject help text, output registered for another command, malformed output, nonzero exits, timeout, output overflow, and incomplete process cleanup as repairable outcomes. Keep process restrictions distinct from a sandbox or trust claim.

Preserve the installed binary and existing installation result regardless of generation failures. Keep generation after binary commit and within the managed-completion boundaries, without changing provider, artifact, executable selection, lifecycle-hook, system-package, state, or existing Cobra completion behavior.

**Blocked by:** 14 / Discover completion scripts from artifact inventory

**Status:** ready-for-agent

- [ ] Generation runs only when completion policy authorizes it, bundled evidence is unavailable or inapplicable, and the request uses a recognized advertised protocol or validated structured argument-vector override.
- [ ] The runner invokes only the absolute installed executable for the selected command identity and never executes arbitrary command text, shell fragments, or guessed generator subcommands.
- [ ] Native execution uses closed standard input, a temporary working directory, and a minimal allowlisted credential-free environment.
- [ ] Native-platform time, output-size, and process-tree limits bound generation; unsupported guarantees are reported explicitly and elevated execution is skipped.
- [ ] Generated Bash, Zsh, and Fish output is validated for supported syntax and registration of the installed command without sourcing or loading the output.
- [ ] Nonzero exits, timeouts, output overflow, child-process cleanup failures, help text, wrong-command registrations, and malformed output produce actionable repairable outcomes rather than trusted completion data.
- [ ] Generation restrictions are documented as containment rather than a sandbox or proof that generated output is trustworthy.
- [ ] A generation failure occurs after binary commit and does not replace, alter, or make an otherwise successful installed binary fail.
- [ ] Existing provider, artifact, executable selection and validation, lifecycle-hook, system-package, completion state, and Cobra completion behavior remains unchanged.


- [ ] Add controlled-helper regressions for each recognized protocol and structured override, rejecting arbitrary command text, shell fragments, guessed subcommands, and mismatched installed identities.
- [ ] Add runner regressions for absolute executable invocation, closed standard input, temporary working directory, allowlisted environment, elevated skip, time limit, output limit, and process-tree cleanup on supported native platforms.
- [ ] Add output-validation regressions for supported Bash, Zsh, and Fish registrations without sourcing, including help text, malformed syntax, wrong-command output, and nonzero generator outcomes.
- [ ] Add lifecycle regressions proving generation is policy-gated, follows bundled evidence, leaves failed outcomes repairable, and preserves the already installed binary and successful installation result.
- [ ] Run the focused completion, command, configuration, and artifact-processing test suites.
- [ ] Run `mise run lint`.
- [ ] Run `mise run test-race` because process control and local completion state cross shared filesystem and locking boundaries.
