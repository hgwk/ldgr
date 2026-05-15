# Ledger Status Taxonomy Refactor Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Status:** Implementation complete in the working tree. Status taxonomy labels,
compatibility filtering, `ldgr migrate legacy-to-v1 --plan|--apply`, canonical
write paths, guidance/suggest output, and viewer 4×2 Kanban support are
implemented and verified locally. Rollout to existing projects remains explicit
and opt-in.

**Goal:** Refactor the ticket status taxonomy and row vocabulary without a
schema-v2 bump. The user-facing labels move toward `ready/doing/review/rework`
while compatibility code keeps historical rows readable. Projects that want the
canonical row vocabulary can opt into the explicit `ldgr migrate legacy-to-v1`
flow.

**Non-goal:** Do not describe this as schema v2. Do not silently mutate existing
projects. Do not mix canonical fields into legacy rows as an implicit partial
rewrite.

**Rollout rule:** Documentation and tooling may be shipped before migrating any real project. A target repo moves to v1 only after `ldgr migrate legacy-to-v1 --target <repo> --plan` has been reviewed, backup behavior is accepted, and `--apply` is run intentionally.

---

## Taxonomy Direction

Legacy rows are the current historical data. Canonical schema v1 is the optional
rewrite target for projects that want the normalized vocabulary.

`ledger/config.json`:

```json
{
  "schema_version": 1,
  "project_id": "...",
  "slug": "agent-zero",
  "name": "Agent Zero",
  "writing_language": "ko"
}
```

v1 ticket rows:

```json
{
  "n": 17,
  "ts": "2026-05-15T01:00:00Z",
  "id": "as-me-4-users-block-ui",
  "parent": "ME",
  "type": "task",
  "state": "doing",
  "area": "frontend",
  "priority": "P1",
  "title": "사용자 차단 UI 구현",
  "owner": "codex",
  "blocked_by": [],
  "acceptance": ["차단/해제 액션이 audit log에 남는다"],
  "evidence": [],
  "event": {
    "actor": "codex",
    "role": "implementer",
    "summary": "작업 시작",
    "notes": "archived=none; borrow=none; reference=...; new=..."
  }
}
```

v1 worklog rows:

```json
{
  "n": 9,
  "ts": "2026-05-15T03:00:00Z",
  "ticket": "as-me-4-users-block-ui",
  "actor": "codex",
  "title": "사용자 차단 UI 구현 완료",
  "summary": "차단/해제 UI와 audit log 연결을 구현했다.",
  "paths": ["packages/web/src/admin/Users.tsx"],
  "commands": ["pnpm test", "ldgr verify"],
  "notes": ""
}
```

Kanban v1 grid:

```text
Ready    Doing    Review    Done
Backlog  Blocked  Rework    Dropped
```

---

## Hard Acceptance Criteria

1. Existing legacy projects still pass v1 verify and v1 viewer paths.
2. Canonical canonical schema v1 projects use only v1 ticket/worklog vocabulary on new writes.
3. `ldgr migrate legacy-to-v1 --plan` writes nothing.
4. `ldgr migrate legacy-to-v1 --apply` creates a backup under `ledger/.backup/legacy-to-v1-<ts>/` before rewriting.
5. Migration rewrites `config.json`, `tickets.jsonl`, and `worklog.jsonl`; `goal.json` remains semantically unchanged.
6. Migration output passes `ldgr verify`.
7. Viewer uses 4×2 `grid` Kanban for schema v1 and keeps existing `columns` projection for legacy.
8. Worklog remains a completed-delivery ledger and is accepted only for tickets whose latest state is `done`.
9. Documentation and instructions no longer describe canonical v1 using legacy terms (`status`, `kind`, `category`, `task`, `role`) except in migration mapping sections.
10. `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .`, and `node --check internal/viewer/assets/app.js` pass.

---

## Phase 1 — Spec And Terms

### Task 1.1: schema v1 spec

- [x] Finalize v1 ticket row required fields.
- [x] Finalize v1 worklog row required fields.
- [x] Finalize enums:
  - `type`: `epic`, `plan`, `issue`, `task`, `audit`, `ops`
  - `state`: `backlog`, `ready`, `doing`, `blocked`, `review`, `rework`, `done`, `dropped`
  - `area`: `frontend`, `backend`, `runtime`, `docs`, `infra`, `test`, `security`, `release`, `ops`
  - `priority`: `P0`, `P1`, `P2`, `P3`
  - `event.role`: `planner`, `implementer`, `auditor`, `operator`, `reviewer`
- [ ] Commit `docs(schema): define ledger schema v1`.

### Task 1.2: migration semantics

- [x] Document migration as an explicit exception to runtime append-only writes.
- [x] Define `--plan`, `--apply`, and `--backup` behavior.
- [x] Define backup path and rollback expectations.
- [ ] Commit `docs(migrate): specify legacy to v1 semantics`.

---

## Phase 2 — Model And Detection

### Task 2.1: schema detector

