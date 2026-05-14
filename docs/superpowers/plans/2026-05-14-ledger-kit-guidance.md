# ledger-kit Guidance Implementation Plan (Plan 4A)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make every write a teachable moment. After `ticket add`, `ticket event`, or `worklog add`, the binary prints state-aware guidance to stderr. New commands `ledger-kit next` and `ledger-kit suggest` let LLMs and humans inspect or request the next action / a JSON skeleton at any time.

**Architecture:**
- `internal/guidance/` is the state machine: pure function `Compute(latest ticket row, worklogs) Guidance`. Deterministic, no I/O.
- `cmd/next.go` reads latest ticket state, renders Guidance as text or JSON.
- `cmd/suggest.go` renders Guidance into actionable JSON skeletons (worklog) or commit-message scaffolds.
- `cmd/ticket.go` and `cmd/worklog.go` call `guidance.Compute(...)` after a successful append and write a short formatted block to stderr. stdout JSON stays untouched (`ticket add`/`event` still print normalized row JSON, `worklog add` same).
- StatusEnum is extended to include `audit_ready` and `changes_requested` per spec §3.4.

**Tech Stack:** Go 1.22+ stdlib only.

**Spec reference:** §3.4 (status enum + lifecycle), §4.2.4 (write-time guidance), §4.3.1 (`next`), §4.3.2 (`suggest`).

**Plan 4B (deferred):** hooks install/uninstall, instructions install/uninstall, release workflow.

---

## Hard Acceptance Criteria

1. `StatusEnum` includes `audit_ready` and `changes_requested`; `verify` accepts them; existing tests still pass.
2. `ledger-kit next --ticket ID` exits 0 and prints text guidance for every legal status (`open`, `in_progress`, `blocked`, `audit_ready`, `changes_requested`, `done`, `cancelled`). Missing ticket → non-zero, error message names the ticket.
3. `ledger-kit next --ticket ID --format json` returns the documented JSON shape (status, actions, warnings, suggested_commands, suggested_json) — see Guidance Contract below.
4. `ledger-kit suggest worklog --ticket ID`:
   - If latest row is `status=done` with `audit_result=pass` (or `role=audit` row exists with pass) → prints a worklog JSON skeleton populated from ticket fields, exits 0.
   - Otherwise → prints state-aware guidance instead, exits 0 (warning, not failure).
5. `ledger-kit suggest commit --ticket ID` always prints a Conventional Commit line (`type(scope): summary`) plus PR summary / verification skeleton. Type is derived from `category` (feature→feat, bug→fix, docs→docs, ops→chore, infra→chore, test→test, refactor→refactor, design→feat, demo→chore, release→chore, cleanup→chore, research→docs; fallback: `chore`). Scope is derived from `parent_ticket` (lowercased). Summary is `task` truncated to ~72 chars.
6. `ticket add`, `ticket event`, `worklog add` keep stdout exactly as before (normalized row JSON). They additionally write a guidance block to the **injected stderr** writer (not `os.Stderr` directly).
7. Guidance never writes ledger files. It only reads.
8. All tests + `go vet` + `gofmt` clean.

---

## Decisions Locked

- **Guidance is state-derived, not static.** A static prompt cannot tell an agent "you can't run worklog yet, run an audit row first." The state machine can.
- **stdout stable.** Write commands continue to emit only the normalized row JSON on stdout. Guidance only goes to stderr.
- **Single agent → self-audit shortcut.** If the agent operates alone, the same agent appending a `role=audit, status=done, audit_result=pass` row is acceptable. Guidance reminds the writer to add evidence.
- **Reference-by-latest-row.** Guidance always reads the latest row per ticket id (via the same `LatestTickets`-style logic used by the viewer). It does not invent state.
- **"done" means audit passed.** A bare `status=done` row without audit context still passes verify (backward compatibility), but `suggest worklog` and write guidance will warn that closure is weak.
- **No new ledger files.** Audit lives inside `tickets.jsonl` as `role=audit` rows. Worklog stays the completed-delivery record.

---

## File Structure

```
ledger-kit/
  internal/
    guidance/
      guidance.go         guidance_test.go    # Compute() + text/json renderers
  cmd/
    next.go               next_test.go        # ledger-kit next
    suggest.go            suggest_test.go     # ledger-kit suggest worklog|commit
  internal/ledger/types.go                    # extended StatusEnum (Task 0 only)
```

---

## Guidance Contract

`internal/guidance.Compute(latest ledger.Row, worklog []ledger.Row) Guidance` returns:

