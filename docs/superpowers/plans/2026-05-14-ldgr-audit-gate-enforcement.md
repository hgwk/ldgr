# ldgr Lifecycle State Machine Enforcement Follow-up Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Status:** Deferred until after v0.1.0. v0.1.0 has shipped; schedule this as the first lifecycle-hardening follow-up.

**Goal:** Stop agents from skipping workflow stages by making the lifecycle a first-class state machine. `ldgr` should make the correct path easier than the wrong path: plan rows can move into implementation, implementation rows can move only to verify/cancel/wait states, verify rows can pass to done or append audit feedback, and worklogs/commit suggestions only unlock after verify pass.

**Problem:** The current lifecycle is documented and guided, but not hard enough. Agents can still append `status=done` from plan/implementation context or create worklog rows before an audit-pass row. This is not model confidence; it is workflow friction plus weak enforcement.

**Architecture:**
- Keep the ledger append-only. Enforcement happens before writing new rows.
- Keep pure state-machine logic in a small `internal/lifecycle` package.
- Model audit feedback as append-only ticket event rows. Do not edit implementation rows to add review comments.
- Reuse existing `ldgr next` / `ldgr suggest` surfaces so agents see the required next action.
- Add strict verifier checks behind policy/config first, then make them default if migration noise is acceptable.
- No new ledger files.

---

## Hard Acceptance Criteria

1. `ldgr ticket add` accepts only initial states that do not imply completed work: `open`, `blocked`, or `cancelled`.
2. `ldgr ticket event` enforces the allowed transition graph:
   - `open -> in_progress | blocked | cancelled`
   - `in_progress -> audit_ready | blocked | cancelled`
   - `blocked -> in_progress | cancelled`
   - `audit_ready -> done | changes_requested | cancelled`
   - `changes_requested -> in_progress | open | cancelled`
   - `done` and `cancelled` are terminal except append-only correction/invalidation rows.
3. `ldgr ticket event` rejects `done` unless the row is a verify/audit pass row:
   - `status=done`
   - `role=audit`
   - `audit_result=pass`
   - non-empty `evidence`
   - `reviewed_n` points at the `audit_ready` row being approved.
4. `ldgr ticket event` rejects `changes_requested` unless the row is an audit feedback row:
   - previous latest status is `audit_ready`
   - `role=audit`
   - `audit_result=changes_requested`
   - non-empty `audit_notes`
   - `reviewed_n` points at the `audit_ready` row being reviewed.
5. `ldgr ticket event` rejects `audit_ready` unless the previous latest status is `in_progress`, `blocked`, or `changes_requested`, and the row includes non-empty `evidence`.
6. `ldgr worklog add` rejects worklog rows unless latest ticket row is audit-pass done.
7. `ldgr suggest worklog --ticket ID` refuses to emit a worklog skeleton before audit pass and prints the audit requirement instead.
8. `ldgr suggest commit --ticket ID` refuses commit scaffold before audit pass unless an explicit override flag is supplied.
9. `ldgr next --ticket ID` points each stage at the only valid next transitions.
10. Add convenience commands or subcommands for the common path:
   - `ldgr ticket ready --ticket ID --evidence TEXT`
   - `ldgr audit pass --ticket ID --evidence TEXT`
   - `ldgr audit request-changes --ticket ID --notes TEXT`
11. `ldgr verify --strict` fails invalid transitions, weak done rows, audit feedback without `reviewed_n`, and premature worklog rows.
12. Default `ldgr verify` reports weak historical rows as warnings, not hard failures, unless config opts into strict enforcement.
13. README and instruction bodies describe the enforced lifecycle with exact commands.
14. `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean.

---

## Lifecycle Policy

Default lifecycle stages:

```text
plan -> impl -> verify -> done
```

Current status mapping:

| Logical stage | Current `status` |
|---|---|
| plan | `open` |
| impl | `in_progress` |
| verify | `audit_ready` |
| changes requested | `changes_requested` |
| wait | `blocked` |
| complete | `done` |
| terminal cancellation | `cancelled` |

Allowed status transitions:

```text
open -> in_progress
open -> blocked
open -> cancelled

in_progress -> audit_ready
in_progress -> blocked
in_progress -> cancelled

blocked -> in_progress
blocked -> cancelled

audit_ready -> done
audit_ready -> changes_requested
audit_ready -> cancelled

changes_requested -> in_progress
changes_requested -> open
changes_requested -> cancelled

