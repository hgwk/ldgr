# ldgr agent guide

This repo uses **ldgr** as the operational source of truth for goals, tickets,
and completed work. Keep the guide short; the CLI enforces the detailed rules.

## Source of truth

- `ledger/goal.json` — current objective.
- `ledger/tickets.jsonl` — append-only ticket lifecycle.
- `ledger/worklog.jsonl` — append-only completed-delivery log.
- `ledger/config.json` — repo identity and policy.

Do not edit ledger JSONL files directly. Use `ldgr`.

## Must follow

- Latest row per `id` is the current ticket state.
- Use the state model: `backlog`, `ready`, `doing`, `review`, `done`,
  `rework`, `blocked`, `dropped`.
- Move implementation to `review`; auditors move it to `done` or `rework`.
- Worklog follows audit-pass `done`; do not add worklog for unaudited work.
- Use the smallest sufficient verification gate. Do not report completion if
  verification did not run.
- Carry explicit success criteria into `review`/`done`; evidence should show
  which criteria passed.
- `done` evidence should include `commit:<sha>`, `pr:<url-or-number>`, or
  `no_commit:<reason>` so completed work is traceable.
- Set `ledger/config.json` `git_evidence` to `fail` when a repo requires Git
  evidence before completion, or `off` for repos where that guardrail is not
  meaningful.
- Prefer existing repo structure, scripts, modules, services, schemas, SDKs,
  and design primitives before adding new ones.
- Keep changes scoped. Do not let multiple agents edit the same file at once.
- Push, release, deploy, publish, upload, and destructive cleanup only when
  explicitly requested.

## Commands

```bash
ldgr next --ticket <id> --format json
ldgr ticket add   --json @-
ldgr ticket event --json @-
ldgr worklog add  --json @-
ldgr verify
ldgr view
```

After every write, read stderr. It contains the next concrete action.

## Shortcuts

```bash
ldgr ticket ready --ticket <id> --evidence "..."
ldgr audit pass --ticket <id> --evidence "..."
ldgr audit request-changes --ticket <id> --notes "..."
ldgr suggest worklog --ticket <id>
ldgr suggest commit  --ticket <id>
```

## Planning and handoff

Plan before non-trivial work: multi-file changes, three or more steps,
API/database/auth/shared type changes, architecture decisions, broad refactors,
or coordinated work.

For parallel work, split by ticket, role, scope, and paths. If blocked,
ambiguous, or assumptions fail repeatedly, append `blocked` or `rework` with the
reason and needed decision. Use stable reason words when useful:
`ambiguous_requirement`, `missing_context`, `conflicting_instruction`, or
`verification_failed`.

Handoff should include objective, owner/scope/paths, completed work, remaining
decisions, verification status, and risks.

## Verification records

Record verification commands in worklog `commands`. If verification could not
run, record the reason and risk in worklog `notes` and report it to the user.

## Implementation provenance

Before implementation, record reuse judgment in ticket `event.notes`:

```text
archived=<path or none>; borrow=<path or none>; reference=<path or none>; new=<delta + why>
```

Use `archived` for repo-local historical code/docs, `borrow` for directly
ported code/structure, `reference` for patterns only, and `new` for repo-specific
work plus why it cannot be borrowed. Add `not_borrowed=<why>` when relevant.

If stuck, run `ldgr next --ticket <id> --format json` and follow the suggested
command and skeleton.
