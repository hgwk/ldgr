# ldgr

[![npm version](https://img.shields.io/npm/v/@hgwk/ldgr.svg)](https://www.npmjs.com/package/@hgwk/ldgr)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Append-only task, ticket, audit, worklog, and coordination ledger for LLM-agent
projects, with a multi-project local viewer.

(Formerly `ledger-kit`. The short name is now the canonical product name; the long name lingers only in older release notes and migration docs.)

## Requirements

- macOS or Linux
- Go is optional for source installs; npm installs download the native binary
  from GitHub Releases

## Install

```bash
npm install -g @hgwk/ldgr
```

Or install from source with Go:

```bash
go install github.com/hgwk/ldgr@latest
```

Make sure `$(go env GOPATH)/bin` is on your `$PATH`. macOS/Linux:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Quick Start

```bash
cd /path/to/project
ldgr init
ldgr version
ldgr verify
ldgr view
```

## Companion Suite

- `cduo doctor` checks pair-agent runtime setup and project hook readiness.
- `ldgr verify` checks ledger lifecycle, audit, worklog, and Git evidence.
- `hrns audit` checks repository structure, docs, config, and code guardrails.

## Migrating from old layouts

If your repository has root-level `agent-tickets.jsonl`, `agent-worklog.jsonl`, or `goal.json`,
preview the migration first:

```bash
ldgr import legacy --target . --plan
```

Apply when you're satisfied:

```bash
ldgr import legacy --target . --apply
# optional: move legacy sources under .ldgr/legacy/
ldgr import legacy --target . --apply --archive-originals
```

`--apply` is idempotent. Running it twice produces "no changes". Ghost rows
(empty `ticket`/`task`) are preserved and neutralized by a companion
`invalidates_n` row so `ldgr verify` reports them as warnings, not fails.
Parse errors are preserved in `.ldgr/import-errors.jsonl`.
`ldgr verify` also surfaces these root legacy files so old layouts are handled
by ldgr itself rather than a separate checker.

## State model migration

Historical status-shaped rows remain readable. The current ticket vocabulary is:

```text
Ready    Doing    Review    Done
Backlog  Blocked  Rework    Dropped
```

Ticket rows use `id`, `state`, `type`, `area`, `title`, `owner`, optional
`team`, and an `event` object. Worklog rows keep their narrow meaning: one
completed delivery after an audit-pass `done` ticket. They do not carry
lifecycle state.

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
`ledger/worklog.jsonl`, records a `historical_baseline` in
`ledger/config.json`, and always creates a backup under
`.ldgr/backups/legacy-to-v1-<timestamp>/`. `goal.json` is left semantically
unchanged. Weak historical `done` and `changes_requested` rows are mapped back to
`review` instead of being promoted into fake audit-pass records; ghost rows are
kept with synthetic IDs and surfaced in the warning summary.

Common warning codes:

- `WEAK_DONE_MAPPED_REVIEW`: a historical `done` row did not prove audit pass,
  so it becomes review work instead of fake completion.
- `WEAK_REWORK_MAPPED_REVIEW`: a historical rework row lacked enough audit
  metadata, so it remains review work.
- `GHOST_TICKET_SYNTHESIZED` / `GHOST_WORKLOG_SYNTHESIZED`: an empty semantic
  row was preserved with a synthetic id instead of being dropped.
- `AREA_DEFAULTED`, `TYPE_DEFAULTED`, `ROLE_DEFAULTED`: source data lacked the
  corresponding classifier; review samples before applying.
- `UNMAPPED_FIELD`: source data had extra fields preserved under `extra` /
  `event.extra`.

Verify and inspect after applying:

```bash
ldgr verify --target .
ldgr view --target .
```

Rollback is manual: copy the backed-up files from
`.ldgr/backups/legacy-to-v1-<timestamp>/ledger/` back over `ledger/config.json`,
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
ldgr view                 # serve and open http://127.0.0.1:3030, all registered projects
ldgr view --port 8080     # custom port
ldgr view --target .      # single-project mode for the current directory
ldgr view --no-open       # serve without opening a browser
```

The dashboard opens in your default browser and polls every 5 seconds. Closing the terminal stops the server.
Ghost rows are hidden from the ticket tree and surfaced in the "Invalidated rows"
insight card.

## Guidance

After every ticket/worklog write, `ldgr` prints state-aware guidance to stderr.
stdout still contains only the normalized row JSON, so automation keeps working.

Ask explicitly:

```bash
ldgr next --ticket BUG-101
ldgr next --ticket BUG-101 --format json     # for LLM consumption
ldgr next --team platform                    # project queue for one team

ldgr suggest plan --ticket PLAN-1 --area backend --owner codex --team platform
ldgr suggest worklog --ticket BUG-101        # JSON skeleton, only after audit pass
ldgr suggest commit  --ticket BUG-101        # Conventional Commit + PR/verification scaffold
```

When multiple lanes run in parallel, add coordination before moving work into
`doing`:

```bash
ldgr claim add --ticket BUG-101 --lane api --resource backend/auth --ttl 2h
ldgr note add --kind decision --scope openapi --ticket BUG-101 --summary "keep v2 response shape"
ldgr claim release --ticket BUG-101 --summary "ready for review"
```

`ldgr next` includes active claims, conflicts, and recent shared notes after the
normal project/ticket guidance. The viewer shows the same information as a
coordination rail beside the Kanban board. Claims are for resources currently
being touched; notes are for lane-wide decisions, risks, handoffs, and
broadcasts. `ldgr verify` warns on stale coordination claims and overlapping
exclusive claims.

`suggest plan` creates a `type=plan`, `state=backlog` ticket skeleton. Use
`--area` for the responsibility lane, `--owner` for the assigned actor, and
optional `--team` for group ownership. `area` and `priority` are validated
against the ledger taxonomy before the JSON is printed.

The lifecycle is **enforced**, not advisory:

- Implementation moves through `ready → doing → review`.
- `review` requires test evidence such as `test:unit`, `test:smoke`,
  `test:browser`, `test:manual`, or a concrete test command.
- `done` requires passing test evidence; commit evidence alone is not enough.
- Closing a ticket requires a separate audit event: `event.role=auditor`,
  `state=done`, `event.result=pass`, passing test evidence, and `reviewed_n`
  pointing at the review row.
- Done evidence should also include `commit:<sha>`, `pr:<url-or-number>`, or
  `no_commit:<reason>`; `ldgr verify` warns when completed work is not tied to
  Git or an explicit exception.
- Requested changes are also an audit event: `state=rework`,
  `event.result=changes_requested`, `event.notes`, and `reviewed_n`.
- `ldgr worklog add` is gated — it requires the ticket's latest row to be
  audit-pass done with passing test evidence. Pre-audit calls are rejected.
- `ldgr suggest commit` refuses to scaffold before audit pass; use
  `--allow-unaudited` only when you know what you're doing.

Shortcut commands handle the common path:

```bash
ldgr ticket ready --ticket BUG-101 --evidence "go test ./..."
ldgr audit pass --ticket BUG-101 --evidence "go test ./..."
ldgr audit request-changes --ticket BUG-101 --notes "missing regression coverage"
```

`audit pass` and `audit request-changes` auto-set `reviewed_n` from the latest
`review` row. `ticket ready` writes `state=review`, `audit pass` writes
`state=done` with `event.result=pass`, and `audit request-changes` writes
`state=rework` with `event.result=changes_requested`.

If you maintain a ledger with older rows, `ldgr verify` may report historical
compatibility warnings: old rows are being checked against the current
lifecycle/taxonomy gates. These do not block normal verify. For active append
gates on a historical project, use a baseline and check only new rows:

```bash
ldgr verify --target . --new-only --since-ticket-n 482 --since-worklog-n 321
```

Use `ldgr verify --strict` only when you've intentionally cleaned or accepted all
historical compatibility warnings. A state-model rewrite can still retain
historical lifecycle/worklog violations behind the baseline.

Set `ledger/config.json` field `git_evidence` to tune completion evidence:

- `warn` or unset: warn when `done` lacks `commit:`, `pr:`, or `no_commit:`
- `fail`: fail verification for completed work without Git evidence
- `off`: suppress this one Git-evidence guardrail

For local development and manual installs, use this shared convention:

```bash
install -m 0755 ldgr ~/.local/bin/ldgr
```

If another PATH directory must expose `ldgr`, prefer a symlink back to
`~/.local/bin/ldgr` instead of copying multiple binaries.

For release tarballs (after a `v*` tag has been published):

```bash
curl -sSL -o ldgr.tar.gz \
  https://github.com/hgwk/ldgr/releases/download/v0.3.11/ldgr_0.3.11_$(uname | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz
tar -xzf ldgr.tar.gz
install -m 0755 ldgr_*/ldgr ~/.local/bin/ldgr
```

Use `~/.local/bin/ldgr` for local installs. A Homebrew tap is not published yet;
until then, use npm, `go install`, or the release tarball.

## Release Flow

- GitHub repository: `hgwk/ldgr`
- npm package: `@hgwk/ldgr`
- GitHub Releases hosts platform-specific Go binaries
- `.github/workflows/release.yml` builds and publishes binaries on version tags
- The npm package is published from GitHub Actions through npm Trusted
  Publishing (OIDC), not a long-lived `NPM_TOKEN`
- The npm package is a thin wrapper that downloads the appropriate binary from
  GitHub Releases on install
- Release tags must match `cmd/version.go` and `package.json`
- npm Trusted Publisher configuration:
  - Publisher: `GitHub Actions`
  - Organization or user: `hgwk`
  - Repository: `ldgr`
  - Workflow filename: `release.yml`
  - Environment name: empty

## Integrate into a repo

```bash
ldgr init                                # seed ledger/* in the current repo
ldgr init --language ko                  # optional: ledger free-text fields use Korean
ldgr hooks install                       # pre-commit verify
ldgr instructions install                # AGENTS.md / CLAUDE.md pointer + home-local body
ldgr view --target .                     # dashboard for this project only
```

See [docs/integration.md](docs/integration.md) for registry cleanup, JSON
contracts, instruction pointer details, sandbox home overrides, ticket JSON
examples, and uninstall commands.
