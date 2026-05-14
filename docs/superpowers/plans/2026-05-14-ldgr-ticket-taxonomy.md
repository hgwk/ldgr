# ldgr Ticket Taxonomy Follow-up Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Status:** Deferred until after v0.1.0. This can run after lifecycle enforcement, or in parallel if write-path changes are coordinated.

**Goal:** Add first-class ticket taxonomy so `ldgr` can distinguish broad plans, issues, and executable tasks, and can prioritize work without adding new ledger files. This should improve `ldgr next`, VCT scanning, and multi-agent handoff.

**Model:** Keep the four canonical files. Add optional ticket row fields:

```json
{
  "kind": "task",
  "priority": "P2",
  "parent_ticket": "plan-agent-runtime"
}
```

No `plans.jsonl` or `issues.jsonl`. Plans and issues are ticket rows with `kind=plan|issue`; executable work is `kind=task`.

---

## Field Semantics

### `kind`

Allowed values:

| kind | Meaning |
|---|---|
| `plan` | Broad work package, workstream, milestone slice, or parent board item. |
| `issue` | Problem, risk, design question, defect report, or decision needing resolution. |
| `task` | Executable implementation/docs/test/ops delivery unit. |
| `audit` | Dedicated audit/verification ticket when audit work is split from implementation. |
| `ops` | Administrative ledger maintenance, migration, invalidation, release chores. |

Default for missing legacy rows: `task` for active implementation-looking rows, otherwise unknown/missing warning only.

### `priority`

Allowed values:

| priority | Meaning |
|---|---|
| `P0` | Critical. Blocks release, safety, data integrity, or primary workflow. |
| `P1` | High. Important next work or serious user-visible issue. |
| `P2` | Normal/default. |
| `P3` | Low/backlog. |

Default for missing rows in projections: `P2`. Verify warns on unknown values.

### Parent semantics

Short term:
- Keep `parent_ticket` backward compatible.
- Existing workstream labels such as `MB`, `MC`, `DOC`, `BUG`, `DEMO` continue to work.
- If `parent_ticket` references another real ticket ID, VCT renders a deeper tree.

Long term:
- Consider adding `area` / `workstream` for category-like grouping.
- Move `parent_ticket` toward a true tree edge only after migration tooling exists.

---

## Hard Acceptance Criteria

1. Ticket rows may include `kind` and `priority`.
2. `ldgr ticket add|event` preserves and carries forward `kind` and `priority`.
3. New tickets default missing `kind` to `task` and missing `priority` to `P2`, unless user explicitly supplies values.
4. `ldgr verify` warns on missing/unknown `kind` or `priority` for legacy rows; strict mode fails unknown enum values.
5. VCT Kanban cards show priority and kind badges.
6. VCT Dashboard shows at least:
   - open P0/P1 count
   - blocked P0/P1 count
   - plan/issue/task counts
7. VCT Kanban includes user-facing filters for priority, kind, status/stage, parent/workstream, agent/claim, blocked-only, and evidence state.
8. VCT Kanban includes user-facing sort controls for priority, updated time, oldest first, parent/workstream, blocked-first, and missing-evidence-first.
9. Filter/sort state is encoded in the URL query string and survives refresh/share links.
10. Tree and Worklog pages get the subset of filters that fit their jobs:
   - Tree: parent/workstream, kind, priority, status/stage.
   - Worklog: ticket text search, agent, date/order.
11. `ldgr next` project-level guidance, once implemented, sorts by priority before age within the same recommendation bucket.
12. `ldgr suggest plan --parent PARENT` and/or future project-aware guidance use `kind=plan`.
13. No new ledger files.
14. `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean.

---

## Example Tree

```text
MB
└─ plan-agent-runtime       kind=plan   priority=P1
   ├─ issue-egress-policy   kind=issue  priority=P0
   ├─ as-mb-seccomp         kind=task   priority=P1
   └─ as-mb-workspace-mount kind=task   priority=P1
