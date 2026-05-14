# ledger-kit Hooks, Instructions, Guidance, and Release Plan (Plan 4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make ledger-kit installable and hard to misuse in real agent workflows: git hooks enforce verification, AGENTS/CLAUDE integration exposes the right commands, contextual guidance tells LLMs the next action at each ticket state, and release automation produces prebuilt binaries.

**Architecture:**
- `cmd/hooks.go` installs/uninstalls idempotent hook marker blocks.
- `cmd/instructions.go` installs ledger-owned instruction bodies plus small reference markers.
- `internal/guidance/` computes state-aware next actions and JSON skeletons from latest ticket rows.
- `cmd/next.go` and `cmd/suggest.go` expose guidance for humans and LLMs.
- Existing `cmd/ticket.go` / `cmd/worklog.go` call `guidance` after successful writes and print guidance to stderr, leaving stdout JSON stable.
- `.github/workflows/release.yml` builds release artifacts.

**Tech Stack:** Go 1.22+ stdlib only. Frontend/build tooling is not involved.

**Spec reference:** `docs/superpowers/specs/2026-05-14-ledger-kit-go-design.md` §4.2.4, §4.3.1, §4.3.2, §8, §9, §12.

---

## Hard Acceptance Criteria

1. `ledger-kit hooks install` inserts a pre-commit marker block near the top of `.git/hooks/pre-commit`, preserves existing hook content, creates a backup, and is idempotent.
2. `ledger-kit hooks uninstall` removes only the ledger-kit marker block and preserves user hook content.
3. `ledger-kit instructions install` creates `ledger/instructions/AGENTS.ledger-kit.md` and `ledger/instructions/CLAUDE.ledger-kit.md`, then prepends small marker pointers to `AGENTS.md` / `CLAUDE.md` without duplicating blocks.
4. Default instruction mode is reference mode. Inline mode exists for compatibility. Symlink mode remains out of scope unless explicitly implemented later.
5. `ledger-kit next --ticket ID` prints state-aware next actions for `open`, `in_progress`, `blocked`, `audit_ready`, `changes_requested`, `done`, and `cancelled`.
6. `ledger-kit next --ticket ID --format json` returns machine-readable actions, warnings, and suggested commands.
7. `ledger-kit suggest worklog --ticket ID` prints a worklog JSON skeleton only when the latest ticket row is an audit-pass `done` row. Otherwise it prints guidance instead of a misleading skeleton.
8. `ledger-kit suggest commit --ticket ID` prints a Conventional Commit line plus PR summary / verification skeleton derived from latest ticket fields, paths, commands/evidence where available.
9. `ticket add`, `ticket event`, and `worklog add` keep stdout as normalized row JSON and print contextual guidance to stderr after successful writes.
10. Guidance never writes ledger files. It only reads current state and suggests next commands/JSON.
11. Release workflow builds darwin/linux arm64/amd64 artifacts and runs `go test ./...`, `go vet ./...`, and `gofmt -l .` before packaging.

---

## Decisions Locked

- **State machine owns guidance**: reminders are derived from latest ticket status, not from a generic static prompt.
- **No extra ledger file**: guidance, audit, and suggestions use existing `tickets.jsonl` / `worklog.jsonl`.
- **stdout remains parseable**: write commands continue to emit only the normalized row JSON on stdout. Human/LLM guidance goes to stderr.
- **`done` means audit pass**: guidance must steer implementation completion to `audit_ready`, not directly to `done`.
- **Worklog after audit**: `suggest worklog` and write-command guidance must reinforce that worklog follows audit-pass `done`.
- **Reference instructions by default**: avoid long prose injection into AGENTS/CLAUDE; install ledger-owned instruction bodies and short pointers.
- **Idempotent installers**: repeated hooks/instructions installs are no-ops except for missing generated instruction files.

---

## File Structure

```
ledger-kit/
  cmd/
    hooks.go hooks_test.go
    instructions.go instructions_test.go
    next.go next_test.go
    suggest.go suggest_test.go
  internal/
    guidance/
      guidance.go guidance_test.go
      suggest.go suggest_test.go
  templates/
    instructions/
      AGENTS.ledger-kit.md
      CLAUDE.ledger-kit.md
  .github/workflows/release.yml
```

---

## Guidance Contract

### `next`

Input: latest ticket state for `--ticket`.

Output text sections:
- current status summary
- required next action
- warnings
- suggested command(s)

Output JSON shape:

