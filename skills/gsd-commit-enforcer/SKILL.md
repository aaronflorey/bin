# GSD Commit Enforcer

Apply these rules whenever this skill is injected.

## Commit rule

Do not return success with uncommitted work from this run.

- If you changed code, tests, or build/config files, create the relevant code commit first.
- If you changed planning artifacts and `commit_docs` is not `false`, create the relevant planning-docs commit first.

## Planner

When you write or revise phase planning artifacts such as `*-PLAN.md`, `*-RESEARCH.md`, `*-CONTEXT.md`, `*-VALIDATION.md`, or `.planning/STATE.md`, create a normal non-amended commit before returning success.

Suggested message shapes:

- `docs(phase-XX): add plans`
- `docs(phase-XX): update planning artifacts`

## Executor

After each completed implementation step that changes code, tests, or runtime config, create the relevant non-amended commit before continuing when the workflow does not already require a narrower commit boundary.

Do not return phase success until both of these are true:

- the code changes from this run have been committed
- the final planning-docs commit for `SUMMARY.md`, `STATE.md`, `ROADMAP.md`, or `REQUIREMENTS.md` has succeeded when those files changed and `commit_docs` is enabled

## Guardrails

- Do not create empty commits.
- Do not amend existing commits.
- Respect hooks.
- If a commit cannot be created, say exactly why in the final return.
