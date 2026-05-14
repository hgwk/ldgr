# ldgr — agent instructions

This repo uses **ldgr** (formerly `ledger-kit`) as the operational source of
truth for goals, tickets, and worklogs. Every multi-step task must flow through
the ledger so agents and humans share a single timeline.

## Ground rules

- **Append-only.** Never edit existing rows in `ledger/tickets.jsonl` or
  `ledger/worklog.jsonl`. Append a new row instead.
- **Latest row wins.** The most recent row per `ticket` is the current state.
- **`done` means audit-pass.** A bare `status=done` is weak; pair it with
  `role=audit`, `audit_result=pass`, and `evidence`.
- **Worklog after audit.** `ldgr worklog add` is for shipped deliveries
  recorded after an audit-pass `done` row, not "I think I'm done" notes.
- **Guidance is always one command away:** `ldgr next --ticket <id>`.

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
open → in_progress → audit_ready → done
                  ↘ changes_requested → in_progress
```

If you get stuck on what to do, run `ldgr next --ticket <id> --format json` and
follow the suggested command + skeleton.