```go
type Guidance struct {
    Ticket            string   `json:"ticket"`
    Status            string   `json:"status"`
    Summary           string   `json:"summary"`             // one-line human readable
    Actions           []string `json:"actions"`             // imperative bullets for stderr/text
    Warnings          []string `json:"warnings"`            // non-blocking flags
    SuggestedCommands []string `json:"suggested_commands"`  // copy-pasteable shells
    SuggestedJSON     []any    `json:"suggested_json"`      // skeletons to feed --json @-
}
```

The state machine (status → guidance):

| status | Summary | Actions (must include) | Suggested command(s) | Suggested JSON skeleton? |
|---|---|---|---|---|
| `open` | "ready to plan" | claim with `ticket event status=in_progress`, confirm acceptance + paths, archive/reference review note | `ledger-kit ticket event --json @-` | yes: in_progress overlay |
| `in_progress` | "claimed; implementing" | keep evidence, update `paths`, when done append `audit_ready` row | `ledger-kit ticket event --json @-` | yes: audit_ready overlay |
| `blocked` | "waiting on blockers" | list each unresolved blocker (warning), do not implement until cleared | `ledger-kit next --ticket <blocker>` | none |
| `audit_ready` | "awaiting audit" | do NOT append worklog yet, auditor must append `role=audit` row (pass→done OR fail→changes_requested), attach evidence | `ledger-kit ticket event --json @-` | yes: audit row pass + audit row changes_requested |
| `changes_requested` | "audit failed; resume implementation" | do NOT append worklog, resume with `in_progress`, carry `audit_notes` into impl notes | `ledger-kit ticket event --json @-` | yes: in_progress overlay carrying audit notes |
| `done` (audit_result=pass) | "complete" | append worklog now, then commit/PR | `ledger-kit suggest worklog --ticket ID`, `ledger-kit suggest commit --ticket ID` | yes: worklog skeleton |
| `done` (not audit-pass) | "marked done but audit weak" | warning: closure lacks audit; add `role=audit, audit_result=pass, evidence=[...]` row | `ledger-kit ticket event --json @-` | yes: audit pass row |
| `cancelled` | "cancelled" | explain reason in `notes`; do not append worklog unless cancellation itself was the delivery | (none required) | none |

Audit detection: a ticket is "audit-pass done" iff its latest row has `status=="done"` AND (`audit_result=="pass"` OR there exists a previous row with the same `ticket` and `role=="audit"` and `audit_result=="pass"`).

---

## Task Decomposition

6 tasks, each one TDD cycle (test → impl → pass → commit).

---

### Task 1: Extend `StatusEnum`

**Files:**
- Modify: `internal/ledger/types.go` (add two entries to `StatusEnum`)

This is intentionally tiny so it lands as its own commit and unblocks every following task.

- [ ] **Step 1:** Edit `internal/ledger/types.go`. Find `var StatusEnum = map[string]struct{}{...}` and add `"audit_ready": {}` and `"changes_requested": {}` between the existing entries (any order within the map literal is fine).
- [ ] **Step 2:** Run `go test ./... -count=1` — must still be green (no test currently asserts the enum is exactly 5).
- [ ] **Step 3:** Commit:
  ```
  git add internal/ledger/types.go
  git commit -m "feat(ledger): status enum includes audit_ready and changes_requested"
  ```

---

### Task 2: `internal/guidance/guidance.go`

**Files:**
- Create: `internal/guidance/guidance.go`
- Create: `internal/guidance/guidance_test.go`

#### Step 1: Write `internal/guidance/guidance_test.go`