```json
{
  "ticket": "example-ticket",
  "status": "audit_ready",
  "actions": ["do_not_append_worklog", "append_audit_row"],
  "warnings": [],
  "suggested_commands": ["ledger-kit ticket event --json @-"],
  "suggested_json": [
    {
      "ticket": "example-ticket",
      "role": "audit",
      "status": "done",
      "audit_result": "pass",
      "evidence": []
    }
  ]
}
```

### State-specific guidance

- `open`: confirm acceptance, paths/write set, parent/category, archive/reference review before implementation.
- `in_progress`: confirm active claim, avoid overlapping paths, keep evidence as commands run.
- `blocked`: show unresolved blockers and tell the agent not to implement until dependencies move.
- `audit_ready`: do not append worklog; append `role=audit` row with pass or changes requested.
- `changes_requested`: do not append worklog; resume with `in_progress`, carry audit notes into implementation notes.
- `done`: if audit-pass, suggest worklog and commit/PR; if not audit-pass, warn that closure is weak.
- `cancelled`: do not append worklog unless cancellation itself is the completed delivery; explain reason in notes.

---

## Task Granularity

7 tasks. Each is one TDD cycle (failing test → implementation → pass → commit).

### Task 1: hooks installer

- [ ] Tests: install into missing hook, existing shebang hook, hook without shebang, idempotent reinstall, uninstall preserves user content.
- [ ] Implement `cmd/hooks.go`.
- [ ] Verify `go test ./cmd`.
- [ ] Commit `feat(hooks): install ledger verify pre-commit hook`.

### Task 2: instruction templates and installer

- [ ] Tests: creates instruction bodies, prepends AGENTS/CLAUDE markers, idempotent reinstall, uninstall removes only markers.
- [ ] Implement `cmd/instructions.go` and templates.
- [ ] Instruction bodies include append-only, latest-row-wins, audit-before-done, worklog-after-audit, `ledger-kit next`, `ledger-kit verify`.
- [ ] Commit `feat(instructions): install ledger agent guidance`.

### Task 3: guidance engine

- [ ] Tests for all ticket statuses and audit-pass vs weak done.
- [ ] Implement `internal/guidance`.
- [ ] Ensure output is deterministic and has text + JSON forms.
- [ ] Commit `feat(guidance): derive next actions from ticket state`.

### Task 4: `next` command

- [ ] Tests: missing ticket fails, text output, JSON output.
- [ ] Implement `cmd/next.go`.
- [ ] Commit `feat(next): show ticket next actions`.

### Task 5: `suggest` commands

- [ ] Tests: worklog skeleton only after audit-pass `done`; commit skeleton includes type/scope guess, summary, verification block.
- [ ] Implement `cmd/suggest.go`.
- [ ] Commit `feat(suggest): generate worklog and commit skeletons`.

### Task 6: write-command guidance integration

- [ ] Tests: `ticket event status=audit_ready` keeps stdout JSON and emits stderr guidance; `worklog add` warns when ticket is not audit-pass done.
- [ ] Wire `guidance` into `ticket add`, `ticket event`, `worklog add`.
- [ ] Commit `feat(ticket): print contextual guidance after writes`.

### Task 7: release workflow and final smoke

- [ ] Add `.github/workflows/release.yml` with test/vet/gofmt gates and darwin/linux arm64/amd64 builds.
- [ ] Release artifacts ship under two names:
      - Primary binary: `ledger-kit`
      - Short alias: `ldgr` (symlink in the same artifact archive, plus a copy of the binary for OSes without symlink-aware archives).
- [ ] Future Homebrew formula publishes both names so `brew install hgwk/tap/ledger-kit` exposes `ledger-kit` and `ldgr` on `$PATH`.
- [ ] Run:
  ```
  go test ./... -count=1 -race
  go vet ./...
  gofmt -l .
  ```
- [ ] Commit `chore(release): add binary release workflow`.

---

## Self-Review Checklist

- [ ] Hooks install/uninstall are idempotent and preserve existing user hooks.
- [ ] Instructions install/uninstall are idempotent and preserve existing AGENTS/CLAUDE content.
- [ ] `next` covers every status in `StatusEnum`.
- [ ] `suggest worklog` refuses to produce a skeleton before audit-pass `done`.
- [ ] Write commands keep stdout machine-parseable JSON.
- [ ] Guidance goes to stderr and contains the next concrete command.
- [ ] Full test/race/vet/gofmt clean.

---

## Out of Scope

- MCP server.
- TUI.
- Automatic PR creation or git commit execution.
- Symlink instruction mode unless explicitly requested later.
- Enforcing broad-change plan audit from git diff heuristics; `doctor` can warn, but hard enforcement is deferred.
- Viewer control-tower / Kanban redesign; deferred to `docs/superpowers/plans/2026-05-14-ldgr-viewer-control-tower.md`.
