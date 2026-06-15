# ldgr Integration

## Repo Setup

```bash
ldgr init
ldgr init --language ko
ldgr hooks install
ldgr instructions install
ldgr view --target .
```

`ldgr init` and `ldgr instructions install` write the authoritative instruction
body to `~/.ldgr/operating-guide.md` and add a top-of-file absolute
`@.../.ldgr/operating-guide.md` reference to both `AGENTS.md` and `CLAUDE.md`,
creating those files when missing.

This is the shared guide-pointer convention used by `cduo`, `ldgr`, and `hrns`:
the home-local guide holds the long body, while root policy files only carry the
absolute `@...` pointer and any project-local rules below it.

Sandboxed runners that cannot write the default home-local path can override
the ldgr home directory with `LDGR_HOME` or `--home`:

```bash
LDGR_HOME=.ldgr-home ldgr init --target .
ldgr init --target . --home .ldgr-home
ldgr instructions install --target . --home .ldgr-home
```

## Registry Cleanup

```bash
ldgr registry list
ldgr registry list --json
ldgr registry prune --dry-run
ldgr registry prune
ldgr registry prune --dry-run --json
```

Registry JSON output is schema-versioned for dashboards and scripts:

```json
{
  "schema_version": 1,
  "project_count": 3,
  "path_count": 3,
  "missing_count": 0,
  "projects": []
}
```

`ldgr registry prune --json` emits `schema_version`, `dry_run`,
`pruned_count`, `project_count`, `projects`, and `paths`. `projects` is a
stable array of `{ "project_id": "...", "paths": [...] }` entries sorted by
project id. Formal contracts live in `schemas/registry-list.schema.json` and
`schemas/registry-prune.schema.json`.

## Ticket JSON

`--language` sets `ledger/config.json` to a preferred `writing_language`.
Agents should use that language for free-text ledger fields such as `task`,
`notes`, `result`, `audit_notes`, `summary`, and `acceptance`; schema keys,
enum values, paths, commands, and code identifiers stay unchanged.

To create a state-model ticket row, start from the built-in example:

```bash
ldgr ticket add --example
ldgr ticket add --json @ticket.json
```

## Uninstall

```bash
ldgr instructions uninstall
ldgr instructions uninstall --keep-bodies
ldgr hooks uninstall
```