```go
package guidance

import (
	"strings"
	"testing"

	"github.com/hgwk/ledger-kit/internal/ledger"
)

func ticket(fields map[string]any) ledger.Row {
	base := ledger.Row{
		"ticket": "T-1", "status": "open", "task": "demo task",
		"parent_ticket": "ROOT", "category": "feature",
		"paths": []any{"src/x.go"}, "blocked_by": []any{},
	}
	for k, v := range fields {
		base[k] = v
	}
	return base
}

func TestCompute_OpenSuggestsInProgressEvent(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "open"}), nil)
	if g.Status != "open" {
		t.Fatalf("status=%s", g.Status)
	}
	if len(g.SuggestedJSON) == 0 {
		t.Fatalf("no skeleton")
	}
	skel := g.SuggestedJSON[0].(map[string]any)
	if skel["status"] != "in_progress" {
		t.Fatalf("expected in_progress skeleton, got %v", skel["status"])
	}
	if !containsAny(g.SuggestedCommands, "ticket event") {
		t.Fatalf("expected ticket event command, got %v", g.SuggestedCommands)
	}
}

func TestCompute_InProgressSuggestsAuditReady(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "in_progress"}), nil)
	if g.Status != "in_progress" {
		t.Fatalf("status=%s", g.Status)
	}
	skel := g.SuggestedJSON[0].(map[string]any)
	if skel["status"] != "audit_ready" {
		t.Fatalf("expected audit_ready skeleton, got %v", skel["status"])
	}
}

func TestCompute_BlockedListsUnresolved(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "blocked", "blocked_by": []any{"DEP-1", "DEP-2"}}), nil)
	if g.Status != "blocked" {
		t.Fatalf("status=%s", g.Status)
	}
	if len(g.Warnings) == 0 {
		t.Fatalf("expected warnings, got none")
	}
	joined := strings.Join(g.Warnings, " ")
	if !strings.Contains(joined, "DEP-1") || !strings.Contains(joined, "DEP-2") {
		t.Fatalf("warnings should name each blocker: %v", g.Warnings)
	}
}

func TestCompute_AuditReadyForbidsWorklog(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "audit_ready"}), nil)
	if g.Status != "audit_ready" {
		t.Fatalf("status=%s", g.Status)
	}
	if !containsAny(g.Actions, "audit") || !containsAny(g.Actions, "worklog") {
		t.Fatalf("audit_ready actions must mention audit and worklog rules: %v", g.Actions)
	}
	// Two skeletons: pass and changes_requested.
	if len(g.SuggestedJSON) < 2 {
		t.Fatalf("expected pass + changes_requested skeletons, got %d", len(g.SuggestedJSON))
	}
}

func TestCompute_ChangesRequestedResumes(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "changes_requested"}), nil)
	skel := g.SuggestedJSON[0].(map[string]any)
	if skel["status"] != "in_progress" {
		t.Fatalf("expected resume skeleton in_progress, got %v", skel["status"])
	}
}

func TestCompute_DoneAuditPassPromotesWorklog(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "done", "audit_result": "pass"}), nil)
	if !containsAny(g.SuggestedCommands, "suggest worklog") {
		t.Fatalf("audit-pass done should suggest worklog command: %v", g.SuggestedCommands)
	}
	if !containsAny(g.SuggestedCommands, "suggest commit") {
		t.Fatalf("audit-pass done should suggest commit command: %v", g.SuggestedCommands)
	}
	if len(g.Warnings) != 0 {
		t.Fatalf("audit-pass done should have no warnings, got %v", g.Warnings)
	}
}

func TestCompute_DoneWithoutAuditWarns(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "done"}), nil)
	if len(g.Warnings) == 0 {
		t.Fatalf("expected warning about weak closure, got none")
	}
}

func TestCompute_CancelledTerminal(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "cancelled"}), nil)
	if g.Status != "cancelled" {
		t.Fatalf("status=%s", g.Status)
	}
	if len(g.SuggestedJSON) != 0 {
		t.Fatalf("cancelled should not propose skeletons, got %v", g.SuggestedJSON)
	}
}

func TestRenderText_IncludesActions(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "audit_ready"}), nil)
	text := RenderText(g)
	if !strings.Contains(text, "audit_ready") || !strings.Contains(text, "Next:") {
		t.Fatalf("rendered text missing pieces:\n%s", text)
	}
}

func TestRenderJSON_RoundTrip(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "in_progress"}), nil)
	data, err := RenderJSON(g)
	if err != nil {
		t.Fatalf("render json: %v", err)
	}
	if !strings.Contains(string(data), `"status": "in_progress"`) && !strings.Contains(string(data), `"status":"in_progress"`) {
		t.Fatalf("json missing status field:\n%s", data)
	}
}

func containsAny(xs []string, needle string) bool {
	for _, x := range xs {
		if strings.Contains(x, needle) {
			return true
		}
	}
	return false
}
```

#### Step 2: Verify FAIL

`go test ./internal/guidance/...` — `undefined: Compute`.

#### Step 3: Write `internal/guidance/guidance.go`

