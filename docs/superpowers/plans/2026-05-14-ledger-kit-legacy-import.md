# ledger-kit Legacy Import Implementation Plan (Plan 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Implement `ledger-kit import legacy --plan|--apply` per spec §11 so existing repos with the old root-level `agent-tickets.jsonl`/`agent-worklog.jsonl`/`goal.json` (or earlier `ledger/*.jsonl` layouts) can be migrated into the new standard with zero data loss.

**Architecture:** A new `internal/legacy` package holds scan/normalize/plan/apply logic. `cmd/import.go` exposes it as `ledger-kit import legacy [...]`. All writes go through `internal/ledger.Append` (lock + monotonic n) or through the same atomic `jsonio.WriteJSON` already used by `cmd/init`. `--plan` is read-only; `--apply` is the only write path.

**Tech Stack:** Same as Plan 1 — Go 1.22+, stdlib only.

**Spec reference:** `docs/superpowers/specs/2026-05-14-ledger-kit-go-design.md` §11

**Process gate:** Plan 2 follows the ledger delivery lifecycle: `open → in_progress → audit_ready → done`. Implementation rows must not mark the ticket `done`; the final `done` row is an audit decision (`role=audit`, `audit_result=pass`) after the acceptance criteria below are verified.

---

## Hard Acceptance Criteria

These must be true after all tasks complete. Any task that breaks one is a failure.

1. `ledger-kit import legacy --target X --plan` makes **zero** filesystem changes under `X`. (Verified by snapshotting the directory tree before and after.)
2. `ledger-kit import legacy --target X --apply` only writes inside `<X>/ledger/` (data files + `import-errors.jsonl` + `.backup/`) and optionally `<X>/ledger/legacy/` when `--archive-originals`. It never modifies legacy source files at the repo root unless `--archive-originals` is set (in which case it **moves**, not overwrites).
3. Existing in-place rows in `<X>/ledger/tickets.jsonl` / `worklog.jsonl` are not mutated. `--apply` either appends new rows or, when the target is being rebuilt, backs up the previous file to `<X>/ledger/.backup/YYYYMMDD-HHMMSS/` then writes the new file.
4. Ghost rows (`task=""` or `ticket=""` on a worklog with `ticket` present, etc.) are preserved either in `<X>/ledger/import-errors.jsonl` (parse failures and rows that cannot be normalized) or as ledger rows neutralized by an accompanying `invalidates_n` row (so `ledger-kit verify` only warns about them, never fails).
5. After `--apply`, `ledger-kit verify --target X` exits 0. Any remaining issues must be warnings, not fails.
6. Running `--apply` twice in a row produces "no changes" on the second run (idempotency). The second run does not create a new backup.
7. Legacy rows using the audit lifecycle statuses (`audit_ready`, `changes_requested`) are preserved as valid statuses, not rewritten or downgraded.
8. Completion requires an audit ticket event: `status=done`, `role=audit`, `audit_result=pass`, with `evidence` naming the verification commands or reports.

---

## File Structure

```
ledger-kit/
  internal/
    legacy/
      types.go        types_test.go        # Source, Plan, Change, ImportError, etc.
      scan.go         scan_test.go         # detect legacy source files, read raw rows
      normalize.go    normalize_test.go    # envelope inference (parent/n/ts/branch/agent)
      plan.go         plan_test.go         # compose Plan from sources + current target
      apply.go        apply_test.go        # write target + backup + errors + gitignore
  cmd/
    import.go         import_test.go       # CLI wrapper (`import legacy`)
  e2e/
    import_legacy_test.go                  # end-to-end against a fixture repo
```

Module path remains `github.com/hgwk/ledger-kit`. Working dir: `./`.

---

## Decisions Locked

These are codified from spec §11 and user direction so subagents don't re-litigate them:

- **Parent inference**: if a ticket's `parent_ticket` is missing or empty, check the ticket id's `<PREFIX>-...` prefix against `config.Parents`. Match → that parent. No match → `LEGACY`. Never `ROOT` by inference.
- **n assignment**: legacy rows are renumbered consecutively from 1 in row-order encountered. Original n (if present and consistent) is preserved when already consecutive; otherwise overwritten.
- **ts**: preserve when ISO8601 UTC and non-decreasing; otherwise replace with the import wall-clock time and emit warning.
- **agent**: priority chain from spec §5.1 with the legacy exception — if nothing resolves, use `"legacy"` literal and emit warning. (Spec §11.4.3 + §5 final paragraph.)
- **category**: NOT backfilled. Legacy rows may lack `category`; verify treats this as warning by default and that's acceptable.
- **Audit lifecycle**: `audit_ready` and `changes_requested` are first-class ticket statuses. Import preserves them exactly. `done` remains a completion state, but Plan 2 itself may only append `done` through an audit row after the full smoke passes.
- **branch**: kept as-is. When missing, leave empty.
- **Ghost rows**:
  - Rows that fail JSON parsing → appended to `ledger/import-errors.jsonl` with the original line text and a parse-error annotation.
  - Rows that parse but lack at least one of `TicketNonEmpty`/`WorklogNonEmpty` semantic fields → written into the target as-is **and** followed by an auto-generated companion row with `invalidates_n: <ghost-row-n>` so verify warns instead of fails.
- **Sources scanned (in order, all optional)**:
  1. `<target>/agent-tickets.jsonl` (rows → target tickets)
  2. `<target>/agent-worklog.jsonl` (rows → target worklog)
  3. `<target>/goal.json` (rows → target goal — replaces if exists, atomic write)
  4. Pre-existing `<target>/ledger/tickets.jsonl` and `<target>/ledger/worklog.jsonl` (already in the new location — used for idempotency comparison; not re-imported)
- **Backup policy**: under `<target>/ledger/.backup/YYYYMMDD-HHMMSS/`. Only files that would actually change are backed up.
- **Archive policy**: `--archive-originals` moves the 3 root-level sources into `<target>/ledger/legacy/`. Default is leave-in-place. Move uses `os.Rename`; if it fails (different filesystems), fall back to copy+remove.
- **Force**: if the would-be-written target has FEWER rows than what's already in `<target>/ledger/`, `--apply` requires `--force`. `--plan` warns only.

---

## Task Granularity

8 tasks. Each is one TDD cycle (failing test → impl → pass → commit). Tasks are intentionally small.

Every task ends with an implementation row no stronger than `audit_ready`. If review finds issues, append `changes_requested` and continue with a new `in_progress` row. The final worklog is appended only after the audit pass row.

---

### Task 1: `internal/legacy/types.go` — data structures

**Files:**
- Create: `internal/legacy/types.go`

- [ ] **Step 1: Write `internal/legacy/types.go`**