- [x] Add config loader support for schema v1 and schema v1.
- [x] Expose `SchemaVersion(target)` helper for CLI/viewer/verify.
- [x] Unknown or missing schema version keeps current v1 compatibility behavior.
- [x] Tests cover v1, v1, missing version, and malformed config.
- [ ] Commit `feat(config): detect ledger schema version`.

### Task 2.2: v1 model constants

- [x] Add v1 required field sets and enum maps.
- [x] Add v1 transition graph.
- [x] Keep v1 constants intact and clearly separated.
- [ ] Commit `feat(ledger): add schema v1 model constants`.

---

## Phase 3 — Verify

### Task 3.1: verifier branching

- [x] Branch verifier by config schema version.
- [x] v1 verifier remains behavior-compatible.
- [x] v1 verifier validates v1 ticket/worklog required fields and enums.
- [ ] Commit `feat(verify): branch by schema version`.

### Task 3.2: v1 transition and audit rules

- [x] Enforce v1 state transition graph:
  ```text
  backlog → ready | dropped
  ready   → doing | blocked | dropped
  doing   → review | blocked | dropped
  blocked → ready | doing | dropped
  review  → done | rework | dropped
  rework  → doing | ready | dropped
  done/dropped terminal
  ```
- [x] Enforce `state=done` audit pass requirements.
- [x] Enforce `state=rework` changes-requested requirements.
- [x] Enforce worklog ticket latest state is `done`.
- [ ] Commit `feat(verify): enforce schema v1 lifecycle`.

---

## Phase 4 — Migration Planner

### Task 4.1: row mapping engine

- [x] Implement pure v1 ticket row → v1 ticket row mapping.
- [x] Implement pure v1 worklog row → v1 worklog row mapping.
- [x] Preserve `n` and `ts` where valid.
- [x] Preserve important unknown fields under a documented `event.extra` or migration warning strategy. Decide before implementation.
- [ ] Commit `feat(migrate): map v1 rows to v1`.

### Task 4.2: mapping warnings

- [x] Emit stable warning codes:
  - `AREA_DEFAULTED`
  - `TYPE_DEFAULTED`
  - `ROLE_DEFAULTED`
  - `SUMMARY_DEFAULTED`
  - `GHOST_TICKET_SYNTHESIZED`
  - `GHOST_WORKLOG_SYNTHESIZED`
  - `WEAK_DONE_MAPPED_REVIEW`
  - `WEAK_REWORK_MAPPED_REVIEW`
  - `WORKLOG_TICKET_DEFAULTED`
  - `UNMAPPED_FIELD`
  - `INVALID_TRANSITION_AFTER_MAPPING`
- [x] `--plan` output groups warnings by code and gives samples.
- [ ] Commit `feat(migrate): report mapping warnings`.

### Task 4.3: migration plan CLI

- [x] Add `ldgr migrate legacy-to-v1 --target . --plan`.
- [x] Writes nothing; test with tree hash before/after.
- [x] Shows row counts for config/tickets/worklog/goal.
- [ ] Commit `feat(migrate): plan schema v1 migration`.

---

## Phase 5 — Migration Apply

### Task 5.1: atomic backup and rewrite

- [x] Add `ldgr migrate legacy-to-v1 --apply`.
- [x] Create `ledger/.backup/legacy-to-v1-<ts>/`.
- [x] Rewrite `config.json`, `tickets.jsonl`, and `worklog.jsonl` atomically.
- [x] Keep `goal.json` semantically unchanged.
- [ ] Commit `feat(migrate): apply schema v1 migration`.

### Task 5.2: post-apply verification

- [x] Run v1 verify after apply.
- [x] If verify fails, leave backup and report rollback path.
- [x] Tests cover successful apply, failed apply, and idempotent second apply.
- [ ] Commit `fix(migrate): verify schema v1 output`.

---

## Phase 6 — v1 CLI Writes

### Task 6.1: ticket add/event v1 write path

- [x] For schema v1 targets, `ticket add` accepts v1 JSON only.
- [x] For schema v1 targets, `ticket event` carries forward v1 snapshot fields and writes v1 event metadata.
- [x] Do not mirror old v1 fields into v1 rows.
- [ ] Commit `feat(ticket): write schema v1 tickets`.

### Task 6.2: shortcut commands

- [x] `ticket ready` writes `state=review`.
- [x] `audit pass` writes `state=done`, `event.role=auditor`, `event.result=pass`, `event.reviewed_n`.
- [x] `audit request-changes` writes `state=rework`, `event.role=auditor`, `event.result=changes_requested`, `event.reviewed_n`.
- [ ] Commit `feat(audit): support schema v1 shortcuts`.

### Task 6.3: worklog add v1 write path

- [x] v1 worklog uses `actor`, `title`, `summary`, `paths`, `commands`, `notes`.
- [x] Reject worklog unless latest ticket state is `done`.
- [ ] Commit `feat(worklog): write schema v1 worklogs`.

---