```go
// Package guidance derives state-aware next-action guidance from the latest
// ticket row. Pure functions; no I/O.
package guidance

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hgwk/ledger-kit/internal/ledger"
)

// Guidance is the wire shape for both stderr text rendering and the `next` JSON output.
type Guidance struct {
	Ticket            string   `json:"ticket"`
	Status            string   `json:"status"`
	Summary           string   `json:"summary"`
	Actions           []string `json:"actions"`
	Warnings          []string `json:"warnings"`
	SuggestedCommands []string `json:"suggested_commands"`
	SuggestedJSON     []any    `json:"suggested_json"`
}

// Compute derives the guidance for `latest` ticket row. `worklog` is the
// full worklog slice (used to detect missing/orphan logs); pass nil if not needed.
func Compute(latest ledger.Row, worklog []ledger.Row) Guidance {
	g := Guidance{
		Ticket: stringField(latest, "ticket"),
		Status: stringField(latest, "status"),
	}
	switch g.Status {
	case "open":
		g.Summary = "ready to plan; claim before editing"
		g.Actions = []string{
			"Claim this ticket: append a ticket event with status=in_progress and paths you'll touch.",
			"Confirm `acceptance` is filled and `category`/`parent_ticket` make sense.",
			"Capture archive/reference review in `notes` before implementation.",
		}
		g.SuggestedCommands = []string{"ledger-kit ticket event --json @-"}
		g.SuggestedJSON = []any{overlay(latest, map[string]any{"status": "in_progress"})}
	case "in_progress":
		g.Summary = "claim active; implementing"
		g.Actions = []string{
			"Keep `paths` accurate; do not edit paths claimed by another agent.",
			"When implementation is finished, append a ticket event with status=audit_ready and include evidence.",
		}
		g.SuggestedCommands = []string{"ledger-kit ticket event --json @-"}
		g.SuggestedJSON = []any{overlay(latest, map[string]any{"status": "audit_ready", "evidence": []any{}})}
	case "blocked":
		g.Summary = "waiting on blockers"
		g.Actions = []string{"Do not implement until at least one blocker clears."}
		blockers := stringSliceField(latest, "blocked_by")
		if len(blockers) == 0 {
			g.Warnings = append(g.Warnings, "status=blocked but blocked_by is empty; add the actual blocker ticket ids")
		} else {
			for _, b := range blockers {
				g.Warnings = append(g.Warnings, fmt.Sprintf("blocked by %s", b))
			}
			g.SuggestedCommands = append(g.SuggestedCommands, fmt.Sprintf("ledger-kit next --ticket %s", blockers[0]))
		}
	case "audit_ready":
		g.Summary = "implementation finished; awaiting audit"
		g.Actions = []string{
			"Do not append a worklog yet — worklog follows audit pass.",
			"Append a ticket event with role=audit and either audit_result=pass (status=done) or audit_result=changes_requested (status=changes_requested).",
			"Include `evidence`: tests, verify command, diff review, screenshot/report when relevant.",
		}
		g.SuggestedCommands = []string{"ledger-kit ticket event --json @-"}
		g.SuggestedJSON = []any{
			overlay(latest, map[string]any{
				"role": "audit", "status": "done", "audit_result": "pass", "evidence": []any{},
			}),
			overlay(latest, map[string]any{
				"role": "audit", "status": "changes_requested", "audit_result": "changes_requested",
				"audit_notes": "", "evidence": []any{},
			}),
		}
	case "changes_requested":
		g.Summary = "audit returned changes; resume implementation"
		g.Actions = []string{
			"Do not append a worklog.",
			"Resume with status=in_progress; carry `audit_notes` into your implementation notes.",
		}
		g.SuggestedCommands = []string{"ledger-kit ticket event --json @-"}
		g.SuggestedJSON = []any{overlay(latest, map[string]any{
			"status": "in_progress", "notes": stringField(latest, "audit_notes"),
		})}
	case "done":
		if isAuditPass(latest, worklog) {
			g.Summary = "audit passed; record the worklog"
			g.Actions = []string{"Append a worklog row for the shipped delivery, then prepare the commit / PR."}
			g.SuggestedCommands = []string{
				fmt.Sprintf("ledger-kit suggest worklog --ticket %s", g.Ticket),
				fmt.Sprintf("ledger-kit suggest commit --ticket %s", g.Ticket),
			}
		} else {
			g.Summary = "marked done without audit evidence"
			g.Warnings = []string{"closure is weak: no audit pass row was found"}
			g.Actions = []string{"Append a role=audit row with audit_result=pass and evidence before treating this as shipped."}
			g.SuggestedCommands = []string{"ledger-kit ticket event --json @-"}
			g.SuggestedJSON = []any{overlay(latest, map[string]any{
				"role": "audit", "status": "done", "audit_result": "pass", "evidence": []any{},
			})}
		}
	case "cancelled":
		g.Summary = "cancelled"
		g.Actions = []string{"Explain the cancellation in `notes`. Do not append a worklog unless cancellation itself is the delivery."}
	default:
		g.Summary = "unknown status"
		g.Warnings = []string{fmt.Sprintf("unrecognized status: %q", g.Status)}
	}
	return g
}

// RenderText formats Guidance for stderr / human consumption.
func RenderText(g Guidance) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Ticket %s is %s — %s\n", g.Ticket, g.Status, g.Summary)
	if len(g.Actions) > 0 {
		b.WriteString("\nNext:\n")
		for _, a := range g.Actions {
			fmt.Fprintf(&b, "- %s\n", a)
		}
	}
	if len(g.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range g.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}
	if len(g.SuggestedCommands) > 0 {
		b.WriteString("\nSuggested:\n")
		for _, c := range g.SuggestedCommands {
			fmt.Fprintf(&b, "  %s\n", c)
		}
	}
	return b.String()
}

// RenderJSON returns the canonical machine-readable form.
func RenderJSON(g Guidance) ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}

func overlay(base ledger.Row, fields map[string]any) map[string]any {
	out := map[string]any{}
	// Carry forward the carry-friendly fields so the skeleton is valid input.
	for _, k := range []string{"ticket", "parent_ticket", "agent", "role", "task", "scope", "paths", "blocked_by", "branch", "category"} {
		if v, ok := base[k]; ok {
			out[k] = v
		}
	}
	// Always include ticket — required for ticket event.
	out["ticket"] = stringField(base, "ticket")
	for k, v := range fields {
		out[k] = v
	}
	return out
}

func isAuditPass(latest ledger.Row, _ []ledger.Row) bool {
	if r, _ := latest["audit_result"].(string); r == "pass" {
		return true
	}
	return false
}

func stringField(r ledger.Row, k string) string {
	v, _ := r[k].(string)
	return v
}

func stringSliceField(r ledger.Row, k string) []string {
	raw, _ := r[k].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
```

