# ldgr

Append-only project ledger for LLM agents. Multi-project unified view.

(Formerly `ledger-kit`. The short name is now the canonical product name; the long name lingers only in older release notes and migration docs.)

See `docs/superpowers/specs/` in the design repo for the full spec.

## Migrating from old layouts

If your repository has root-level `agent-tickets.jsonl`, `agent-worklog.jsonl`, or `goal.json`,
preview the migration first:

```bash
ldgr import legacy --target . --plan
```

Apply when you're satisfied:

```bash
ldgr import legacy --target . --apply
# optional: move legacy sources under ledger/legacy/
ldgr import legacy --target . --apply --archive-originals
```

`--apply` is idempotent. Running it twice produces "no changes". Ghost rows
(empty `ticket`/`task`) are preserved and neutralized by a companion
`invalidates_n` row so `ldgr verify` reports them as warnings, not fails.
Parse errors are preserved in `ledger/import-errors.jsonl`.

## Schema v1 migration

Historical legacy rows remain readable. Canonical schema v1 is the cleaned-up ticket
vocabulary:

```text
Ready    Doing    Review    Done
Backlog  Blocked  Rework    Dropped
```

New v1 ticket rows use `id`, `state`, `type`, `area`, `title`, `owner`, and an
`event` object. New v1 worklog rows keep their narrow meaning: one completed
delivery after an audit-pass `done` ticket. They do not carry lifecycle state.

To preview the rewrite:

```bash
ldgr migrate legacy-to-v1 --target . --plan
```

Apply only after reviewing the warning summary and deciding the backup/rollback
path is acceptable:

```bash
ldgr migrate legacy-to-v1 --target . --apply
```

`--apply` rewrites `ledger/config.json`, `ledger/tickets.jsonl`, and
`ledger/worklog.jsonl`, and always creates a backup under
`ledger/.backup/legacy-to-v1-<timestamp>/`. `goal.json` is left semantically
unchanged. Weak historical `done` and `changes_requested` rows are mapped back
to `review` instead of being promoted into fake audit-pass records; ghost rows
are kept with synthetic IDs and surfaced in the warning summary.

Common warning codes:

- `WEAK_DONE_MAPPED_REVIEW`: a historical `done` row did not prove audit pass,
  so it becomes review work instead of fake completion.
- `WEAK_REWORK_MAPPED_REVIEW`: a historical rework row lacked enough audit
  metadata, so it remains review work.
- `GHOST_TICKET_SYNTHESIZED` / `GHOST_WORKLOG_SYNTHESIZED`: an empty semantic
  row was preserved with a synthetic id instead of being dropped.
- `AREA_DEFAULTED`, `TYPE_DEFAULTED`, `ROLE_DEFAULTED`: v1 data lacked the
  corresponding v1 classifier; review samples before applying.
- `UNMAPPED_FIELD`: source data had extra fields preserved under `extra` /
  `event.extra`.

Verify and inspect after applying:

```bash
ldgr verify --target .
ldgr view --target .
```

Rollback is manual: copy the backed-up files from
`ledger/.backup/legacy-to-v1-<timestamp>/ledger/` back over `ledger/config.json`,
`ledger/tickets.jsonl`, and `ledger/worklog.jsonl`.

For production or active multi-agent repos, use this order:

```bash
ldgr migrate legacy-to-v1 --target . --plan
ldgr verify --target .
# inspect the warning samples and current dashboard
ldgr view --target .
# then, intentionally:
ldgr migrate legacy-to-v1 --target . --apply
ldgr verify --target .
ldgr view --target .
```

## Viewing your projects

`ldgr view` runs a read-only HTTP dashboard on `localhost`:

```bash
ldgr view                 # serve http://127.0.0.1:3030, all registered projects
ldgr view --port 8080     # custom port
ldgr view --target .      # single-project mode for the current directory
```