```

Rows:

```json
{
  "ticket": "plan-agent-runtime",
  "parent_ticket": "MB",
  "kind": "plan",
  "priority": "P1",
  "status": "open",
  "task": "Container runtime hardening plan"
}
```

```json
{
  "ticket": "issue-egress-policy",
  "parent_ticket": "plan-agent-runtime",
  "kind": "issue",
  "priority": "P0",
  "status": "open",
  "task": "Resolve proxy-only egress enforcement boundary"
}
```

```json
{
  "ticket": "as-mb-seccomp",
  "parent_ticket": "plan-agent-runtime",
  "kind": "task",
  "priority": "P1",
  "status": "in_progress",
  "task": "Implement seccomp profile enforcement"
}
```

---

## Task Granularity

7 tasks. Each task is one TDD cycle and should end with a commit.

### Task 1: model enums and defaults

- [ ] Add `KindEnum` and `PriorityEnum` to `internal/ledger/types.go`.
- [ ] Add defaulting in ticket add for missing `kind=task`, `priority=P2`.
- [ ] Ensure ticket event carry-forward preserves both fields.
- [ ] Tests for add defaults, event carry-forward, and explicit override.
- [ ] Commit `feat(ticket): add kind and priority fields`.

### Task 2: verify taxonomy rules

- [ ] Verify warns for missing `kind` / `priority` on historical rows.
- [ ] Verify warns for unknown kind/priority in non-strict mode.
- [ ] Strict mode fails unknown enum values.
- [ ] Keep legacy missing fields non-fatal by default.
- [ ] Commit `feat(verify): validate ticket taxonomy fields`.

### Task 3: viewer projections

- [ ] Extend dashboard projection with priority counts and kind counts.
- [ ] Count active P0/P1 and blocked P0/P1.
- [ ] Tests for counts and legacy default behavior.
- [ ] Commit `feat(viewer): project priority and kind counts`.

### Task 4: VCT UI badges and filters

- [ ] Kanban cards show priority badge and kind badge.
- [ ] Dashboard renders P0/P1 and kind distribution metrics.
- [ ] Add compact Kanban toolbar filters:
  - priority: `All | P0 | P1 | P2 | P3`
  - kind: `All | plan | issue | task | audit | ops`
  - stage/status: `All | Plan | Implement | Verify | Complete | blocked | changes_requested`
  - parent/workstream
  - agent/claimed_by
  - blocked only
  - evidence: `All | has evidence | missing evidence`
- [ ] Add Kanban sort menu:
  - priority
  - recently updated
  - oldest first
  - parent/workstream
  - blocked first
  - missing evidence first
- [ ] Persist filter/sort state in URL query params, e.g. `?page=kanban&priority=P1&kind=task&sort=priority`.
- [ ] Tree page supports parent/workstream, kind, priority, and status/stage filters.
- [ ] Worklog page supports ticket text search, agent filter, and newest/oldest sort.
- [ ] Check no card text overlap at mobile widths.
- [ ] Commit `feat(viewer): filter and sort ticket taxonomy`.

### Task 5: tree semantics polish

- [ ] If `parent_ticket` references a real ticket, tree rendering nests under that ticket instead of only grouping by parent label.
- [ ] Preserve current workstream grouping for parent labels that are not real ticket IDs.
- [ ] Tests for `MB -> plan -> task` nested view.
- [ ] Commit `feat(viewer): render nested ticket taxonomy tree`.

### Task 6: guidance priority integration

- [ ] Update project-aware guidance plan implementation, if present, to sort by priority.
- [ ] If project-level `next` is not implemented yet, add shared priority sort helpers for future use.
- [ ] `ldgr next --ticket ID` includes priority/kind context in JSON.
- [ ] Commit `feat(guidance): include ticket taxonomy context`.

### Task 7: docs and smoke

- [ ] README documents `kind`, `priority`, examples, and migration behavior.
- [ ] Instruction bodies tell agents to set `priority` and `kind` when opening tickets.
- [ ] Run:
  ```
  go test ./... -count=1
  go test -race ./...
  go vet ./...
  gofmt -l .
  ```
- [ ] Commit `docs(ticket): document taxonomy fields`.

---

## Self-Review Checklist

- [ ] Priority affects what humans and agents see first.
- [ ] Users can narrow VCT to P0/P1, blocked, verify, kind, parent/workstream, and agent without leaving the page.
- [ ] Filter/sort state survives refresh and can be shared as a URL.
- [ ] Plans/issues/tasks are distinguishable without new files.
- [ ] Existing ledgers remain readable.
- [ ] Missing taxonomy on historical rows does not block normal verify.
- [ ] Unknown taxonomy values are surfaced clearly.
- [ ] VCT becomes easier to scan, not noisier.
- [ ] No external dependencies.

---

## Out of Scope

- Moving workstream/category into a required `area` field.
- Automatic migration of all historical `parent_ticket` labels into real parent tickets.
- Drag/drop priority editing in the UI.
- Hosted issue tracker integration.