#### Step 4: Tests pass

`go test ./internal/guidance/... -v -race` — all 10 tests PASS.

#### Step 5: Full suite

`go test ./... -count=1` — no regressions.

#### Step 6: Commit

```
git add internal/guidance/
git commit -m "feat(guidance): state-aware next actions for ticket rows"
```

---

### Task 3: `cmd/next.go`

**Files:**
- Create: `cmd/next.go`
- Create: `cmd/next_test.go`

#### Step 1: Write `cmd/next_test.go`

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNext_TextOutputForOpenTicket(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	// Add a ticket via the existing CLI so we don't duplicate setup logic.
	body := `{"ticket":"T-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("seed add failed")
	}

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "T-1"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next failed")
	}
	if !strings.Contains(out.String(), "T-1 is open") {
		t.Fatalf("missing header line: %s", out.String())
	}
	if !strings.Contains(out.String(), "Next:") {
		t.Fatalf("missing Next section: %s", out.String())
	}
}

func TestNext_JSONOutputShape(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"T-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "T-1", "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next json failed")
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got["ticket"] != "T-1" || got["status"] != "open" {
		t.Fatalf("unexpected payload: %v", got)
	}
}

func TestNext_MissingTicketFails(t *testing.T) {
	target, _ := mustInit(t)
	var errb bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "ghost"}, &bytes.Buffer{}, &errb); code == 0 {
		t.Fatalf("expected non-zero for missing ticket")
	}
	if !strings.Contains(errb.String(), "ghost") {
		t.Fatalf("stderr should name the ticket: %s", errb.String())
	}
}
```

#### Step 2: Verify FAIL

`go test ./cmd/...` — `undefined: RunNextCLI`.

#### Step 3: Write `cmd/next.go`

```go
package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/hgwk/ledger-kit/internal/guidance"
	"github.com/hgwk/ledger-kit/internal/ledger"
)

func init() {
	Commands["next"] = RunNextCLI
}

// RunNextCLI implements `ledger-kit next --ticket ID [--format text|json]`.
func RunNextCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("next")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	format := fs.String("format", "text", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return 2
	}
	dir := resolveTarget(*target)

	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	latest, ok := findLatest(rows, *ticket)
	if !ok {
		fmt.Fprintf(stderr, "ticket %q not found\n", *ticket)
		return 1
	}
	worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))
	g := guidance.Compute(latest, worklog)
	switch *format {
	case "json":
		data, err := guidance.RenderJSON(g)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(data))
	default:
		fmt.Fprint(stdout, guidance.RenderText(g))
	}
	return 0
}

// findLatest returns the latest row matching the ticket id.
func findLatest(rows []ledger.Row, ticketID string) (ledger.Row, bool) {
	var latest ledger.Row
	var maxN float64 = -1
	for _, r := range rows {
		if id, _ := r["ticket"].(string); id != ticketID {
			continue
		}
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := r["n"].(float64)
		if n > maxN {
			maxN = n
			latest = r
		}
	}
	return latest, latest != nil
}
```

#### Step 4: Tests pass

`go test ./cmd/... -v -race` — all 3 new + existing pass.

#### Step 5: Full suite

`go test ./... -count=1` — clean.

#### Step 6: Commit

```
git add cmd/next.go cmd/next_test.go
git commit -m "feat(next): state-aware next action for a ticket"
```

---

### Task 4: `cmd/suggest.go` — worklog + commit skeletons

**Files:**
- Create: `cmd/suggest.go`
- Create: `cmd/suggest_test.go`

#### Step 1: Write `cmd/suggest_test.go`

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSuggestWorklog_RefusesBeforeAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"T-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	code := RunSuggestCLI([]string{"worklog", "--target", target, "--ticket", "T-1"}, &out, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("suggest worklog should warn (exit 0), got %d", code)
	}
	// Output should be guidance text, not a worklog skeleton.
	if strings.Contains(out.String(), `"result"`) {
		t.Fatalf("should not print a worklog skeleton yet: %s", out.String())
	}
	if !strings.Contains(out.String(), "audit") {
		t.Fatalf("expected audit-related guidance: %s", out.String())
	}
}