done -> terminal
cancelled -> terminal
```

Correction/invalidation rows are the only terminal-state exception. They must be explicit ops rows, for example `role=ops` plus `invalidates_n`.

### Append-only audit opinion rows

Audit opinions are ordinary ticket event rows. They do not mutate the implementation row they review.

Implementation ready for verify:

```json
{
  "ticket": "BUG-101",
  "role": "impl",
  "status": "audit_ready",
  "evidence": ["go test ./...", "go vet ./..."]
}
```

Audit requests changes:

```json
{
  "ticket": "BUG-101",
  "role": "audit",
  "status": "changes_requested",
  "audit_result": "changes_requested",
  "audit_notes": "Missing regression coverage for invalid input.",
  "evidence": ["go test ./..."],
  "reviewed_n": 11
}
```

Implementation resumes by appending another row:

```json
{
  "ticket": "BUG-101",
  "role": "impl",
  "status": "in_progress",
  "notes": "Addressing audit feedback from n=12."
}
```

Audit passes:

```json
{
  "ticket": "BUG-101",
  "role": "audit",
  "status": "done",
  "audit_result": "pass",
  "audit_notes": "Verified implementation and regression coverage.",
  "evidence": ["go test ./...", "go vet ./..."],
  "reviewed_n": 14
}
```

Invalid close row:

```json
{
  "ticket": "BUG-101",
  "role": "impl",
  "status": "done"
}
```

The invalid row should be rejected before append with a precise error:

```text
there is no lifecycle edge from in_progress to done.
Use status=audit_ready first, then run:
  ldgr next --ticket BUG-101
```

---

## Config Policy

Add optional lifecycle policy to `ledger/config.json`:

```json
{
  "lifecycle": {
    "require_audit_for_done": true,
    "require_evidence_for_audit_pass": true,
    "require_reviewed_n_for_audit": true,
    "require_audit_pass_for_worklog": true,
    "allow_commit_suggestion_before_audit": false
  }
}
```

Defaults should be conservative for new ledgers. For imported legacy ledgers, weak historical rows remain warnings unless `--strict` is used.

---

## Task Granularity

7 tasks. Each task is one TDD cycle and should end with a commit.

### Task 1: lifecycle state validator

- [ ] Add `internal/lifecycle`.
- [ ] Model the allowed status transition graph.
- [ ] Validate proposed ticket add/event rows against previous latest row.
- [ ] Reject plan/impl direct `done` because that edge does not exist.
- [ ] Allow explicit ops correction/invalidation rows without opening terminal states for normal workflow.
- [ ] Commit `feat(lifecycle): validate ticket state transitions`.

### Task 2: enforce in write commands

- [ ] Wire validator into `ticket add` and `ticket event`.
- [ ] Ensure rejection happens before JSONL append.
- [ ] Error message includes exact next command.
- [ ] Tests prove no write occurs on invalid transition.
- [ ] Commit `fix(ticket): enforce lifecycle transitions before append`.

### Task 3: gate worklog writes

- [ ] Before `worklog add`, load latest ticket row.
- [ ] Reject missing ticket and non-audit-pass latest state.
- [ ] Keep append-only semantics for valid worklog rows.
- [ ] Tests for missing ticket, audit_ready, changes_requested, weak done, audit-pass done.
- [ ] Commit `fix(worklog): require audit pass before delivery log`.

### Task 4: strengthen suggest/next guidance

- [ ] `next` for each status shows only valid next transitions.
- [ ] `suggest worklog` refuses skeleton before audit pass.
- [ ] `suggest commit` refuses scaffold before audit pass unless `--allow-unaudited`.
- [ ] Commit `feat(guidance): gate suggestions on audit pass`.

### Task 5: convenience commands

- [ ] Add `ticket ready --ticket --evidence`.
- [ ] Add `audit pass --ticket --evidence`; it sets `reviewed_n` to the latest `audit_ready` row.
- [ ] Add `audit request-changes --ticket --notes`; it sets `reviewed_n` to the latest `audit_ready` row.
- [ ] Commands still append ordinary JSONL rows, no hidden mutation.
- [ ] Commit `feat(audit): add lifecycle shortcut commands`.

### Task 6: verifier strictness

- [ ] `verify --strict` fails invalid transition edges.
- [ ] `verify --strict` fails weak done rows.
- [ ] `verify --strict` fails audit pass / changes-requested rows missing `reviewed_n`.
- [ ] `verify --strict` fails worklog rows whose latest preceding ticket state is not audit-pass done.
- [ ] Non-strict verify warns for historical/imported violations.
- [ ] Commit `feat(verify): enforce lifecycle graph in strict mode`.

### Task 7: docs, instructions, smoke

- [ ] Update README lifecycle examples.
- [ ] Update `cmd/instructions/*.ldgr.md` with shortcut commands and no-direct-done rule.
- [ ] Run:
  ```
  go test ./... -count=1
  go test -race ./...
  go vet ./...
  gofmt -l .
  ```
- [ ] Commit `docs(audit): document enforced lifecycle`.

---

## Self-Review Checklist

- [ ] A model cannot accidentally skip plan/impl/verify stages.
- [ ] Audit feedback is represented as append-only ticket rows with `reviewed_n`.
- [ ] Worklog cannot be written before audit pass.
- [ ] The happy path is shorter than manual JSON row authoring.
- [ ] Error messages provide the next exact command.
- [ ] Historical ledgers remain usable without immediate migration.
- [ ] Strict mode gives maintainers a release gate.
- [ ] No external dependencies.

---

## Out of Scope

- Multi-user auth or reviewer identity verification.
- Hosted PR integration.
- Automatic code review.
- UI editing/drag-drop status changes.
- Changing the four-file ledger model.
