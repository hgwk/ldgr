# Claude — ldgr operating notes

These are the rules the agent in this repo must respect. Read once, then keep
the daily commands at hand.

## Source of truth

`ledger/` holds the operational state. Treat the four files as canonical:

- `ledger/goal.json` — current objective.
- `ledger/tickets.jsonl` — append-only ticket lifecycle.
- `ledger/worklog.jsonl` — append-only completed-delivery log.
- `ledger/config.json` — repo identity + parent rules.

You do not edit these files directly. Use the CLI:

```bash
ldgr ticket add   --json @-
ldgr ticket event --json @-
ldgr worklog add  --json @-
ldgr goal set     --json @-
```

After every write, stderr contains the next concrete action. Read it.

## Writing language

Free-text ledger fields (`task`, `notes`, `result`, `decision`, `audit_notes`,
`handoff`, `summary`, `acceptance`) follow `ledger/config.json` →
`writing_language` when set. Do not translate schema field names, enum values,
code identifiers, file paths, commands, or quoted API names.

## Lifecycle the binary will enforce

1. Implementation: `open → in_progress → audit_ready` (this is you).
2. Audit (separate row): `role=audit`, `status=done`, `audit_result=pass`,
   non-empty `evidence`, and `reviewed_n` referencing the audit_ready row.
3. Worklog: only after the audit-pass `done` row. Worklog also requires
   a `ticket` field on the new CLI surface.
4. Commit/PR: `ldgr suggest commit --ticket <id>` gives the Conventional
   Commit + verification block (refused before audit pass without
   `--allow-unaudited`).

## When confused

```bash
ldgr next --ticket <id>
ldgr next --ticket <id> --format json
ldgr suggest worklog --ticket <id>
ldgr suggest commit  --ticket <id>
```

For the common audit path:

```bash
ldgr ticket ready --ticket <id> --evidence "go test ./..."
ldgr audit pass --ticket <id> --evidence "go test ./..."
ldgr audit request-changes --ticket <id> --notes "..."
```

These read the ledger, validate the transition, and write the right row.

If `ledger/config.json` has `schema_version: 1`, use v1 vocabulary:
`id/state/type/area/title/event`. The enforced flow is
`ready → doing → review → done|rework`, with `backlog`, `blocked`, and
`dropped` as explicit side states. In v1, the latest row is keyed by `id`, not
by v1 `ticket`; the same shortcut commands write the matching v1 rows.

## Implementation provenance

Implementation tickets should not start from a blank page. Put this minimum
provenance block in ticket `notes`:

```text
archived=<path or none>; borrow=<path or none>; reference=<path or none>; new=<delta + why>
```

Use `not_borrowed=<why>` when a reference exists but is intentionally skipped
because it is stack-specific, domain-specific, or otherwise mismatched.
