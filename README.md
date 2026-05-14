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

The state machine pushes you through `open → in_progress → audit_ready → done`.
`done` means an audit row with `audit_result=pass` is on file. `ldgr worklog add`
is intended for shipped work after that audit row; the binary warns when it sees
you doing it earlier.

## Install

Until v1 is tagged, build from source:

```bash
go install github.com/hgwk/ldgr@latest
```

After v1, GitHub Releases will publish `ldgr_<version>_<os>_<arch>.tar.gz`
artifacts for darwin/linux × arm64/amd64.

## Integrate into a repo

```bash
ldgr init                                # seed ledger/* in the current repo
ldgr hooks install                       # pre-commit verify
ldgr instructions install                # AGENTS.md / CLAUDE.md pointer + ledger-owned bodies
ldgr view --target .                     # dashboard for this project only
```

Uninstall:

```bash
ldgr instructions uninstall              # remove pointer + bodies
ldgr instructions uninstall --keep-bodies
ldgr hooks uninstall                     # remove only the LDGR hook block
```
