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

1. Implementation: `open → in_progress → audit_ready` (you).
2. Audit: append `role=audit` row with `audit_result=pass` (done) or
   `changes_requested`.
3. Worklog: only after the audit-pass `done` row.
4. Commit/PR: `ldgr suggest commit --ticket <id>` gives you the Conventional
   Commit line + verification block.

## When confused

```bash
ldgr next --ticket <id>
ldgr suggest worklog --ticket <id>
ldgr suggest commit  --ticket <id>
```

These read the ledger and tell you exactly which JSON to feed back into
`--json @-`.
