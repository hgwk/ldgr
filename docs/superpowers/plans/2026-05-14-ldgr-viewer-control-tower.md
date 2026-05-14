# ldgr Viewer Control Tower Follow-up Plan (deferred)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Status:** Deferred until after Plan 4. Do not mix this work into the current hooks/instructions/guidance/release implementation.

**Goal:** Redesign `ldgr view` end-to-end into a control-tower UI: dashboard metrics first, sidebar navigation, Kanban workflow view, parent completion progress, ticket detail drawer, and separate pages for tree/worklog/insights. This is not a light CSS refresh; it replaces the current one-page layout and visual system.

**Architecture:**
- Keep single static binary, `embed.FS`, vanilla HTML/CSS/JS, zero external dependencies.
- Extend `internal/viewer/aggregate.go` with dashboard and Kanban projections.
- Extend HTTP API with read-only endpoints for dashboard/kanban/ticket detail.
- Replace one-page asset layout with app-shell navigation: Dashboard, Kanban, Tree, Worklog, Insights.
- Replace the visual system: spacing scale, typography scale, status colors, cards, tables/lists, metric bands, drawers, empty/error/loading states.
- No UI editing in this plan.

**Spec reference:** `docs/superpowers/specs/2026-05-14-ledger-kit-go-design.md` §7.2 and §7.2.1.

---

## Hard Acceptance Criteria

1. `ldgr view --target X` still boots as a single binary and serves all existing API endpoints.
2. Dashboard page is the first screen and shows:
   - overall Progress
   - Parent ticket completion
   - Audit pipeline
   - Delivery health
   - Recent activity
3. Kanban page groups latest ticket rows into `Plan`, `Implement`, `Verify`, `Complete`.
4. Kanban mapping follows spec §7.2.1 exactly.
5. Parent completion excludes `cancelled` from progress denominator and shows cancelled separately.
6. Sidebar contains project selector plus page navigation.
7. Tree, Worklog, and Insights move into separate pages without losing existing content.
8. Invalidated rows stay hidden from ticket/worklog lists and surfaced in Insights/Delivery health.
9. Full redesign acceptance:
   - no nested card stacks
   - no marketing/hero layout
   - control-tower density with scan-first hierarchy
   - consistent status color system across Dashboard/Kanban/Tree/Insights
   - clear empty, loading, missing-project, and API-error states
