# ldgr — agent instructions

This repo uses **ldgr** (formerly `ledger-kit`) as the operational source of
truth for goals, tickets, and worklogs. Every multi-step task must flow through
the ledger so agents and humans share a single timeline.

## Ground rules

- **Append-only.** Never edit existing rows in `ledger/tickets.jsonl` or
  `ledger/worklog.jsonl`. Append a new row instead.
- **Latest row wins.** The most recent row per legacy `ticket` or canonical `id` is the
  current state.
- **`done` is an audit decision.** Implementers go to `audit_ready`; only an
  audit row (`role=audit`, `audit_result=pass`, `evidence`, `reviewed_n`) can
  move a ticket to `done`. Direct impl → done is rejected at append time.
- **Schema v1 vocabulary.** If `ledger/config.json` has `schema_version: 1`,
  use `id/state/type/area/title/event` instead of legacy
  `ticket/status/kind/category/task/role`. The enforced flow is
  `ready → doing → review → done|rework`, with `backlog`, `blocked`, and
  `dropped` as explicit side states.
- **Worklog requires audit pass.** `ldgr worklog add` is gated on the ticket
  being audit-pass done. New worklog rows must have a `ticket`; ticket-less
  worklog rows are reserved for ldgr-internal automation.
- **Writing language.** Free-text ledger fields (`task`, `notes`, `result`,
  `decision`, `audit_notes`, `handoff`, `summary`, `acceptance`) must use
  `ledger/config.json` → `writing_language` when set. Keep schema field names,
  enum values, code identifiers, file paths, and commands in their original
  technical form.
- **Guidance is one command away:** `ldgr next --ticket <id>`.

## Daily commands

```bash
ldgr next --ticket <id>                # what should I do next on this ticket?
ldgr ticket add   --json @-            # create a new ticket
ldgr ticket event --json @-            # state transition or correction
ldgr worklog add  --json @-            # record shipped delivery (after audit pass)
ldgr verify                            # validate the ledger
ldgr view                              # multi-project dashboard (http://127.0.0.1:3030)
```

## Lifecycle

```
plan      = open
implement = in_progress
verify    = audit_ready
done      = audit row with audit_result=pass

open → in_progress → audit_ready ──▶ done (audit pass)
                                 └─▶ changes_requested → in_progress
                              (blocked / cancelled exits are allowed too)
```

## Shortcuts

```bash
ldgr ticket ready --ticket <id> --evidence "..."
ldgr audit pass --ticket <id> --evidence "..."
ldgr audit request-changes --ticket <id> --notes "..."
```

These wrap the verbose `ticket event` flow and auto-set `reviewed_n` for audit
rows.

In schema v1 projects, the same shortcuts write `state=review`,
`state=done/event.result=pass`, and
`state=rework/event.result=changes_requested`.

## Implementation provenance

Before implementation, record the reuse decision in ticket `notes` using this
minimum shape:

```text
archived=<path or none>; borrow=<path or none>; reference=<path or none>; new=<delta + why>
```

Use `archived` for repo-local historical code/docs that were checked, `borrow`
for code or structure directly ported, `reference` for patterns only, and `new`
for repo-specific work plus why it cannot be borrowed. If a tempting reference
is intentionally not borrowed, include `not_borrowed=<why>` in the same notes.

If you get stuck on what to do, run `ldgr next --ticket <id> --format json` and
follow the suggested command + skeleton.