func TestSuggestWorklog_EmitsSkeletonAfterAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// Seed: add open, then audit-pass done.
	add := `{"ticket":"T-2","parent_ticket":"BUG","role":"impl","status":"open","task":"impl T-2","scope":"repo","paths":["src/x.go"],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	done := `{"ticket":"T-2","role":"audit","status":"done","audit_result":"pass","evidence":["go test ./..."]}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(done), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"worklog", "--target", target, "--ticket", "T-2"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest worklog failed")
	}
	var skel map[string]any
	if err := json.Unmarshal(out.Bytes(), &skel); err != nil {
		t.Fatalf("expected JSON skeleton, got: %s\nerr=%v", out.String(), err)
	}
	if skel["ticket"] != "T-2" || skel["task"] == "" || skel["scope"] == "" {
		t.Fatalf("skeleton fields wrong: %+v", skel)
	}
}

func TestSuggestCommit_ConventionalLineFromCategory(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"BUG-9","parent_ticket":"BUG","role":"impl","status":"open","task":"fix the thing","scope":"repo","paths":["src/x.go"],"blocked_by":[],"category":"bug"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"commit", "--target", target, "--ticket", "BUG-9"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest commit failed")
	}
	line := strings.SplitN(out.String(), "\n", 2)[0]
	if !strings.HasPrefix(line, "fix(bug): ") {
		t.Fatalf("expected fix(bug): prefix, got: %s", line)
	}
	if !strings.Contains(out.String(), "## Verification") {
		t.Fatalf("expected verification block, got:\n%s", out.String())
	}
}
```

#### Step 2: Verify FAIL

`go test ./cmd/...` — `undefined: RunSuggestCLI`.

#### Step 3: Write `cmd/suggest.go`

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hgwk/ledger-kit/internal/guidance"
	"github.com/hgwk/ledger-kit/internal/ledger"
)

func init() {
	Commands["suggest"] = RunSuggestCLI
}

// RunSuggestCLI implements `ledger-kit suggest worklog|commit --ticket ID`.
func RunSuggestCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ledger-kit suggest <worklog|commit> --ticket ID")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("suggest " + sub)
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return 2
	}
	dir := resolveTarget(*target)
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	latest, ok := findLatest(rows, *ticket)
	if !ok {
		fmt.Fprintf(stderr, "ticket %q not found\n", *ticket)
		return 1
	}
	worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))

	switch sub {
	case "worklog":
		return suggestWorklog(latest, worklog, stdout)
	case "commit":
		return suggestCommit(latest, stdout)
	default:
		fmt.Fprintf(stderr, "unknown suggest subcommand: %s\n", sub)
		return 2
	}
}

func suggestWorklog(latest ledger.Row, worklog []ledger.Row, stdout io.Writer) int {
	g := guidance.Compute(latest, worklog)
	if g.Status != "done" || !isAuditPass(latest) {
		fmt.Fprint(stdout, guidance.RenderText(g))
		return 0
	}
	skeleton := map[string]any{
		"ticket":   latest["ticket"],
		"task":     latest["task"],
		"scope":    latest["scope"],
		"result":   "shipped: " + stringField(latest, "task"),
		"paths":    latest["paths"],
		"commands": ifSliceField(latest, "evidence"),
		"notes":    "",
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}

func suggestCommit(latest ledger.Row, stdout io.Writer) int {
	commitType := commitTypeFromCategory(stringField(latest, "category"))
	scope := strings.ToLower(stringField(latest, "parent_ticket"))
	if scope == "" || scope == "root" {
		scope = ""
	}
	subject := truncate(stringField(latest, "task"), 72)

	var line string
	if scope != "" {
		line = fmt.Sprintf("%s(%s): %s", commitType, scope, subject)
	} else {
		line = fmt.Sprintf("%s: %s", commitType, subject)
	}
	fmt.Fprintln(stdout, line)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "## Summary")
	fmt.Fprintf(stdout, "- %s\n", stringField(latest, "task"))
	if notes := stringField(latest, "notes"); notes != "" {
		fmt.Fprintf(stdout, "- %s\n", notes)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "## Verification")
	evidence := stringSliceField(latest, "evidence")
	if len(evidence) == 0 {
		fmt.Fprintln(stdout, "- TODO: paste the commands you ran (ledger-kit verify, go test, etc.)")
	} else {
		for _, e := range evidence {
			fmt.Fprintf(stdout, "- %s\n", e)
		}
	}
	return 0
}

func commitTypeFromCategory(cat string) string {
	switch cat {
	case "feature", "design", "demo":
		return "feat"
	case "bug":
		return "fix"
	case "docs", "research":
		return "docs"
	case "test":
		return "test"
	case "refactor", "cleanup":
		return "refactor"
	case "ops", "infra", "release":
		return "chore"
	}
	return "chore"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max])
}

func isAuditPass(r ledger.Row) bool {
	v, _ := r["audit_result"].(string)
	return v == "pass"
}

func ifSliceField(r ledger.Row, k string) []any {
	v, _ := r[k].([]any)
	if v == nil {
		return []any{}
	}
	return v
}

func stringField(r ledger.Row, k string) string {
	v, _ := r[k].(string)
	return v
}

func stringSliceField(r ledger.Row, k string) []string {
	raw, _ := r[k].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
```