The dashboard polls every 5 seconds. Closing the terminal stops the server.
Ghost rows are hidden from the ticket tree and surfaced in the "Invalidated rows"
insight card.

## Guidance

After every ticket/worklog write, `ldgr` prints state-aware guidance to stderr.
stdout still contains only the normalized row JSON, so automation keeps working.

Ask explicitly:

```bash
ldgr next --ticket BUG-101
ldgr next --ticket BUG-101 --format json     # for LLM consumption

ldgr suggest worklog --ticket BUG-101        # JSON skeleton, only after audit pass
ldgr suggest commit  --ticket BUG-101        # Conventional Commit + PR/verification scaffold
```

The lifecycle is **enforced**, not advisory:

- Implementation moves through `open → in_progress → audit_ready` only.
- `audit_ready` requires non-empty `evidence`.
- Closing a ticket requires a separate audit row: `role=audit`, `status=done`,
  `audit_result=pass`, non-empty `evidence`, and `reviewed_n` pointing at the
  audit_ready row.
- `changes_requested` is also an audit row: `role=audit`,
  `audit_result=changes_requested`, `audit_notes`, and `reviewed_n`.
- `ldgr worklog add` is gated — it requires the ticket's latest row to be
  audit-pass done. Pre-audit calls are rejected.
- `ldgr suggest commit` refuses to scaffold before audit pass; use
  `--allow-unaudited` only when you know what you're doing.

Shortcut commands handle the common path:

```bash
ldgr ticket ready --ticket BUG-101 --evidence "go test ./..."
ldgr audit pass --ticket BUG-101 --evidence "go test ./..."
ldgr audit request-changes --ticket BUG-101 --notes "missing regression coverage"
```

`audit pass` and `audit request-changes` auto-set `reviewed_n` from the latest
`audit_ready` row.

For schema v1 projects, the same shortcuts use the v1 lifecycle:
`ready → doing → review → done|rework`. `ticket ready` writes `state=review`,
`audit pass` writes `state=done` with `event.result=pass`, and
`audit request-changes` writes `state=rework` with
`event.result=changes_requested`.

If you maintain a ledger with older rows, `ldgr verify` may report historical
compatibility warnings: old rows are being checked against the current
lifecycle/taxonomy gates. These do not block normal verify. For active append
gates on a historical project, use a baseline and check only new rows:

```bash
ldgr verify --target . --new-only --since-ticket-n 482 --since-worklog-n 321
```

Use `ldgr verify --strict` only when you've intentionally cleaned the historical
rows or migrated the project to canonical schema v1.

## Install

```bash
go install github.com/hgwk/ldgr@latest
```

Make sure `$(go env GOPATH)/bin` is on your `$PATH`. macOS/Linux:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

For release tarballs (after a `v*` tag has been published):

```bash
curl -sSL -o ldgr.tar.gz \
  https://github.com/hgwk/ldgr/releases/download/v0.1.0/ldgr_0.1.0_$(uname | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz
tar -xzf ldgr.tar.gz
sudo mv ldgr_*/ldgr /usr/local/bin/ldgr
```

(Adjust the architecture detection for your platform if the substitution fails.)

## Integrate into a repo

```bash
ldgr init                                # seed ledger/* in the current repo
ldgr init --language ko                  # optional: ledger free-text fields use Korean
ldgr hooks install                       # pre-commit verify
ldgr instructions install                # AGENTS.md / CLAUDE.md pointer + ledger-owned bodies
ldgr view --target .                     # dashboard for this project only
```

`--language` sets `ledger/config.json` → `writing_language`. Agents should use
that language for free-text ledger fields such as `task`, `notes`, `result`,
`audit_notes`, `summary`, and `acceptance`; schema keys, enum values, paths,
commands, and code identifiers stay unchanged.

Uninstall:

```bash
ldgr instructions uninstall              # remove pointer + bodies
ldgr instructions uninstall --keep-bodies
ldgr hooks uninstall                     # remove only the LDGR hook block
```