## Phase 7 — Guidance And Suggestions

### Task 7.1: v1 next guidance

- [x] `ldgr next --ticket` uses v1 state names and transition graph.
- [x] Ticket-scoped JSON output includes v1 vocabulary only.
- [x] Text output uses `writing_language` hint unchanged.
- [x] Project-wide `ldgr next` queue uses v1 latest tickets and v1 role guidance.
- [ ] Commit `feat(guidance): support schema v1 next`.

### Task 7.2: v1 suggest skeletons

- [x] `suggest worklog` emits v1 fields for v1 projects.
- [x] `suggest audit` emits v1 fields for v1 projects.
- [x] `suggest plan`, `suggest commit`, `suggest pr`, and `suggest correction` emit v1 fields for v1 projects.
- [x] v1 project behavior remains unchanged.
- [ ] Commit `feat(suggest): emit schema v1 skeletons`.

---

## Phase 8 — Viewer

### Task 8.1: v1 kanban projection

- [x] Add `grid` API projection for schema v1:
  ```text
  Ready    Doing    Review    Done
  Backlog  Blocked  Rework    Dropped
  ```
- [x] Keep v1 `columns` projection unchanged.
- [x] Tests cover all eight states.
- [ ] Commit `feat(viewer): project schema v1 kanban grid`.

### Task 8.2: v1 viewer UI

- [x] Render v1 grid when `/kanban` response has `grid`.
- [x] Keep v1 rendering when response has `columns`.
- [x] Update cards to show `id`, `title`, `type`, `area`, `owner`, `priority`, evidence/audit status.
- [ ] Commit `feat(viewer): render schema v1 kanban grid`.

### Task 8.3: v1 drawer

- [x] Drawer shows v1 ticket snapshot and `event` metadata.
- [x] Reviewed row backlink uses `event.reviewed_n`.
- [x] Worklog links use v1 `ticket` id.
- [ ] Commit `feat(viewer): render schema v1 drawer`.

---

## Phase 9 — Docs And Instructions

### Task 9.1: instruction templates

- [x] Update AGENTS/CLAUDE templates to describe v1 vocabulary when target is schema v1.
- [x] Keep v1 instructions available for canonical schema v1 projects until migration.
- [ ] Commit `docs(instructions): add schema v1 operating guide`.

### Task 9.2: migration guide

- [x] README documents:
  ```bash
  ldgr migrate legacy-to-v1 --target . --plan
  ldgr migrate legacy-to-v1 --target . --apply --backup
  ldgr verify --target .
  ldgr view --target .
  ```
- [x] Explain rollback from `ledger/.backup/legacy-to-v1-<ts>/`.
- [ ] Commit `docs(migrate): add schema v1 migration guide`.

---

## Mapping Rules

Ticket fields:

| legacy | canonical v1 |
|---|---|
| `ticket` | `id` |
| `parent_ticket` | `parent` |
| `kind` | `type` |
| `status` | `state` |
| `category` | `area` |
| `task` | `title` |
| `agent` | `event.actor` |
| `role` | `event.role` |
| `notes` | `event.notes` |
| `decision` | `event.summary` or `event.decision` |
| `audit_notes` | `event.notes` |
| `audit_result` | `event.result` |
| `reviewed_n` | `event.reviewed_n` |

State:

| legacy | canonical v1 |
|---|---|
| `open` + `kind=plan|issue` | `backlog` |
| `open` | `ready` |
| `in_progress` | `doing` |
| `blocked` | `blocked` |
| `audit_ready` | `review` |
| `changes_requested` | `rework` |
| `done` | `done` |
| `cancelled` | `dropped` |

Role:

| legacy | canonical v1 |
|---|---|
| `impl` | `implementer` |
| `plan` | `planner` |
| `audit` | `auditor` |
| `ops` | `operator` |
| `review` | `reviewer` |
| `design` | `planner` |

Area defaulting:

- `category` that matches a v1 area is copied.
- `category=docs` or parent `DOC` maps to `docs`.
- `category=test` maps to `test`.
- `category=infra` maps to `infra`.
- `category=release` maps to `release`.
- Otherwise `area=ops` and warning `AREA_DEFAULTED`.

---

## Open Decisions

- Should unknown v1 fields be preserved under `event.extra`, dropped with `UNMAPPED_FIELD`, or copied top-level under an `x_` prefix?
- Should fresh `ldgr init` default to schema v1 until v1 stabilizes, or offer `--schema-version 2` immediately?
- Should migration preserve exact row `n`, or renumber after dropping invalidated/ghost rows? Default recommendation: preserve row `n` unless a row is omitted into migration errors.
- Should v1 `goal.json` remain schema version 1, or use the project config schema version only? Default recommendation: leave `goal.json` unchanged.

---

## Out Of Scope

- Hosted dashboard/auth.
- New canonical ledger files.
- Automatic migration during `ldgr init`, `ldgr verify`, or `ldgr view`.
- Supporting partial mixed v1/v1 ticket rows in the same project.