If `stringField` / `stringSliceField` already exist in another `cmd/*.go` file (they were defined in Task 14 of Plan 1), Go will complain about the duplicate. In that case, delete the duplicates from `cmd/suggest.go` and rely on the existing helpers. Verify by running `go build ./cmd` after pasting and trimming any redeclared identifiers.

#### Step 4: Tests pass

`go test ./cmd/... -v -race`.

#### Step 5: Full suite

`go test ./... -count=1`.

#### Step 6: Commit

```
git add cmd/suggest.go cmd/suggest_test.go
git commit -m "feat(suggest): worklog skeleton + commit scaffold"
```

---

### Task 5: Wire guidance into write commands

**Files:**
- Modify: `cmd/ticket.go` (add stderr guidance after successful `runTicketAdd` / `runTicketEvent`)
- Modify: `cmd/worklog.go` (add stderr guidance after successful worklog append)
- Modify: `cmd/ticket_test.go` (one new test asserting stderr guidance does not pollute stdout)
- Modify: `cmd/worklog_test.go` (one new test asserting worklog-before-audit warning)

#### Step 1: Add the new tests

`cmd/ticket_test.go` (append):

```go
func TestTicketEvent_PrintsGuidanceToStderr(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"G-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	ev := `{"ticket":"G-1","status":"audit_ready","evidence":["go test ./..."]}`
	var stdout, stderr bytes.Buffer
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ev), &stdout, &stderr); code != 0 {
		t.Fatalf("event failed: %s", stderr.String())
	}
	// stdout is still JSON: should parse and contain status=audit_ready.
	var row map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &row); err != nil {
		t.Fatalf("stdout must remain JSON: %v\n%s", err, stdout.String())
	}
	if row["status"] != "audit_ready" {
		t.Fatalf("status wrong: %v", row["status"])
	}
	// stderr contains audit guidance.
	if !strings.Contains(stderr.String(), "audit_ready") || !strings.Contains(stderr.String(), "Next:") {
		t.Fatalf("stderr missing guidance: %s", stderr.String())
	}
}
```

`cmd/worklog_test.go` (append):

