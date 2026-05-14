# ldgr Project-Aware Guidance Follow-up Plan (deferred)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Status:** Deferred until after v0.1.0. Do not mix this work into release/tag cleanup.

**Goal:** Expand `ldgr next` / `ldgr suggest` from ticket-local state guidance into project-aware workflow coaching. The tool should tell an LLM not just "what this ticket needs next", but "what the project needs next" using blockers, claims, parent progress, audit queues, evidence/worklog health, and optional git context.

**Architecture:**
- Keep `internal/guidance` pure and deterministic.
- Add a project-level context builder that reads latest tickets, worklogs, config, and optionally git state.
- Add `ldgr next` without `--ticket` for project queue recommendations.
- Add role-aware filtering: implementer, auditor, planner, maintainer.
- Add new `suggest` subcommands for audit/correction/plan/PR skeletons.
- No new ledger files. Policy knobs live in `ledger/config.json`.

**Spec impact:** Future update to §4.3 (`next`/`suggest`), §6.2 warnings, and `config.json` guidance policy.

---

## Hard Acceptance Criteria

1. `ldgr next` with no `--ticket` returns a project-level prioritized queue.
2. `ldgr next --ticket ID` includes project-aware context:
   - blocker status
   - active claim/path conflict
   - parent progress
   - worklog/evidence coverage
   - stale state warnings
3. `ldgr next --role implementer|auditor|planner|maintainer` changes queue ordering without changing ledger files.
4. `ldgr next --format json` includes severity-coded guidance items.
5. `ldgr suggest audit --ticket ID` emits pass and changes-requested JSON skeletons.
6. `ldgr suggest correction --ticket ID` emits append-only correction row skeletons.
7. `ldgr suggest plan --parent PARENT` emits a plan ticket skeleton for broad changes.
8. `ldgr suggest pr --ticket ID` emits PR title/body with summary, verification, ledger, and risk sections.
9. Optional `--git` adds dirty-file/branch/commit context but degrades gracefully outside git worktrees.
10. `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

---

## Guidance Priority Model

Default `ldgr next` project queue order:

1. `changes_requested` tickets with clear audit notes.
2. `audit_ready` tickets waiting for review.
3. stale `in_progress` tickets.
4. `blocked` tickets whose blockers are now done/cancelled and should be unblocked.
5. unblocked `open` tickets with acceptance and no active claim.
6. closed tickets missing worklog/evidence.
7. orphan worklog / invalidated-row cleanup.

Role-specific ordering:

| Role | Priority bias |
|---|---|
| `implementer` | `changes_requested`, unblocked `open`, stale own `in_progress` |
| `auditor` | `audit_ready`, weak `done`, missing evidence |
| `planner` | missing acceptance, broad-change tickets, blocked graph, parent gaps |
| `maintainer` | orphan worklogs, closed-without-worklog, invalidated rows, release readiness |

---

## JSON Guidance Shape

Extend current guidance JSON with severity-coded items:

```json
{
  "scope": "project",
  "role": "auditor",
  "recommendations": [
    {
      "ticket": "BUG-101",
      "status": "audit_ready",
      "priority": 1,
      "reason": "waiting for audit",
      "actions": ["append_audit_row"],
      "severity": "action"
    }
  ],
  "warnings": [
    {
      "code": "path_claim_conflict",
      "severity": "warning",
      "ticket": "FE-2",
      "message": "paths overlap with active claim by claude"
    }
  ]
}
```

Severity values:

- `info`
- `action`
- `warning`
- `error`

---

## Config Policy

Add optional `guidance` policy to `ledger/config.json`:

```json
{
  "guidance": {
    "require_audit_for_done": true,
    "require_worklog_for_done": true,
    "require_acceptance_for_broad_changes": true,
    "stale_in_progress_hours": 24,
    "ready_priority": ["changes_requested", "audit_ready", "open"]
  }
}
```

Defaults preserve current behavior when the block is absent.

---

## Task Granularity

7 tasks. Each is one TDD cycle.

### Task 1: project context builder

- [ ] Add `internal/guidance/context.go`.
- [ ] Build latest tickets, worklog index, blocker graph, claim/path index, parent progress.
- [ ] Tests for blocker status, claims, parent progress, worklog coverage.
- [ ] Commit `feat(guidance): build project context`.

### Task 2: project-level `next`

- [ ] `ldgr next` works without `--ticket`.
- [ ] Text output shows top recommendations and why.
- [ ] JSON output follows the project guidance shape.
- [ ] Commit `feat(next): recommend project queue`.

### Task 3: role-aware guidance

- [ ] Add `--role implementer|auditor|planner|maintainer`.
- [ ] Tests for ordering differences.
- [ ] Invalid role fails with a clear message.
- [ ] Commit `feat(guidance): prioritize by agent role`.

### Task 4: severity-coded warnings

- [ ] Add stable warning/action codes.
- [ ] Include severity in JSON output.
- [ ] Map existing ticket-local warnings into coded items.
- [ ] Commit `feat(guidance): emit severity coded items`.

### Task 5: expanded `suggest`

- [ ] Implement `suggest audit`.
- [ ] Implement `suggest correction`.
- [ ] Implement `suggest plan`.
- [ ] Implement `suggest pr`.
- [ ] Tests for skeleton shape and non-writing behavior.
- [ ] Commit `feat(suggest): add audit correction plan pr skeletons`.

### Task 6: optional git context

- [ ] Add `--git` to `next` and `suggest commit|pr`.
- [ ] Detect current branch, dirty paths, changed files.
- [ ] Warn when changed files fall outside latest ticket paths.
- [ ] Degrade gracefully outside git.
- [ ] Commit `feat(guidance): include optional git context`.

### Task 7: docs and smoke

- [ ] README examples for project-level `next`, role-aware queues, and expanded suggestions.
- [ ] Run:
  ```
  go test ./... -count=1
  go vet ./...
  gofmt -l .
  ```
- [ ] Commit `docs(guidance): document project-aware next`.

---

## Self-Review Checklist

- [ ] `ldgr next` without `--ticket` is useful as the first command an agent runs.
- [ ] Role-aware output changes ordering but never mutates files.
- [ ] JSON output is stable enough for LLM/tool consumption.
- [ ] `suggest` commands do not bypass audit/worklog rules.
- [ ] Git-aware guidance is optional and does not make non-git repos fail.
- [ ] No new ledger files.
- [ ] No external dependencies.

---

## Out of Scope

- Auto-assigning tickets.
- Automatically committing or opening PRs.
- MCP server.
- Viewer UI integration; the deferred control-tower plan may display these recommendations later.