```go
// Package legacy implements the data migration from earlier ledger layouts
// to the spec §3 standard. Spec §11.
package legacy

import "github.com/hgwk/ledger-kit/internal/ledger"

// Source enumerates the files we scan in the target directory.
type Source struct {
	Path      string       // absolute path on disk
	Kind      SourceKind   // tickets, worklog, goal, etc.
	Rows      []ledger.Row // parsed rows (empty for Goal)
	Goal      *ledger.Goal // populated only for SourceGoal
	ParseErrs []ParseError // rows that failed JSON parsing, keyed by 1-based line
	Exists    bool
}

type SourceKind int

const (
	SourceUnknown SourceKind = iota
	SourceLegacyTickets       // <target>/agent-tickets.jsonl
	SourceLegacyWorklog       // <target>/agent-worklog.jsonl
	SourceLegacyGoal          // <target>/goal.json
	SourceCurrentTickets      // <target>/ledger/tickets.jsonl
	SourceCurrentWorklog      // <target>/ledger/worklog.jsonl
	SourceCurrentGoal         // <target>/ledger/goal.json
)

type ParseError struct {
	Line int    // 1-based
	Raw  string // raw line text
	Err  string // parse error message
}

// Plan describes what `--apply` would do. Build once from sources, then
// either render (--plan) or execute (--apply).
type Plan struct {
	TargetDir   string
	Sources     []Source     // exists==true subset only
	Changes     []Change     // per output file
	Warnings    []string     // human-readable, surfaced in plan + apply
	ParseErrors []ParseError // aggregated across sources; routed to import-errors.jsonl
	Counts      Counts
}

// Change represents a write the apply phase will perform.
type Change struct {
	OutputPath string // relative to TargetDir
	Action     ChangeAction
	NewBytes   []byte // for create/replace — full file contents
}

type ChangeAction int

const (
	ActionNoop ChangeAction = iota
	ActionCreate
	ActionReplace
)

// Counts feeds the plan report.
type Counts struct {
	TicketsImported  int
	WorklogImported  int
	GoalCreated      bool
	ParentInferred   int
	BranchInferred   int
	NReassigned      int
	TSReplaced       int
	AgentDefaulted   int
	GhostTickets     int // rows with empty semantic ticket fields
	GhostWorklog     int // rows with empty semantic worklog fields
	ParseErrors      int
	OrphanWorklog    int
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```
git add internal/legacy/types.go
git commit -m "feat(legacy): data structures for import"
```

---

### Task 2: `internal/legacy/scan.go` — detect sources and parse rows

**Files:**
- Create: `internal/legacy/scan.go`
- Create: `internal/legacy/scan_test.go`

`Scan(targetDir)` returns the six possible Source values (one per SourceKind). Each Source's `Exists` reflects whether the file is present, and parsed rows + parse errors are filled in.

- [ ] **Step 1: Write `internal/legacy/scan_test.go`**

```go
package legacy

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestScan_DetectsAllKinds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"x","task":"t","status":"open","ts":"2026-05-14T10:00:00Z","parent_ticket":"ROOT","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	writeFile(t, dir, "agent-worklog.jsonl", `{"n":1,"ticket":"x","task":"t","scope":"repo","result":"r","ts":"2026-05-14T10:00:00Z","agent":"codex","paths":[],"commands":[],"notes":"","branch":"","commit":""}`+"\n")
	writeFile(t, dir, "goal.json", `{"schema_version":1,"summary":"hi"}`)

	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	exists := map[SourceKind]bool{}
	for _, s := range srcs {
		if s.Exists {
			exists[s.Kind] = true
		}
	}
	for _, k := range []SourceKind{SourceLegacyTickets, SourceLegacyWorklog, SourceLegacyGoal} {
		if !exists[k] {
			t.Fatalf("expected kind %v to be detected", k)
		}
	}
}

func TestScan_MissingFilesAreFine(t *testing.T) {
	dir := t.TempDir()
	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, s := range srcs {
		if s.Exists {
			t.Fatalf("nothing should exist in empty dir, got %v", s)
		}
	}
}

func TestScan_PreservesParseErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"x","task":"t"}
not json
{"n":3,"ticket":"y","task":"t"}
`)
	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var src Source
	for _, s := range srcs {
		if s.Kind == SourceLegacyTickets {
			src = s
		}
	}
	if !src.Exists {
		t.Fatalf("tickets source should exist")
	}
	if len(src.Rows) != 2 {
		t.Fatalf("expected 2 good rows, got %d", len(src.Rows))
	}
	if len(src.ParseErrs) != 1 || src.ParseErrs[0].Line != 2 {
		t.Fatalf("expected one parse error on line 2, got %+v", src.ParseErrs)
	}
}

func TestScan_DetectsCurrentLedger(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ledger/tickets.jsonl", `{"n":1,"ticket":"x"}`+"\n")
	writeFile(t, dir, "ledger/goal.json", `{"schema_version":1}`)
	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	got := map[SourceKind]bool{}
	for _, s := range srcs {
		if s.Exists {
			got[s.Kind] = true
		}
	}
	if !got[SourceCurrentTickets] || !got[SourceCurrentGoal] {
		t.Fatalf("expected current ledger files detected, got %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/legacy/...`
Expected: FAIL with `undefined: Scan`.

- [ ] **Step 3: Implement `internal/legacy/scan.go`**

```go
package legacy

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/hgwk/ledger-kit/internal/jsonio"
	"github.com/hgwk/ledger-kit/internal/ledger"
)

// Scan returns the six well-known sources for targetDir. Sources that do
// not exist are returned with Exists=false so callers can render a stable
// plan report regardless of which subset is present.
func Scan(targetDir string) ([]Source, error) {
	specs := []struct {
		kind SourceKind
		rel  string
		goal bool
	}{
		{SourceLegacyTickets, "agent-tickets.jsonl", false},
		{SourceLegacyWorklog, "agent-worklog.jsonl", false},
		{SourceLegacyGoal, "goal.json", true},
		{SourceCurrentTickets, "ledger/tickets.jsonl", false},
		{SourceCurrentWorklog, "ledger/worklog.jsonl", false},
		{SourceCurrentGoal, "ledger/goal.json", true},
	}
	out := make([]Source, 0, len(specs))
	for _, sp := range specs {
		full := filepath.Join(targetDir, sp.rel)
		src := Source{Path: full, Kind: sp.kind}
		if _, err := os.Stat(full); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				out = append(out, src)
				continue
			}
			return nil, err
		}
		src.Exists = true
		if sp.goal {
			var g ledger.Goal
			if err := jsonio.ReadJSON(full, &g); err != nil {
				src.ParseErrs = append(src.ParseErrs, ParseError{Line: 0, Raw: full, Err: err.Error()})
			} else {
				src.Goal = &g
			}
		} else {
			rows, errs, err := readJSONL(full)
			if err != nil {
				return nil, err
			}
			src.Rows = rows
			src.ParseErrs = errs
		}
		out = append(out, src)
	}
	return out, nil
}

func readJSONL(path string) ([]ledger.Row, []ParseError, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var rows []ledger.Row
	var errs []ParseError
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var r ledger.Row
		if err := json.Unmarshal(raw, &r); err != nil {
			errs = append(errs, ParseError{Line: line, Raw: string(raw), Err: err.Error()})
			continue
		}
		rows = append(rows, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return rows, errs, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/legacy/... -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```