10. UI remains responsive at desktop and mobile widths; no overlapping text/cards.
11. `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

---

## API Additions

Existing endpoints remain unchanged.

### `GET /api/projects/{project_id}/dashboard`

```json
{
  "progress": {
    "done": 10,
    "active": 5,
    "cancelled": 2,
    "percent": 66
  },
  "parents": [
    {
      "parent": "DOC",
      "done": 4,
      "active": 2,
      "blocked": 1,
      "cancelled": 1,
      "percent": 66
    }
  ],
  "audit": {
    "audit_ready": 2,
    "changes_requested": 1,
    "weak_done": 0
  },
  "health": {
    "closed_without_worklog": 3,
    "orphan_worklog": 4,
    "invalidated": 2,
    "missing_evidence": 1
  },
  "recent": [
    { "kind": "ticket", "ticket": "DOC-1", "ts": "...", "status": "audit_ready", "task": "..." },
    { "kind": "worklog", "ticket": "DOC-0", "ts": "...", "result": "..." }
  ]
}
```

### `GET /api/projects/{project_id}/kanban`

```json
{
  "columns": [
    { "id": "plan", "title": "Plan", "tickets": [] },
    { "id": "implement", "title": "Implement", "tickets": [] },
    { "id": "verify", "title": "Verify", "tickets": [] },
    { "id": "complete", "title": "Complete", "tickets": [] }
  ]
}
```

### `GET /api/projects/{project_id}/tickets/{ticket_id}`

Returns latest row, full history, linked worklogs, invalidation metadata, and guidance summary.

---

## Visual Direction

- Full redesign target: control-tower feel, dense, operational, scan-first.
- Avoid marketing hero/card-heavy layout.
- Use full-width app shell with left sidebar and compact metric bands.
- Cards are only for repeated items and metrics, not nested containers.
- Use tables/lists for dense operational data where cards would waste space.
- Keep text sizes compact and stable; no viewport-width font scaling.
- First viewport must immediately show project identity, progress, parent completion, audit state, and top blocker/health signals.
- Status colors should be restrained and multi-hue:
  - Plan/open: neutral
  - Implement/in_progress/blocked/changes_requested: blue/amber/red accents
  - Verify/audit_ready: violet or teal accent
  - Complete/done/cancelled: green/gray
- Progress bars must have fixed heights and stable widths to avoid layout shift.
- Detail drawer replaces page jumps for ticket inspection. It must show latest row, event history, linked worklogs, paths, blockers, evidence, and guidance summary.
- Loading/error states must be designed, not raw exception strings in the main layout.

---

## Task Granularity

7 tasks. Each is one TDD cycle.

### Task 1: dashboard projections

- [ ] Add `Dashboard` projection types in `internal/viewer/aggregate.go`.
- [ ] Tests for overall progress, parent completion, audit pipeline, delivery health, recent activity.
- [ ] Commit `feat(viewer): compute control tower dashboard`.

### Task 2: Kanban projections

- [ ] Add `Kanban` projection types.
- [ ] Tests for Plan/Implement/Verify/Complete mapping.
- [ ] Ensure latest-row semantics and invalidated row exclusion.
- [ ] Commit `feat(viewer): compute ticket kanban`.

### Task 3: HTTP endpoints

- [ ] Add `/dashboard`, `/kanban`, `/tickets/{ticket_id}` handlers.
- [ ] Tests for JSON shape and unknown ticket/project errors.
- [ ] Keep existing endpoint tests green.
- [ ] Commit `feat(viewer): expose dashboard and kanban APIs`.

### Task 4: information architecture and visual system

- [ ] Replace current one-page layout with an app shell: sidebar, top project context, main page region, optional detail drawer.
- [ ] Define CSS variables for spacing, type, status colors, surfaces, borders, progress bars.
- [ ] Add loading, empty, missing project, and API error states.
- [ ] Commit `feat(viewer): redesign app shell and visual system`.

### Task 5: dashboard and page navigation

- [ ] Update `index.html`/`app.js`/`style.css` to sidebar navigation.
- [ ] Dashboard is default page.
- [ ] Render Progress, Parent completion, Audit pipeline, Delivery health, Recent activity in the first viewport.
- [ ] Existing tree/worklog/insights content is moved into pages.
- [ ] Commit `feat(viewer): add control tower navigation`.

### Task 6: Kanban UI and ticket drawer

- [ ] Render four Kanban columns.
- [ ] Cards include ticket id, parent, category, task, blocker/claim/evidence badges, branch.
- [ ] Clicking a card opens a detail drawer with latest/history/worklogs.
- [ ] Commit `feat(viewer): add ticket kanban view`.

### Task 7: responsive polish and smoke

- [ ] Verify desktop and mobile widths with Browser/in-app browser screenshots or equivalent HTTP/static smoke.
- [ ] Check no text overlap, no horizontal overflow, and no blank primary panels.
- [ ] Run:
  ```
  go test ./... -count=1
  go vet ./...
  gofmt -l .
  go list -m all
  ```
- [ ] Confirm `go list -m all` still shows no external deps.
- [ ] Commit `test(viewer): smoke control tower UI`.

---

## Self-Review Checklist

- [ ] Dashboard answers "what is the project state?" within first viewport.
- [ ] The redesigned shell feels like an operational control tower, not a generic generated dashboard.
- [ ] Parent completion is visible without opening the tree.
- [ ] Kanban makes Plan/Implement/Verify/Complete bottlenecks obvious.
- [ ] `audit_ready` and `changes_requested` are visible and not collapsed into generic open.
- [ ] Invalidated rows are hidden from normal lists but counted in health/insights.
- [ ] No text overlap at narrow widths.
- [ ] Empty/error/loading states are intentionally designed.
- [ ] Existing Plan 3 endpoints remain backward compatible.

---

## Out of Scope

- Drag/drop status changes.
- Editing ledger rows from UI.
- Auth, sharing, hosted mode.
- Websocket/SSE.
- Charting libraries.
