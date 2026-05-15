# ldgr Post-v0 Pipeline Hardening Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Status:** Deferred until after v0.1.0. v0.1.0 has shipped; run this after or alongside the audit-gate enforcement plan.

**Goal:** Tighten the whole `ldgr` adoption pipeline after real use in `agent-zero`: migration, install/PATH, verifier signal quality, write-policy consistency, and docs/instructions truthfulness. This plan does not change the four-file ledger model.

**Context:** The v0.1.0 pipeline works end-to-end, but review found several friction points:
- legacy import can produce ledger rows without a real config/registry install step;
- `ldgr verify` is too noisy on historical projects;
- worklog ticket linkage is still optional in the model while the product expects ticket-linked delivery rows;
- instructions describe some lifecycle behavior as enforced before all enforcement exists;
- installed repos may have `ldgr` unavailable on `PATH`, causing wrapper scripts to fail.

---

## Hard Acceptance Criteria

1. Legacy migration has a single documented happy path that leaves the target repo initialized, registered, and viewable.
2. `ldgr import legacy` either requires an existing config or offers an explicit `--init` / `--register` path; no silent `import-stub` final state.
3. README migration examples include `ldgr init`, `ldgr import legacy --plan`, `--apply`, `ldgr verify`, and `ldgr view --target .`.
4. `ldgr verify` gains concise summary output so large historical ledgers do not bury important issues.
5. `ldgr verify` supports warning grouping by stable code.
6. `ldgr verify --strict` remains a release gate, while default verify stays usable for imported historical ledgers.
7. New `ldgr worklog add` writes require a non-empty `ticket`; legacy/imported missing-ticket rows remain verify warnings, not immediate migration blockers.
8. Instructions accurately distinguish enforced rules from guidance-only behavior.
9. Install docs explain how to make `ldgr` available on `PATH` after `go install`, and release docs describe binary install.
10. Implementation-ticket guidance requires a stable provenance note shape:
    `archived=<path or none>; borrow=<path or none>; reference=<path or none>; new=<delta + why>`.
11. Guidance documents explain `not_borrowed=<why>` for references that were reviewed but intentionally skipped.
12. `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean.

---

## Task Granularity

8 tasks. Each task is one TDD cycle and should end with a commit.

### Task 1: migration happy path

- [ ] Update README migration section with full sequence:
  ```
  ldgr init
  ldgr import legacy --plan
  ldgr import legacy --apply
  ldgr verify
  ldgr view --target .
  ```
- [ ] Document when `--archive-originals` is safe.
- [ ] Add note that `ldgr init` registers the project for multi-project view.
- [ ] Commit `docs(import): document initialized migration path`.

### Task 2: import config/registry guard

- [ ] Change `import legacy --apply` so a missing `ledger/config.json` is not silently left as `import-stub`.
- [ ] Add explicit behavior: either fail with "run ldgr init first" or add `--init` to create config/registry before apply.
- [ ] Tests for missing config, existing config, and idempotent apply.
- [ ] Commit `fix(import): require real project config before apply`.

### Task 3: verify warning codes and summary

- [ ] Add stable issue codes to `verify.Issue`.
- [ ] Group warning output by code with counts.
- [ ] Add `--summary` or make default output concise while preserving full output with `--verbose`.
- [ ] Tests for grouped missing-category/orphan/invalidated warnings.
- [ ] Commit `feat(verify): group warnings by issue code`.

### Task 4: historical ledger noise controls

- [ ] Add `--new-only` or equivalent mode for checking rows appended after a known baseline.
- [ ] Use split baselines for independent JSONL streams, e.g. `--since-ticket-n` and `--since-worklog-n`; a single `--since-n` may remain only as shorthand for small projects.
- [ ] Keep default verify backward compatible.
- [ ] Document recommended CI mode for new projects vs imported projects.
- [ ] Commit `feat(verify): support scoped historical checks`.

### Task 5: worklog ticket policy

- [ ] Make `ldgr worklog add` require non-empty `ticket` for new writes.
- [ ] Keep verifier tolerant of historical/imported missing-ticket rows as warnings unless `--strict`.
- [ ] Update instructions and README examples.
- [ ] Commit `fix(worklog): require ticket on new delivery rows`.

### Task 6: instruction truthfulness pass

- [ ] Audit `cmd/instructions/*.ldgr.md` against actual CLI behavior.
- [ ] Replace "binary will enforce" wording where behavior is guidance-only until audit gate plan lands.
- [ ] After audit gate lands, update wording again to exact enforcement.
- [ ] Tests for installed instruction bodies if snapshots exist.
- [ ] Commit `docs(instructions): align wording with current enforcement`.

### Task 7: install/PATH polish

- [ ] README explains where `go install` places the binary (`$(go env GOPATH)/bin` when `GOBIN` is empty).
- [ ] Add a quick check:
  ```
  command -v ldgr || echo 'add $(go env GOPATH)/bin to PATH'
  ```
- [ ] Release install section covers downloading a tarball and moving `ldgr` into a PATH directory.
- [ ] Optional: add `ldgr doctor` plan note if not implemented here.
- [ ] Commit `docs(install): clarify PATH setup`.

### Task 8: implementation provenance guidance

- [ ] Update spec guidance so implementation tickets use this minimum `notes` shape:
  ```
  archived=<path or none>; borrow=<path or none>; reference=<path or none>; new=<delta + why>
  ```
- [ ] Update `cmd/instructions/AGENTS.ldgr.md` and `cmd/instructions/CLAUDE.ldgr.md` with the same shape.
- [ ] Explain `not_borrowed=<why>` for references that are stack-specific, domain-specific, or otherwise intentionally skipped.
- [ ] Add/adjust instruction-body tests if snapshots exist; otherwise cover through install smoke.
- [ ] Commit `docs(instructions): require implementation provenance notes`.

---

## Self-Review Checklist

- [ ] A new project can go from zero to initialized ledger, instructions, hook, and viewer without reading source.
- [ ] A legacy project can migrate without ending in an unregistered/import-stub state.
- [ ] Verify output remains actionable on large historical ledgers.
- [ ] New writes are stricter than legacy rows, without breaking imported history.
- [ ] Installed instructions do not overpromise enforcement.
- [ ] New implementation tickets carry archived/borrow/reference/new provenance before code work begins.
- [ ] Shell scripts in target repos do not fail just because `ldgr` is outside `PATH`.
- [ ] No external dependencies.

---

## Out of Scope

- Audit lifecycle hard enforcement; see `2026-05-14-ldgr-audit-gate-enforcement.md`.
- Homebrew tap automation.
- MCP server.
- Hosted dashboard/auth.
- Changing `ledger/goal.json`, `ledger/tickets.jsonl`, `ledger/worklog.jsonl`, `ledger/config.json` as the only canonical files.