git add internal/legacy
git commit -m "feat(legacy): scan target for legacy and current sources"
```

---

### Task 3: `internal/legacy/normalize.go` — envelope inference

**Files:**
- Create: `internal/legacy/normalize.go`
- Create: `internal/legacy/normalize_test.go`

This is the heart of the migration. Given raw rows from a legacy file, produce normalized rows + a list of inferences performed (for reporting) + ghost-row detections (for invalidates_n companion rows downstream).

- [ ] **Step 1: Write `internal/legacy/normalize_test.go`**

```go
package legacy

import (
	"testing"

	"github.com/hgwk/ledger-kit/internal/ledger"
)

func TestNormalizeTickets_InfersParentFromPrefix(t *testing.T) {
	parents := []string{"ROOT", "BUG", "FE", "LEGACY"}
	in := []ledger.Row{
		{"n": float64(1), "ticket": "BUG-101", "task": "fix", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}},
	}
	rows, n, _ := NormalizeTickets(in, parents, fixedNow())
	if n.ParentInferred != 1 {
		t.Fatalf("expected 1 parent inference, got %d", n.ParentInferred)
	}
	if rows[0]["parent_ticket"] != "BUG" {
		t.Fatalf("expected BUG, got %v", rows[0]["parent_ticket"])
	}
}

func TestNormalizeTickets_UnknownPrefixGetsLegacy(t *testing.T) {
	parents := []string{"ROOT", "BUG"}
	in := []ledger.Row{
		{"n": float64(1), "ticket": "XYZ-1", "task": "t", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}},
	}
	rows, _, _ := NormalizeTickets(in, parents, fixedNow())
	if rows[0]["parent_ticket"] != "LEGACY" {
		t.Fatalf("expected LEGACY, got %v", rows[0]["parent_ticket"])
	}
}

func TestNormalizeTickets_NeverInfersRoot(t *testing.T) {
	parents := []string{"ROOT", "BUG"}
	in := []ledger.Row{
		{"n": float64(1), "ticket": "foo-1", "task": "t", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}},
	}
	rows, _, _ := NormalizeTickets(in, parents, fixedNow())
	if rows[0]["parent_ticket"] == "ROOT" {
		t.Fatalf("ROOT must not be inferred")
	}
}

