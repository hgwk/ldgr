# ldgr Viewer Ops Surface Follow-up Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Status:** Deferred follow-up after Viewer Control Tower.

**Goal:** Make `ldgr view` a stronger day-to-day control surface for multi-agent operations: richer drawer detail, owner/claim visibility, audit queue, lifecycle latency, activity, verify state, search, and dependency graph hints.

**Context:** Viewer Control Tower delivered dashboard, kanban, tree, worklog, insights, filtering, sorting, and bounded panes. Real use now shows that humans need faster answers to: who owns this, what did audit say, what is stale, what needs audit now, what changed recently, and which blocker/path clusters matter.

---

## Hard Acceptance Criteria

1. No new canonical ledger files are introduced.
2. Viewer remains read-only.
3. Backend stays Go standard-library only; frontend remains vanilla JS/CSS with no build step.
4. Dashboard/kanban/drawer surfaces expose ownership, audit, provenance, and stale-claim signals without requiring raw JSON inspection.
5. Audit-ready work is visible as a first-class queue.
6. Metrics are derived from append-only rows and latest-row semantics, never by mutating historical rows.
7. Long content stays inside bounded panes; page-level scroll does not become the primary navigation.
8. `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .`, and `node --check internal/viewer/assets/app.js` pass.

---

## Plan A — Drawer & Owner Surface

Immediate value: richer ticket understanding and visible ownership.

### Task A1: drawer detail fields

- [ ] Extend ticket detail drawer to show `audit_notes`, `acceptance`, `decision`, `notes`, provenance markers, `claimed_by`, `claim_until`, `handoff_to`, `handoff`, and `reviewed_n`.
- [ ] Render missing optional fields quietly; do not show empty noise.
- [ ] For `reviewed_n`, add a backlink to the referenced ticket history row when present.
- [ ] Add focused aggregate/viewer tests where projection support is needed.
- [ ] Commit `feat(viewer): enrich ticket drawer details`.

### Task A2: owner badges on cards

- [ ] Add kanban card owner badge from `claimed_by` or latest `agent`.
- [ ] Show stale/expired claim state when `claim_until` is past.
- [ ] Keep card density high; owner badge must not wrap the card layout badly.
- [ ] Commit `feat(viewer): show ticket ownership on kanban cards`.

### Task A3: stale claims dashboard tile

- [ ] Add Dashboard tile for expired and near-expiring claims.
- [ ] Use latest ticket rows only; terminal `done/cancelled` tickets are excluded.
- [ ] Near-expiring threshold should be a small constant first, not configurable.
- [ ] Commit `feat(viewer): surface stale claims`.

### Task A4: drawer polish and bounded panes

- [ ] Ensure long notes/handoff/audit text scrolls inside drawer sections.
- [ ] Verify mobile drawer still fits and content remains reachable.
- [ ] Commit `fix(viewer): bound long drawer content`.

---

## Plan B — Audit Queue & Lifecycle Metrics

Immediate value: make verification work visible and measure flow.

### Task B1: audit page

- [ ] Add `/audit` viewer page and sidebar item.
- [ ] Show `audit_ready` tickets sorted by priority then age.
- [ ] Reuse ticket detail drawer on row/card click.
- [ ] Empty state should distinguish no audit work from load failure.
- [ ] Commit `feat(viewer): add audit queue page`.

### Task B2: lifecycle latency projection

- [ ] Compute per-ticket cycle time from first active row to audit-pass `done`.
- [ ] Compute audit latency from latest `audit_ready` to audit-pass `done` or now.
- [ ] Add aggregate metrics to dashboard projection.
- [ ] Tests must cover repeated status rows, changes_requested loops, and cancelled tickets.
- [ ] Commit `feat(viewer): compute lifecycle latency metrics`.

### Task B3: dashboard lifecycle tiles

