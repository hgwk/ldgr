# ldgr Audit Gate Enforcement Follow-up Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Status:** Deferred until after v0.1.0. v0.1.0 has shipped; schedule this as the first lifecycle-hardening follow-up.

**Goal:** Stop agents from skipping the audit phase. `ldgr` should make the correct path easier than the wrong path: implementation work moves to `audit_ready`, independent audit rows move to `done` with `audit_result=pass`, and worklogs/commit suggestions only unlock after audit pass.

**Problem:** The current lifecycle is documented and guided, but not hard enough. Agents can still append `status=done` from implementation context or create worklog rows before an audit-pass row. This is not model confidence; it is workflow friction plus weak enforcement.

**Architecture:**
- Keep the ledger append-only. Enforcement happens before writing new rows.
- Keep pure state-machine logic in `internal/guidance` or a small `internal/lifecycle` package.
- Reuse existing `ldgr next` / `ldgr suggest` surfaces so agents see the required next action.
- Add strict verifier checks behind policy/config first, then make them default if migration noise is acceptable.
- No new ledger files.

---

## Hard Acceptance Criteria

1. `ldgr ticket event` rejects direct implementation `done` unless the row is an audit pass row:
   - `status=done`
   - `role=audit`
   - `audit_result=pass`
   - non-empty `evidence`
2. `ldgr worklog add` rejects worklog rows unless latest ticket row is audit-pass done.
3. `ldgr suggest worklog --ticket ID` refuses to emit a worklog skeleton before audit pass and prints the audit requirement instead.
4. `ldgr suggest commit --ticket ID` refuses commit scaffold before audit pass unless an explicit override flag is supplied.
5. `ldgr next --ticket ID` strongly points `in_progress` implementation rows to `audit_ready`.
6. Add convenience commands or subcommands for the common path:
   - `ldgr ticket ready --ticket ID --evidence TEXT`
   - `ldgr audit pass --ticket ID --evidence TEXT`
   - `ldgr audit request-changes --ticket ID --notes TEXT`
7. `ldgr verify --strict` fails weak done rows and premature worklog rows.
8. Default `ldgr verify` reports weak historical rows as warnings, not hard failures, unless config opts into strict enforcement.
9. README and instruction bodies describe the enforced lifecycle with exact commands.
10. `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean.

---

## Lifecycle Policy

Default lifecycle:

```text
open -> in_progress -> audit_ready -> done
                   \-> changes_requested -> in_progress
```

Valid close row:

```json
{
  "ticket": "BUG-101",
  "role": "audit",
  "status": "done",
  "audit_result": "pass",
  "evidence": ["go test ./...", "go vet ./..."]
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
impl delivery cannot move directly to done.
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

- [ ] Add pure validator for proposed ticket event rows.
- [ ] Reject impl/direct `done`.
- [ ] Accept audit-pass `done` with evidence.
- [ ] Accept `audit_ready`, `changes_requested`, and normal in-progress transitions.
- [ ] Commit `feat(lifecycle): validate audit gate transitions`.

### Task 2: enforce in write commands

- [ ] Wire validator into `ticket event`.
- [ ] Ensure rejection happens before JSONL append.
- [ ] Error message includes exact next command.
- [ ] Tests prove no write occurs on invalid transition.
- [ ] Commit `fix(ticket): reject direct done without audit pass`.

### Task 3: gate worklog writes

- [ ] Before `worklog add`, load latest ticket row.
- [ ] Reject missing ticket and non-audit-pass latest state.
- [ ] Keep append-only semantics for valid worklog rows.
- [ ] Tests for missing ticket, audit_ready, changes_requested, weak done, audit-pass done.
- [ ] Commit `fix(worklog): require audit pass before delivery log`.

### Task 4: strengthen suggest/next guidance

- [ ] `next` for `in_progress` implementation rows recommends `audit_ready`.
- [ ] `suggest worklog` refuses skeleton before audit pass.
- [ ] `suggest commit` refuses scaffold before audit pass unless `--allow-unaudited`.
- [ ] Commit `feat(guidance): gate suggestions on audit pass`.

### Task 5: convenience commands

- [ ] Add `ticket ready --ticket --evidence`.
- [ ] Add `audit pass --ticket --evidence`.
- [ ] Add `audit request-changes --ticket --notes`.
- [ ] Commands still append ordinary JSONL rows, no hidden mutation.
- [ ] Commit `feat(audit): add lifecycle shortcut commands`.

### Task 6: verifier strictness

- [ ] `verify --strict` fails weak done rows.
- [ ] `verify --strict` fails worklog rows whose latest preceding ticket state is not audit-pass done.
- [ ] Non-strict verify warns for historical/imported violations.
- [ ] Commit `feat(verify): enforce audit gate in strict mode`.

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

- [ ] A model cannot accidentally close implementation work as `done`.
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