func TestNormalizeTickets_ConsecutiveN(t *testing.T) {
	in := []ledger.Row{
		{"ticket": "a", "task": "t", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
		{"n": float64(5), "ticket": "b", "task": "t", "ts": "2026-05-14T10:01:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
	}
	rows, n, _ := NormalizeTickets(in, []string{"ROOT"}, fixedNow())
	if rows[0]["n"].(int) != 1 || rows[1]["n"].(int) != 2 {
		t.Fatalf("expected n=1,2, got %v %v", rows[0]["n"], rows[1]["n"])
	}
	if n.NReassigned == 0 {
		t.Fatalf("expected reassignment count > 0")
	}
}

func TestNormalizeTickets_MissingTSGetsNowAndWarn(t *testing.T) {
	in := []ledger.Row{
		{"ticket": "a", "task": "t", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
	}
	rows, n, warns := NormalizeTickets(in, []string{"ROOT"}, fixedNow())
	if rows[0]["ts"] == "" {
		t.Fatalf("ts should be filled, got empty")
	}
	if n.TSReplaced == 0 || len(warns) == 0 {
		t.Fatalf("expected ts replacement count and warning, got %+v warns=%v", n, warns)
	}
}

func TestNormalizeTickets_DefaultsAgentToLegacy(t *testing.T) {
	in := []ledger.Row{
		{"ticket": "a", "task": "t", "ts": "2026-05-14T10:00:00Z", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
	}
	rows, n, _ := NormalizeTickets(in, []string{"ROOT"}, fixedNow())
	if rows[0]["agent"] != "legacy" {
		t.Fatalf("expected agent=legacy, got %v", rows[0]["agent"])
	}
	if n.AgentDefaulted == 0 {
		t.Fatalf("expected agent default count")
	}
}

func TestNormalizeTickets_DetectsGhostRow(t *testing.T) {
	in := []ledger.Row{
		{"ticket": "", "task": "", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "done", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
	}
	rows, n, _ := NormalizeTickets(in, []string{"ROOT"}, fixedNow())
	if n.GhostTickets != 1 {
		t.Fatalf("expected 1 ghost ticket, got %d", n.GhostTickets)
	}
	if rows[0]["ticket"] != "" {
		t.Fatalf("ghost row content must be preserved, got %v", rows[0]["ticket"])
	}
}

func TestNormalizeWorklog_TicketOptional(t *testing.T) {
	in := []ledger.Row{
		{"task": "goal change", "scope": "ledger", "result": "ok", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "paths": []any{}, "commands": []any{}, "notes": "", "branch": "", "commit": ""},
	}
	rows, n, _ := NormalizeWorklog(in, fixedNow())
	if _, present := rows[0]["ticket"]; present {
		t.Fatalf("worklog without ticket should not get a ticket field")
	}
	if n.GhostWorklog != 0 {
		t.Fatalf("optional ticket is not ghost")
	}
}

func fixedNow() string { return "2026-05-14T12:00:00Z" }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/legacy/...`
Expected: FAIL with `undefined: NormalizeTickets`.

- [ ] **Step 3: Implement `internal/legacy/normalize.go`**

```go
package legacy

import (
	"regexp"
	"strings"

	"github.com/hgwk/ledger-kit/internal/ledger"
)

var isoRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)

// NormalizeTickets walks the input rows, returns normalized rows + counts +
// human-readable warnings. parents is config.Parents (used for prefix inference).
func NormalizeTickets(in []ledger.Row, parents []string, now string) ([]ledger.Row, Counts, []string) {
	allowed := setOf(parents)
	out := make([]ledger.Row, 0, len(in))
	var counts Counts
	var warns []string
	prevTS := ""
	for i, raw := range in {
		r := copyRow(raw)
		// n consecutive from 1
		want := i + 1
		got, _ := r["n"].(float64)
		if int(got) != want {
			counts.NReassigned++
		}
		r["n"] = want
		// ts ISO + non-decreasing
		ts, _ := r["ts"].(string)
		if !isoRe.MatchString(ts) || (prevTS != "" && ts < prevTS) {
			r["ts"] = now
			counts.TSReplaced++
			warns = append(warns, "ts replaced on legacy ticket row")
		}
		prevTS, _ = r["ts"].(string)
		// agent default
		if a, _ := r["agent"].(string); a == "" {
			r["agent"] = "legacy"
			counts.AgentDefaulted++
		}
		// parent inference
		if p, _ := r["parent_ticket"].(string); p == "" {
			id, _ := r["ticket"].(string)
			inferred := inferParent(id, allowed)
			r["parent_ticket"] = inferred
			counts.ParentInferred++
		}
		// branch missing → "" (already so, no-op; keep for clarity)
		if _, ok := r["branch"]; !ok {
			r["branch"] = ""
		}
		// ghost detection (post-normalization)
		if isGhostTicket(r) {
			counts.GhostTickets++
		}
		out = append(out, r)
	}
	return out, counts, warns
}

// NormalizeWorklog mirrors NormalizeTickets but for worklog rows. ticket is optional.
func NormalizeWorklog(in []ledger.Row, now string) ([]ledger.Row, Counts, []string) {
	out := make([]ledger.Row, 0, len(in))
	var counts Counts
	var warns []string
	prevTS := ""
	for i, raw := range in {
		r := copyRow(raw)
		want := i + 1
		got, _ := r["n"].(float64)
		if int(got) != want {
			counts.NReassigned++
		}
		r["n"] = want
		ts, _ := r["ts"].(string)
		if !isoRe.MatchString(ts) || (prevTS != "" && ts < prevTS) {
			r["ts"] = now
			counts.TSReplaced++
			warns = append(warns, "ts replaced on legacy worklog row")
		}
		prevTS, _ = r["ts"].(string)
		if a, _ := r["agent"].(string); a == "" {
			r["agent"] = "legacy"
			counts.AgentDefaulted++
		}
		// Drop empty-string ticket field entirely (ticket is optional in worklog).
		if t, ok := r["ticket"].(string); ok && t == "" {
			delete(r, "ticket")
		}
		// Ensure required envelope present (empty strings allowed for branch/commit/notes).
		for _, f := range []string{"branch", "commit", "notes"} {
			if _, ok := r[f]; !ok {
				r[f] = ""
			}
		}
		if isGhostWorklog(r) {
			counts.GhostWorklog++
		}
		out = append(out, r)
	}
	return out, counts, warns
}

func inferParent(ticketID string, allowed map[string]struct{}) string {
	if i := strings.IndexByte(ticketID, '-'); i > 0 {
		prefix := ticketID[:i]
		if _, ok := allowed[prefix]; ok {
			return prefix
		}
	}
	return "LEGACY"
}

func isGhostTicket(r ledger.Row) bool {
	for _, f := range ledger.TicketNonEmpty {
		v, _ := r[f].(string)
		if v == "" {
			return true
		}
	}
	return false
}

func isGhostWorklog(r ledger.Row) bool {
	for _, f := range ledger.WorklogNonEmpty {
		v, _ := r[f].(string)
		if v == "" {
			return true
		}
	}
	return false
}

func setOf(in []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, v := range in {
		out[v] = struct{}{}
	}
	return out
}

func copyRow(in ledger.Row) ledger.Row {
	out := make(ledger.Row, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/legacy/... -v`
Expected: PASS (all 9 normalize tests + 4 scan tests).

- [ ] **Step 5: Commit**

```
git add internal/legacy/normalize.go internal/legacy/normalize_test.go
git commit -m "feat(legacy): envelope normalization with prefix→parent inference"
```

---

### Task 4: `internal/legacy/plan.go` — compose Plan from Sources

**Files:**
- Create: `internal/legacy/plan.go`
- Create: `internal/legacy/plan_test.go`

`Compose(targetDir, sources, cfg) Plan` builds the in-memory Plan. For each output (tickets.jsonl, worklog.jsonl, goal.json, optionally import-errors.jsonl), compute the new bytes and compare with what's currently at the target. Same → ActionNoop; different → ActionReplace (or ActionCreate if absent).

Ghost-row companion invalidates_n rows are inserted by Compose: when a ghost ticket is detected at row k, a follow-up row `{n: k+1, ts: now, ticket: "legacy-invalid-<k>", parent_ticket: "LEGACY", agent: "legacy", role: "ops", status: "cancelled", task: "invalidate ghost row " + k, scope: "ledger", paths: [], blocked_by: [], branch: "", invalidates_n: k}` is appended directly after the ghost row, and `n` of all subsequent rows is bumped by 1.

- [ ] **Step 1: Write `internal/legacy/plan_test.go`**

```go
package legacy

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ledger-kit/internal/config"
)

func TestCompose_CreatesNewLedger(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"BUG-1","task":"fix","status":"open","ts":"2026-05-14T10:00:00Z","parent_ticket":"","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	srcs, _ := Scan(dir)
	cfg := config.Default("x", "id", "")
	plan := Compose(dir, srcs, cfg, fixedNow())
	if len(plan.Changes) == 0 {
		t.Fatalf("expected at least one Change, got 0")
	}
	var hasTickets bool
	for _, c := range plan.Changes {
		if filepath.Base(c.OutputPath) == "tickets.jsonl" && c.Action == ActionCreate {
			hasTickets = true
		}
	}
	if !hasTickets {
		t.Fatalf("expected tickets.jsonl create, got %+v", plan.Changes)
	}
}

func TestCompose_NoChangeWhenAlreadyConverted(t *testing.T) {
	dir := t.TempDir()
	row := `{"n":1,"ts":"2026-05-14T10:00:00Z","parent_ticket":"BUG","ticket":"BUG-1","agent":"codex","role":"impl","status":"open","task":"fix","scope":"repo","paths":[],"blocked_by":[],"branch":""}`
	// Plant the same row in both legacy and new locations.
	writeFile(t, dir, "agent-tickets.jsonl", row+"\n")
	writeFile(t, dir, "ledger/tickets.jsonl", row+"\n")
	srcs, _ := Scan(dir)
	plan := Compose(dir, srcs, config.Default("x", "id", ""), fixedNow())
	for _, c := range plan.Changes {
		if filepath.Base(c.OutputPath) == "tickets.jsonl" && c.Action != ActionNoop {
			t.Fatalf("expected noop for already-converted tickets, got %v", c.Action)
		}
	}
}

func TestCompose_InsertsInvalidatesNForGhostRow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl",
		`{"n":1,"ticket":"","task":"","status":"done","ts":"2026-05-14T10:00:00Z","parent_ticket":"ROOT","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"+
			`{"n":2,"ticket":"BUG-1","task":"real","status":"open","ts":"2026-05-14T10:01:00Z","parent_ticket":"BUG","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	srcs, _ := Scan(dir)
	plan := Compose(dir, srcs, config.Default("x", "id", ""), fixedNow())
	var ticketsBytes []byte
	for _, c := range plan.Changes {
		if filepath.Base(c.OutputPath) == "tickets.jsonl" {
			ticketsBytes = c.NewBytes
		}
	}
	if !strings.Contains(string(ticketsBytes), `"invalidates_n":1`) {
		t.Fatalf("expected invalidates_n=1 companion row, got:\n%s", ticketsBytes)
	}
}

func TestCompose_ParseErrorsRouteToImportErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", "not json\n")
	srcs, _ := Scan(dir)
	plan := Compose(dir, srcs, config.Default("x", "id", ""), fixedNow())
	var hasErrors bool
	for _, c := range plan.Changes {
		if filepath.Base(c.OutputPath) == "import-errors.jsonl" && c.Action == ActionCreate {
			hasErrors = true
		}
	}
	if !hasErrors {
		t.Fatalf("expected import-errors.jsonl create, got %+v", plan.Changes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/legacy/...`
Expected: FAIL with `undefined: Compose`.

- [ ] **Step 3: Implement `internal/legacy/plan.go`**

```go
package legacy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hgwk/ledger-kit/internal/config"
	"github.com/hgwk/ledger-kit/internal/ledger"
)

// Compose builds an executable Plan from sources discovered by Scan.
// now is injected to keep tests deterministic.
func Compose(targetDir string, sources []Source, cfg config.Config, now string) Plan {
	plan := Plan{TargetDir: targetDir}

	var ticketRows, worklogRows []ledger.Row
	var goal *ledger.Goal

	for _, s := range sources {
		if !s.Exists {
			continue
		}
		plan.Sources = append(plan.Sources, s)
		switch s.Kind {
		case SourceLegacyTickets:
			rows, c, w := NormalizeTickets(s.Rows, cfg.Parents, now)
			rows, c = insertInvalidates(rows, c, now, "ticket")
			ticketRows = rows
			mergeCounts(&plan.Counts, c)
			plan.Counts.TicketsImported = len(ticketRows)
			plan.Warnings = append(plan.Warnings, w...)
			plan.ParseErrors = append(plan.ParseErrors, s.ParseErrs...)
		case SourceLegacyWorklog:
			rows, c, w := NormalizeWorklog(s.Rows, now)
			rows, c = insertInvalidates(rows, c, now, "worklog")
			worklogRows = rows
			mergeCounts(&plan.Counts, c)
			plan.Counts.WorklogImported = len(worklogRows)
			plan.Warnings = append(plan.Warnings, w...)
			plan.ParseErrors = append(plan.ParseErrors, s.ParseErrs...)
		case SourceLegacyGoal:
			if s.Goal != nil {
				goal = s.Goal
				plan.Counts.GoalCreated = true
			}
		}
	}

	// Add Changes for each output file.
	if ticketRows != nil {
		plan.Changes = append(plan.Changes, diffJSONL(targetDir, "ledger/tickets.jsonl", ticketRows))
	}
	if worklogRows != nil {
		plan.Changes = append(plan.Changes, diffJSONL(targetDir, "ledger/worklog.jsonl", worklogRows))
	}
	if goal != nil {
		plan.Changes = append(plan.Changes, diffJSON(targetDir, "ledger/goal.json", goal))
	}
	if len(plan.ParseErrors) > 0 {
		plan.Counts.ParseErrors = len(plan.ParseErrors)
		plan.Changes = append(plan.Changes, parseErrorsChange(targetDir, plan.ParseErrors))
	}
	return plan
}

// insertInvalidates inserts a companion `invalidates_n` row after each
// ghost row. The kind parameter selects the row shape ("ticket" or "worklog").
func insertInvalidates(rows []ledger.Row, counts Counts, now string, kind string) ([]ledger.Row, Counts) {
	out := make([]ledger.Row, 0, len(rows))
	insertions := 0
	for _, r := range rows {
		out = append(out, r)
		ghost := false
		if kind == "ticket" {
			ghost = isGhostTicket(r)
		} else {
			ghost = isGhostWorklog(r)
		}
		if ghost {
			n, _ := r["n"].(int)
			out = append(out, companionRow(kind, n, now))
			insertions++
		}
	}
	// Re-number all rows consecutively after insertions. Each companion row
	// sits directly after its ghost, so the ghost's new n equals the
	// companion's 0-based index `i` (because the ghost is at index i-1, so its
	// new n = (i-1)+1 = i).
	for i := range out {
		out[i]["n"] = i + 1
		if _, ok := out[i]["invalidates_n"]; ok {
			out[i]["invalidates_n"] = i // ghost's new n
		}
	}
	return out, counts
}

func companionRow(kind string, ghostN int, now string) ledger.Row {
	if kind == "ticket" {
		return ledger.Row{
			"ts":            now,
			"parent_ticket": "LEGACY",
			"ticket":        fmt.Sprintf("legacy-invalid-%d", ghostN),
			"agent":         "legacy",
			"role":          "ops",
			"status":        "cancelled",
			"task":          fmt.Sprintf("invalidate ghost row %d", ghostN),
			"scope":         "ledger",
			"paths":         []any{},
			"blocked_by":    []any{},
			"branch":        "",
			"invalidates_n": ghostN,
		}
	}
	return ledger.Row{
		"ts":            now,
		"agent":         "legacy",
		"task":          fmt.Sprintf("invalidate ghost worklog row %d", ghostN),
		"scope":         "ledger",
		"result":        fmt.Sprintf("invalidate ghost worklog row %d", ghostN),
		"paths":         []any{},
		"commands":      []any{},
		"notes":         "",
		"branch":        "",
		"commit":        "",
		"invalidates_n": ghostN,
	}
}

func diffJSONL(targetDir, rel string, rows []ledger.Row) Change {
	var buf bytes.Buffer
	for _, r := range rows {
		b, _ := json.Marshal(r)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	full := filepath.Join(targetDir, rel)
	return diffBytes(full, rel, buf.Bytes())
}

func diffJSON(targetDir, rel string, v any) Change {
	b, _ := json.MarshalIndent(v, "", "  ")
	b = append(b, '\n')
	full := filepath.Join(targetDir, rel)
	return diffBytes(full, rel, b)
}

func parseErrorsChange(targetDir string, errs []ParseError) Change {
	var buf bytes.Buffer
	for _, e := range errs {
		row := map[string]any{"line": e.Line, "raw": e.Raw, "error": e.Err}
		b, _ := json.Marshal(row)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	rel := "ledger/import-errors.jsonl"
	full := filepath.Join(targetDir, rel)
	return diffBytes(full, rel, buf.Bytes())
}

func diffBytes(full, rel string, want []byte) Change {
	cur, err := os.ReadFile(full)
	if err != nil && !os.IsNotExist(err) {
		// Treat read error as needing replace so apply re-runs cleanly.
		return Change{OutputPath: rel, Action: ActionReplace, NewBytes: want}
	}
	if err != nil { // not exist
		return Change{OutputPath: rel, Action: ActionCreate, NewBytes: want}
	}
	if bytes.Equal(cur, want) {
		return Change{OutputPath: rel, Action: ActionNoop, NewBytes: want}
	}
	return Change{OutputPath: rel, Action: ActionReplace, NewBytes: want}
}

func mergeCounts(dst *Counts, src Counts) {
	dst.NReassigned += src.NReassigned
	dst.TSReplaced += src.TSReplaced
	dst.AgentDefaulted += src.AgentDefaulted
	dst.ParentInferred += src.ParentInferred
	dst.BranchInferred += src.BranchInferred
	dst.GhostTickets += src.GhostTickets
	dst.GhostWorklog += src.GhostWorklog
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/legacy/... -v`
Expected: PASS (4 new plan tests + existing).

- [ ] **Step 5: Commit**

```
git add internal/legacy/plan.go internal/legacy/plan_test.go
git commit -m "feat(legacy): compose plan with invalidates_n for ghost rows"
```

---

### Task 5: `internal/legacy/apply.go` — execute plan with backup + lock

**Files:**
- Create: `internal/legacy/apply.go`
- Create: `internal/legacy/apply_test.go`

`Apply(plan, opts) error` does:
1. Acquire `<target>/ledger/.lock`
2. For each non-noop Change: back up the existing file (if any) into `<target>/ledger/.backup/<ts>/`, then write the new bytes via atomic temp+rename.
3. Update `.gitignore` (same as `cmd/init.ensureGitignore` — reuse if practical, otherwise replicate the small block).
4. If `opts.ArchiveOriginals`: move `<target>/agent-*.jsonl` and `<target>/goal.json` into `<target>/ledger/legacy/`.

- [ ] **Step 1: Write `internal/legacy/apply_test.go`**

```go
package legacy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hgwk/ledger-kit/internal/config"
)

func runPlan(t *testing.T, dir string) Plan {
	t.Helper()
	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return Compose(dir, srcs, config.Default("x", "id", ""), fixedNow())
}

func TestApply_WritesTickets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"BUG-1","task":"t","status":"open","ts":"2026-05-14T10:00:00Z","parent_ticket":"BUG","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	plan := runPlan(t, dir)
	if err := Apply(plan, ApplyOpts{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil || len(data) == 0 {
		t.Fatalf("tickets not written: err=%v len=%d", err, len(data))
	}
}

func TestApply_Idempotent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"BUG-1","task":"t","status":"open","ts":"2026-05-14T10:00:00Z","parent_ticket":"BUG","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	if err := Apply(runPlan(t, dir), ApplyOpts{}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	// Capture state then re-run; nothing should change.
	first, _ := os.ReadFile(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err := Apply(runPlan(t, dir), ApplyOpts{}); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	second, _ := os.ReadFile(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if string(first) != string(second) {
		t.Fatalf("second apply changed tickets file:\nfirst=%s\nsecond=%s", first, second)
	}
	// And there should be exactly one backup directory (from the first run only,
	// if any).
	entries, _ := os.ReadDir(filepath.Join(dir, "ledger", ".backup"))
	if len(entries) > 1 {
		t.Fatalf("second run should not create a new backup; got %d backup dirs", len(entries))
	}
}

func TestApply_BacksUpExistingFile(t *testing.T) {
	dir := t.TempDir()
	// First, install a previous-version target file.
	writeFile(t, dir, "ledger/tickets.jsonl", `{"n":1,"ts":"2026-05-14T09:00:00Z","ticket":"OLD-1","parent_ticket":"OLD","agent":"codex","role":"impl","status":"open","task":"old","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	// Then plant a different legacy source.
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"new","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	plan := runPlan(t, dir)
	if err := Apply(plan, ApplyOpts{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "ledger", ".backup"))
	if len(entries) != 1 {
		t.Fatalf("expected one backup dir, got %d", len(entries))
	}
}

func TestApply_ArchiveOriginalsMovesLegacySources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	writeFile(t, dir, "goal.json", `{"schema_version":1,"summary":"hi"}`)
	plan := runPlan(t, dir)
	if err := Apply(plan, ApplyOpts{ArchiveOriginals: true}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent-tickets.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("legacy source should be moved away after --archive-originals, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ledger", "legacy", "agent-tickets.jsonl")); err != nil {
		t.Fatalf("legacy source should be at ledger/legacy/: %v", err)
	}
}

func TestApply_NoChangesDoesNotBackup(t *testing.T) {
	dir := t.TempDir()
	plan := runPlan(t, dir) // no sources, no changes
	if err := Apply(plan, ApplyOpts{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ledger", ".backup")); !os.IsNotExist(err) {
		t.Fatalf(".backup should not exist when nothing was changed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/legacy/...`
Expected: FAIL with `undefined: Apply`.

- [ ] **Step 3: Implement `internal/legacy/apply.go`**

```go
package legacy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hgwk/ledger-kit/internal/locks"
)

type ApplyOpts struct {
	ArchiveOriginals bool
	// Force suppresses the safety check that rejects shrinking ledgers.
	Force bool
	// Clock allows tests to inject deterministic timestamps for the backup dir.
	Clock func() time.Time
}

func (o ApplyOpts) now() time.Time {
	if o.Clock != nil {
		return o.Clock()
	}
	return time.Now().UTC()
}

func Apply(plan Plan, opts ApplyOpts) error {
	if !plan.hasWork() {
		return nil
	}
	// Acquire the ledger lock for the target. Ensure ledger dir exists.
	ledgerDir := filepath.Join(plan.TargetDir, "ledger")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		return err
	}
	release, err := locks.Acquire(filepath.Join(ledgerDir, ".lock"), locks.Options{})
	if err != nil {
		return err
	}
	defer release()

	// Compute backup dir lazily so a noop run doesn't create it.
	stamp := opts.now().Format("20060102-150405")
	backupDir := filepath.Join(ledgerDir, ".backup", stamp)

	for _, c := range plan.Changes {
		if c.Action == ActionNoop {
			continue
		}
		full := filepath.Join(plan.TargetDir, c.OutputPath)
		// Backup existing file if any.
		if c.Action == ActionReplace {
			if err := backupFile(backupDir, plan.TargetDir, c.OutputPath); err != nil {
				return err
			}
		}
		if err := writeAtomic(full, c.NewBytes); err != nil {
			return err
		}
	}

	if err := ensureGitignore(plan.TargetDir); err != nil {
		return err
	}

	if opts.ArchiveOriginals {
		if err := archiveOriginals(plan.TargetDir); err != nil {
			return err
		}
	}
	return nil
}

func (p Plan) hasWork() bool {
	for _, c := range p.Changes {
		if c.Action != ActionNoop {
			return true
		}
	}
	return false
}

func backupFile(backupDir, root, rel string) error {
	src := filepath.Join(root, rel)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	dst := filepath.Join(backupDir, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func ensureGitignore(target string) error {
	required := []string{
		"ledger/.lock",
		"ledger/.backup/",
		"ledger/import-errors.jsonl",
		"ledger/legacy/",
	}
	path := filepath.Join(target, ".gitignore")
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	out := existing
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	for _, line := range required {
		present := false
		for _, l := range strings.Split(existing, "\n") {
			if strings.TrimSpace(l) == line {
				present = true
				break
			}
		}
		if !present {
			out += line + "\n"
		}
	}
	if out == existing {
		return nil
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

func archiveOriginals(target string) error {
	legacyDir := filepath.Join(target, "ledger", "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		return err
	}
	for _, rel := range []string{"agent-tickets.jsonl", "agent-worklog.jsonl", "goal.json"} {
		src := filepath.Join(target, rel)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		dst := filepath.Join(legacyDir, rel)
		if err := os.Rename(src, dst); err != nil {
			// Cross-filesystem fallback: copy then remove.
			if cpErr := copyFile(src, dst); cpErr != nil {
				return fmt.Errorf("archive %s: rename=%v copy=%v", rel, err, cpErr)
			}
			if rmErr := os.Remove(src); rmErr != nil {
				return rmErr
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
```


- [ ] **Step 4: Run tests**

Run: `go test ./internal/legacy/... -v -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/legacy/apply.go internal/legacy/apply_test.go
git commit -m "feat(legacy): atomic apply with backup, lock, and optional archive"
```

---

### Task 6: `cmd/import.go` — CLI wrapper

**Files:**
- Create: `cmd/import.go`
- Create: `cmd/import_test.go`

- [ ] **Step 1: Write `cmd/import_test.go`**

```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportLegacy_PlanWritesNothing(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "agent-tickets.jsonl"),
		[]byte(`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before := snapshot(t, target)

	out := &bytes.Buffer{}
	if code := RunImportCLI([]string{"legacy", "--target", target, "--plan"}, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("plan failed")
	}
	if !strings.Contains(out.String(), "Legacy import plan") {
		t.Fatalf("plan output should contain banner, got: %s", out.String())
	}

	after := snapshot(t, target)
	if before != after {
		t.Fatalf("--plan must not change the filesystem")
	}
}

func TestImportLegacy_ApplyCreatesLedger(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "agent-tickets.jsonl"),
		[]byte(`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if code := RunImportCLI([]string{"legacy", "--target", target, "--apply"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("apply failed")
	}
	if _, err := os.Stat(filepath.Join(target, "ledger", "tickets.jsonl")); err != nil {
		t.Fatalf("tickets file not produced: %v", err)
	}
}

func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		info, _ := d.Info()
		b.WriteString(rel)
		b.WriteString("|")
		if d.IsDir() {
			b.WriteString("DIR")
		} else {
			data, _ := os.ReadFile(p)
			b.WriteString(string(data))
			_ = info // size implied by content
		}
		b.WriteString("\n")
		return nil
	})
	return b.String()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL with `undefined: RunImportCLI`.

- [ ] **Step 3: Implement `cmd/import.go`**

```go
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hgwk/ledger-kit/internal/config"
	"github.com/hgwk/ledger-kit/internal/legacy"
)

func init() {
	Commands["import"] = RunImportCLI
}

func RunImportCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "legacy" {
		fmt.Fprintln(stderr, "usage: ledger-kit import legacy --target PATH (--plan | --apply [--archive-originals] [--force])")
		return 2
	}
	fs := newFlagSet("import legacy")
	target := fs.String("target", "", "")
	planFlag := fs.Bool("plan", false, "")
	applyFlag := fs.Bool("apply", false, "")
	archive := fs.Bool("archive-originals", false, "")
	force := fs.Bool("force", false, "")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *planFlag == *applyFlag {
		fmt.Fprintln(stderr, "specify exactly one of --plan or --apply")
		return 2
	}
	dir := resolveTarget(*target)

	cfg, err := loadOrDefaultConfig(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	srcs, err := legacy.Scan(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	plan := legacy.Compose(dir, srcs, cfg, time.Now().UTC().Format("2006-01-02T15:04:05Z"))

	if *planFlag {
		renderPlan(stdout, dir, plan, *force)
		return 0
	}

	if !*force && shrinkingTarget(plan) {
		fmt.Fprintln(stderr, "refusing to shrink an existing ledger; re-run with --force if intentional")
		return 1
	}

	if err := legacy.Apply(plan, legacy.ApplyOpts{ArchiveOriginals: *archive, Force: *force}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	renderApply(stdout, plan)
	return 0
}

func loadOrDefaultConfig(dir string) (config.Config, error) {
	cfg, err := config.Load(filepath.Join(dir, "ledger", "config.json"))
	if err != nil && !os.IsNotExist(err) {
		return cfg, err
	}
	if err != nil || cfg.ProjectID == "" {
		// Caller did not run init yet. Synthesize a temporary config so we can
		// infer parents. The real init must follow `import legacy --apply`.
		return config.Default(filepath.Base(dir), "import-stub", ""), nil
	}
	return cfg, nil
}

func shrinkingTarget(plan legacy.Plan) bool {
	for _, c := range plan.Changes {
		if c.Action != legacy.ActionReplace {
			continue
		}
		current, err := os.ReadFile(filepath.Join(plan.TargetDir, c.OutputPath))
		if err != nil {
			continue
		}
		if strings.Count(string(current), "\n") > strings.Count(string(c.NewBytes), "\n") {
			return true
		}
	}
	return false
}

func renderPlan(w io.Writer, dir string, plan legacy.Plan, force bool) {
	fmt.Fprintln(w, "Legacy import plan")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Sources:")
	for _, s := range plan.Sources {
		fmt.Fprintf(w, "  %s\t%d rows\n", filepath.Base(s.Path), len(s.Rows))
	}
	if len(plan.Sources) == 0 {
		fmt.Fprintln(w, "  (none detected)")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Target:")
	for _, c := range plan.Changes {
		fmt.Fprintf(w, "  %s\t%s\n", c.OutputPath, actionName(c.Action))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Changes:")
	fmt.Fprintf(w, "  ticket rows imported     %d\n", plan.Counts.TicketsImported)
	fmt.Fprintf(w, "  worklog rows imported    %d\n", plan.Counts.WorklogImported)
	fmt.Fprintf(w, "  parent_ticket inferred   %d\n", plan.Counts.ParentInferred)
	fmt.Fprintf(w, "  n reassigned             %d\n", plan.Counts.NReassigned)
	fmt.Fprintf(w, "  ts replaced              %d\n", plan.Counts.TSReplaced)
	fmt.Fprintf(w, "  agent defaulted          %d\n", plan.Counts.AgentDefaulted)
	fmt.Fprintf(w, "  ghost tickets            %d\n", plan.Counts.GhostTickets)
	fmt.Fprintf(w, "  ghost worklog            %d\n", plan.Counts.GhostWorklog)
	fmt.Fprintf(w, "  parse errors             %d\n", plan.Counts.ParseErrors)
	fmt.Fprintln(w)
	if len(plan.Warnings) > 0 {
		fmt.Fprintln(w, "Warnings:")
		for _, w0 := range plan.Warnings {
			fmt.Fprintf(w, "  %s\n", w0)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "Original files:")
	fmt.Fprintln(w, "  preserve in place (use --archive-originals to move them under ledger/legacy/)")
	if !force && shrinkingTarget(plan) {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "WARNING: existing target has more rows than the import would produce. --apply requires --force.")
	}
}

func renderApply(w io.Writer, plan legacy.Plan) {
	if !plan.HasChanges() {
		fmt.Fprintln(w, "no changes")
		return
	}
	for _, c := range plan.Changes {
		if c.Action == legacy.ActionNoop {
			continue
		}
		fmt.Fprintf(w, "%s %s\n", actionName(c.Action), c.OutputPath)
	}
}

func actionName(a legacy.ChangeAction) string {
	switch a {
	case legacy.ActionCreate:
		return "create"
	case legacy.ActionReplace:
		return "update"
	case legacy.ActionNoop:
		return "noop"
	}
	return "?"
}
```

In `internal/legacy/types.go`, add this helper used by `renderApply`:
```go
// HasChanges returns true if at least one Change is non-noop.
func (p Plan) HasChanges() bool {
	for _, c := range p.Changes {
		if c.Action != ActionNoop {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./... -count=1 -race`
Expected: PASS (all old + new).

- [ ] **Step 5: Commit**

```
git add cmd/import.go cmd/import_test.go internal/legacy/types.go
git commit -m "feat(import): legacy --plan/--apply wired into dispatcher"
```

---

### Task 7: end-to-end fixture test

**Files:**
- Create: `e2e/import_legacy_test.go`
- Create: `e2e/fixtures/legacy/agent-tickets.jsonl`
- Create: `e2e/fixtures/legacy/agent-worklog.jsonl`
- Create: `e2e/fixtures/legacy/goal.json`

This is the real-world acceptance check: copy a representative fixture into a temp dir, run the built binary with `--plan` (assert no diff), then `--apply` (assert tickets/worklog/goal exist and `verify` passes).

- [ ] **Step 1: Create fixture files**

`e2e/fixtures/legacy/agent-tickets.jsonl`:
```
{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-101","parent_ticket":"","agent":"codex","role":"impl","status":"open","task":"fix login","scope":"repo","paths":["src/login.go"],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:05:00Z","ticket":"","parent_ticket":"","agent":"","role":"impl","status":"done","task":"","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":3,"ts":"2026-05-14T10:10:00Z","ticket":"FE-2","parent_ticket":"FE","agent":"claude","role":"impl","status":"open","task":"render","scope":"repo","paths":[],"blocked_by":[],"branch":""}
```

(Note: row 2 is a deliberate ghost row — empty ticket and task — to exercise invalidates_n.)

`e2e/fixtures/legacy/agent-worklog.jsonl`:
```
{"n":1,"ts":"2026-05-14T10:11:00Z","ticket":"BUG-101","agent":"codex","task":"fix login","scope":"repo","result":"patched","paths":["src/login.go"],"commands":["go test"],"notes":"","branch":"","commit":""}
```

`e2e/fixtures/legacy/goal.json`:
```
{"schema_version":1,"track":"project","version":"0.1.0","updated":"2026-05-14T10:00:00Z","source_of_truth":"README.md","summary":"legacy goal","success_criteria":[]}
```

- [ ] **Step 2: Write `e2e/import_legacy_test.go`**

```go
package e2e

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportLegacy_EndToEnd(t *testing.T) {
	bin := buildBinary(t)
	work := t.TempDir()
	home := t.TempDir()

	// Copy fixture/legacy/* into work/.
	fixDir := filepath.Join(repoRoot(t), "e2e", "fixtures", "legacy")
	if err := copyTree(fixDir, work); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	envBase := append(os.Environ(), "LEDGER_KIT_HOME="+home, "LEDGER_AGENT=codex")
	runEnv := func(args ...string) (string, string, int) {
		c := exec.Command(bin, args...)
		c.Env = envBase
		var so, se bytes.Buffer
		c.Stdout = &so
		c.Stderr = &se
		err := c.Run()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return so.String(), se.String(), code
	}

	// Init first so config exists. (Acceptance #2: init writes ledger/config.json but no rows.)
	if _, se, code := runEnv("init", "--target", work, "--slug", "fixture"); code != 0 {
		t.Fatalf("init: %s", se)
	}

	beforePlanHash := treeHash(t, work)

	if so, se, code := runEnv("import", "legacy", "--target", work, "--plan"); code != 0 {
		t.Fatalf("plan: stdout=%s stderr=%s", so, se)
	} else if !strings.Contains(so, "Legacy import plan") {
		t.Fatalf("missing plan banner: %s", so)
	}

	afterPlanHash := treeHash(t, work)
	if beforePlanHash != afterPlanHash {
		t.Fatalf("--plan must not change the filesystem")
	}

	if _, se, code := runEnv("import", "legacy", "--target", work, "--apply"); code != 0 {
		t.Fatalf("apply: %s", se)
	}

	for _, want := range []string{"ledger/tickets.jsonl", "ledger/worklog.jsonl", "ledger/goal.json"} {
		if _, err := os.Stat(filepath.Join(work, want)); err != nil {
			t.Fatalf("missing %s: %v", want, err)
		}
	}

	if so, se, code := runEnv("verify", "--target", work); code != 0 {
		t.Fatalf("verify after import: stdout=%s stderr=%s", so, se)
	}

	// Re-running apply must produce "no changes".
	if so, _, code := runEnv("import", "legacy", "--target", work, "--apply"); code != 0 || !strings.Contains(so, "no changes") {
		t.Fatalf("second apply should be no-op: code=%d stdout=%s", code, so)
	}
}

func treeHash(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		fmt.Fprintf(h, "%s\n", rel)
		if !d.IsDir() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			io.Copy(h, f)
			f.Close()
		}
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
```

- [ ] **Step 3: Run e2e**

Run: `go test ./e2e/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```
git add e2e/
git commit -m "test(e2e): legacy import end-to-end fixture"
```

---

### Task 8: README + final smoke

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Append usage block to `README.md`** (after the existing intro):

```markdown

## Migrating from old layouts

If your repository has root-level `agent-tickets.jsonl`, `agent-worklog.jsonl`, or `goal.json`,
preview the migration first:

```bash
ledger-kit import legacy --target . --plan
```

Apply when you're satisfied:

```bash
ledger-kit import legacy --target . --apply
# optional: move legacy sources under ledger/legacy/
ledger-kit import legacy --target . --apply --archive-originals
```

`--apply` is idempotent. Running it twice produces "no changes".
```

- [ ] **Step 2: Final full smoke**

Run:
```
go test ./... -count=1 -race
go vet ./...
gofmt -l .
```
All three must be clean / green.

- [ ] **Step 3: Commit**

```
git add README.md
git commit -m "docs(readme): legacy import usage"
```

---

## Self-Review Checklist

After all tasks pass:

- [ ] `--plan` exit code 0, zero filesystem changes (verified by treeHash in e2e)
- [ ] `--apply` writes only inside `<target>/ledger/...` (and optionally `<target>/ledger/legacy/`)
- [ ] Existing in-place ticket/worklog rows are preserved verbatim when target was up-to-date
- [ ] Ghost rows produce companion invalidates_n rows; verify reports them as warnings, not fails
- [ ] `verify` exits 0 after `--apply` on the fixture
- [ ] `verify` accepts imported `audit_ready` / `changes_requested` statuses
- [ ] Final ticket closure is an audit row: `role=audit`, `audit_result=pass`, `evidence` populated
- [ ] Running `--apply` twice → "no changes" + no new backup directory
- [ ] `.gitignore` contains the four operational-artifact lines (idempotent)
- [ ] All commits on `main`, working tree clean
- [ ] `go test ./... -count=1 -race` green
- [ ] `go vet ./...` clean
- [ ] `gofmt -l .` empty

---

## Out of Scope (deferred to later plans)

- Migration from spec §11 archive→archive-originals option is in; renaming the flag is not.
- A `--rollback` to restore from `.backup/<ts>/` is not implemented (manual restore documented if needed).
- Multi-project batch import (run `import legacy` once per project — same as `init`).
- Plan 3 (viewer) and Plan 4 (hooks/instructions/release) remain untouched by Plan 2.
