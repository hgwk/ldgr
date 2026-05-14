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

If you maintain a legacy ledger with historical rows that violate the new
lifecycle, run `ldgr verify` (default) for warnings; weak/historical rows do
not block commits. Use `ldgr verify --strict` only when you've intentionally
cleaned things up.

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
