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