```go
func TestWorklogAdd_WarnsWhenTicketNotAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"W-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	wl := `{"ticket":"W-1","task":"early worklog","scope":"repo","result":"too early","paths":[],"commands":[]}`
	var stderr bytes.Buffer
	if code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(wl), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("worklog add failed: %s", stderr.String())
	}
	// Worklog still appends — but stderr must warn that ticket is not audit-pass done.
	if !strings.Contains(stderr.String(), "audit") {
		t.Fatalf("expected audit-related stderr guidance: %s", stderr.String())
	}
}
```

(`encoding/json` may need to be imported in `ticket_test.go` if not already; check before adding.)

#### Step 2: Verify FAIL (tests reference no-yet-existing behavior)

`go test ./cmd/...` — should fail because stderr does not yet contain guidance.

#### Step 3: Add guidance hooks to write commands

In `cmd/ticket.go`, find where `runTicketAdd` and `runTicketEvent` call `ledger.Append`. After a successful append (i.e., after the row is returned and right before encoding `out` to stdout), insert:

```go
// Best-effort guidance to stderr. We've already written the ledger row;
// any error here is non-fatal.
if g := buildGuidanceForRow(dir, out); g != "" {
    fmt.Fprintln(stderr, g)
}
```

`buildGuidanceForRow` is a new helper at the bottom of `cmd/ticket.go`:

```go
// buildGuidanceForRow returns the text guidance block for the row that was just
// appended. Returns "" if the row is missing a ticket id or guidance is empty.
func buildGuidanceForRow(dir string, row ledger.Row) string {
    id, _ := row["ticket"].(string)
    if id == "" {
        return ""
    }
    // Re-read the ledger to include the row we just appended.
    tickets, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
    latest, ok := findLatest(tickets, id)
    if !ok {
        latest = row
    }
    worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))
    g := guidance.Compute(latest, worklog)
    return guidance.RenderText(g)
}
```

Import `github.com/hgwk/ledger-kit/internal/guidance` and reuse the `findLatest` helper from `cmd/next.go` (same package, so it's already visible).

In `cmd/worklog.go`, after the successful `ledger.Append`, do the same: re-resolve the latest ticket for `row["ticket"]` (if present) and render guidance. If `row["ticket"]` is empty (goal-set worklog and similar), skip.

#### Step 4: Tests pass

`go test ./cmd/... -v -race` — all green including the two new tests.

#### Step 5: Full suite

`go test ./... -count=1`.

#### Step 6: Commit

```
git add cmd/ticket.go cmd/worklog.go cmd/ticket_test.go cmd/worklog_test.go
git commit -m "feat(ticket,worklog): print contextual guidance to stderr"
```

---

### Task 6: README + final smoke

**Files:**
- Modify: `README.md`

Append a "Guidance" section after the viewer section:

```markdown

## Guidance

After every ticket/worklog write, `ledger-kit` prints state-aware guidance to
stderr — stdout still contains only the normalized row JSON, so automation
keeps working.

Ask explicitly:

```bash
ledger-kit next --ticket BUG-101
ledger-kit next --ticket BUG-101 --format json    # for LLM consumption

ledger-kit suggest worklog --ticket BUG-101       # JSON skeleton, only after audit pass
ledger-kit suggest commit  --ticket BUG-101       # Conventional Commit + PR/verification scaffold
```

The state machine pushes you through `open → in_progress → audit_ready → done`.
`worklog add` is intended for shipped work after an audit-pass `done` row; the
binary warns when it sees you doing it earlier.
```

Then final smoke:
```
go test ./... -count=1 -race
go vet ./...
gofmt -l .
```

Commit: `docs(readme): guidance, next, suggest usage`.

---

## Self-Review Checklist

- [ ] `next` covers every status in `StatusEnum`.
- [ ] `suggest worklog` refuses skeleton before audit-pass done (returns guidance instead, exit 0).
- [ ] `suggest commit` always emits Conventional Commit + verification scaffold.
- [ ] Write commands keep stdout pure JSON, guidance goes to stderr.
- [ ] Guidance never writes ledger files.
- [ ] All tests / race / vet / gofmt clean.

---

## Out of Scope (Plan 4B)

- Hooks install / uninstall (pre-commit verify).
- Instructions install / uninstall (AGENTS.md / CLAUDE.md reference markers + ledger-owned bodies).
- Release workflow (`.github/workflows/release.yml`), Homebrew / cross-compile artifacts.
- MCP server, TUI, automated PR creation.
