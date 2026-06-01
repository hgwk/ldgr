# Changelog

## v0.2.0 - 2026-06-01

### Changed

- Install a single shared ldgr instruction body at `ledger/instructions/ldgr.md`.
- Point both `AGENTS.md` and `CLAUDE.md` at that shared body with a top-of-file
  `@ledger/instructions/ldgr.md` reference.
- Migrate old split instruction pointers and remove old split instruction
  bodies on reinstall.

## v0.1.0 - 2026-05-15

Initial public release of `ldgr`.

### Features

- Go CLI for project-local ledgers: `init`, `ticket`, `worklog`, `audit`,
  `verify`, `next`, `suggest`, `import`, `migrate`, `view`, hooks, and
  instructions install.
- Append-only audit lifecycle with transition checks, `reviewed_n` backlinks,
  audit pass/request-changes shortcuts, and worklog gating after audit pass.
- Stable verification codes, `--summary`, `--verbose`, `--strict`, and
  `--new-only` baseline filtering for historical ledgers.
- Project-aware guidance and skeleton generation for implementation, audit,
  correction, planning, commit, and PR handoff flows.
- Read-only web viewer with project selection, Kanban/grid views, drawer detail,
  taxonomy badges, ownership, stale lifecycle/claim indicators, active agents,
  audit queue, lifecycle metrics, and verify status.
- Legacy import and explicit `legacy-to-v1` canonical rewrite flow with backup,
  plan/apply modes, mapping warnings, and `historical_baseline` compatibility.
- Status taxonomy aliases for `doing`/`review` while preserving historical
  `in_progress`/`audit_ready` rows.
- Release workflow that runs Go quality gates, builds Linux/macOS tarballs, and
  attaches artifacts to `v*` GitHub releases.

### Fixes

- Reject direct implementation-to-done transitions without audit pass evidence.
- Require audit pass before delivery worklogs.
- Keep import apply from fabricating config unless explicitly initialized.
- Make viewer layout stable under long content, nested ticket trees, URL restore,
  and full-page Kanban scrolling.
- Replace emoji indicators with inline SVG icons for consistent rendering.

### Compatibility Notes

- No schema v2 exists in this release. The recent taxonomy work is a status
  label/compatibility refactor plus an optional canonical schema v1 rewrite path.
- Historical rows may surface lifecycle/taxonomy compatibility warnings. Default
  `ldgr verify` keeps those as warnings, and configured `historical_baseline`
  values can suppress old compatibility warnings for active append gates.
- `ldgr verify --strict` promotes warnings to failures. Use it only after
  historical compatibility warnings are intentionally cleaned or accepted.
- Local release install supports `go install`, direct tarball install, and manual
  placement under `/opt/homebrew/bin` for Homebrew users. A Homebrew tap is not
  published yet.