- [ ] Add dashboard tiles for cycle time and audit latency.
- [ ] Keep tile labels operational and compact.
- [ ] Avoid precision that implies false accuracy; days/hours buckets are enough.
- [ ] Commit `feat(viewer): show lifecycle metrics`.

### Task B4: kanban age color

- [ ] Add age tone to kanban cards:
  - `in_progress` 5d+ is stale.
  - `audit_ready` 2d+ is audit-stale.
- [ ] Use subtle badges/borders, not full-card alarm coloring.
- [ ] Commit `feat(viewer): highlight stale lifecycle age`.

---

## Plan C — Activity & Verify Surface

Medium effort: connect live collaboration and ledger health.

### Task C1: active agents widget

- [ ] Add Dashboard widget for active agents based on last 24h ticket/worklog rows.
- [ ] Group by actor and role, with row counts and latest timestamp.
- [ ] Exclude empty/missing agent values from the main list; surface them as a small unknown count if needed.
- [ ] Commit `feat(viewer): show active agents`.

### Task C2: verify endpoint

- [ ] Add read-only `GET /api/projects/{project_id}/verify`.
- [ ] Support strict/default mode through query param, e.g. `?strict=1`.
- [ ] Return grouped issue counts and a short recent issue sample; do not stream full noisy output by default.
- [ ] Tests cover success, strict warnings promoted to fail, and missing project.
- [ ] Commit `feat(viewer): expose verify status endpoint`.

### Task C3: verify dashboard widget

- [ ] Add Dashboard "Verify status" widget with default/strict toggle.
- [ ] Show fail/warn counts by code.
- [ ] Never block the rest of dashboard if verify calculation fails; show local error in the widget.
- [ ] Commit `feat(viewer): show verify status`.

---

## Plan D — Search & Graph

Design-heavy follow-up. Do after a separate UI review.

### Task D1: global search

- [ ] Add global search focus shortcut `/`.
- [ ] Search ticket id, task, notes, decision, audit_notes, handoff, paths, and worklog result.
- [ ] Keep search local/in-memory; no index file.
- [ ] Results should deep-link to drawer/page context.
- [ ] Commit `feat(viewer): add global search`.

### Task D2: blocker chain mini-diagram

- [ ] Add mini dependency diagram in ticket drawer from `blocked_by` chains.
- [ ] Keep it small and textual/HTML-based; no graph dependency.
- [ ] Detect missing blocker ids and cycles as warnings.
- [ ] Commit `feat(viewer): show blocker chains`.

### Task D3: paths heatmap

- [ ] Add dashboard tile showing path prefixes touched by active/recent tickets.
- [ ] Use simple prefix buckets; no expensive file-system scan.
- [ ] Clicking a bucket filters kanban/search when feasible.
- [ ] Commit `feat(viewer): add paths heatmap`.

### Task D4: design review pass

- [ ] Review Dashboard, Audit, Drawer, Search, and Graph surfaces as one control room flow.
- [ ] Remove duplicated signals and noisy labels.
- [ ] Verify desktop and mobile screenshots manually before closing the plan.
- [ ] Commit `design(viewer): refine ops surface`.

---

## Self-Review Checklist

- [ ] A maintainer can answer "who owns this?" from kanban without opening raw JSON.
- [ ] An auditor can find the next audit target from the sidebar in one click.
- [ ] A stale claim or stale audit-ready ticket is visible on the dashboard.
- [ ] Drawer contains enough history/provenance to audit the decision path.
- [ ] Verify state is visible without running a shell command.
- [ ] Search and graph features improve navigation without making the viewer feel like a generic issue tracker.
- [ ] No external dependencies or generated frontend build artifacts.

---

## Out of Scope

- Editing ledger rows from the viewer.
- Authentication or hosted dashboard.
- New canonical files beyond `config.json`, `goal.json`, `tickets.jsonl`, and `worklog.jsonl`.
- Replacing CLI verify/workflow gates with viewer-only checks.
