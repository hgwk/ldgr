# ledger-kit Viewer Implementation Plan (Plan 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** `ledger-kit view` — a single-binary, dependency-free HTTP dashboard that reads registered projects' ledgers and renders goal / ticket tree / worklog timeline / insights in one page. Mirrors the UX validated by `templates/serve-ledger.cjs` (the Node prototype), but ported to Go with `embed.FS`.

**Architecture:**
- `internal/viewer/` holds aggregation (latest-row semantics, tree, insights), HTTP handlers, and the embedded SPA assets.
- `cmd/view.go` is the CLI front-end.
- All data is read from disk on each request (cheap, deterministic, no cache). 3–5s client-side polling.
- No external Go deps. No websockets. No build step for the frontend (vanilla JS + CSS).

**Tech Stack:** Go 1.22+ stdlib (`net/http`, `embed`, `encoding/json`). Frontend: vanilla HTML/CSS/JS.

**Spec reference:** `docs/superpowers/specs/2026-05-14-ledger-kit-go-design.md` §7.2

---

## Hard Acceptance Criteria

1. `ledger-kit view [--port 3030] [--target PATH]` boots an HTTP server on the chosen port.
2. With no `--target`: server reads `~/.ledger-kit/registry.json` and exposes every registered project. With `--target X`: only that project (config must exist at `X/ledger/config.json`).
3. All 5 API endpoints return JSON with documented shape (see §API below). Endpoints never write to disk.
4. Latest-row semantics: ticket state = row with greatest `n` per `ticket` id. Rows pointed to by `invalidates_n` are excluded from view, surfaced as `insights.invalidated`.
5. Tree structure: tickets grouped under their `parent_ticket`, sorted by latest `ts`.
6. Insights mirror the Node prototype: `readyQueue`, `topBlockers`, `staleInProgress`, `closedWithoutWorklog`, `worklogsWithoutTicket`, `invalidated`.
7. Binary is single static file. `go build .` produces a working server with frontend.
8. Running `ledger-kit view` then `curl http://localhost:3030/api/projects` returns the registry payload.
9. `go test ./...` green, `go vet ./...` clean, `gofmt -l .` empty.

---

## File Structure

```
ledger-kit/
  internal/
    viewer/
      aggregate.go         aggregate_test.go    # projections: latest, tree, insights
      server.go            server_test.go       # HTTP handlers
      embed.go                                  # //go:embed assets
      assets/
        index.html
        app.js
        style.css
  cmd/
    view.go                                     # subcommand wiring
  e2e/
    view_test.go                                # HTTP smoke against built binary
```

Working dir: `./`. Module path: `github.com/hgwk/ledger-kit`.

---

## API Contract

All responses are JSON; errors use HTTP status + `{"error":"..."}` body.

### `GET /api/projects`
Returns an array of project summaries.

```json
[
  {
    "project_id": "9f8a7c6b...",
    "slug": "myapp",
    "name": "My App",
    "display": "myapp-9f8a7c",
    "paths": ["/Users/.../myapp"],
    "goal_summary": "Ship the foo",
    "open_tickets": 4,
    "recent_worklog_ts": "2026-05-14T10:11:00Z"
  }
]
```

When a path no longer has a readable `ledger/config.json`, the entry is still returned with `"missing": true` and counts set to 0.

### `GET /api/projects/{project_id}`
Returns a single project's full state.

```json
{
  "project_id": "9f8a7c6b...",
  "slug": "myapp",
  "name": "My App",
  "display": "myapp-9f8a7c",
  "goal": { /* contents of ledger/goal.json */ },
  "counts": { "open": 4, "in_progress": 2, "blocked": 1, "done": 12, "cancelled": 0 }
}
```

### `GET /api/projects/{project_id}/tickets`
Returns the parent-grouped tree of **latest** ticket rows (invalidated rows excluded).

```json
{
  "tree": [
    {
      "parent": "BUG",
      "tickets": [
        { "ticket": "BUG-101", "status": "open", "task": "fix login", "ts": "...", "branch": "...", "paths": [...], "blocked_by": [], "n": 7 }
      ]
    }
  ]
}
```

### `GET /api/projects/{project_id}/worklog`
Returns worklog rows newest-first (latest `ts` first). Companion `invalidates_n` rows are kept (they describe legitimate "invalidate" events).

```json
{ "rows": [ { "n": 9, "ts": "...", "ticket": "BUG-101", "task": "...", "result": "...", ... } ] }
```

### `GET /api/projects/{project_id}/insights`
Same categories as the Node prototype.

```json
{
  "readyQueue":         [ { "ticket": "BUG-101", "ts": "...", "task": "..." } ],
  "topBlockers":        [ { "ticket": "BUG-9", "dependents": ["FE-2","BE-3"], "status": "in_progress" } ],
  "staleInProgress":    [ { "ticket": "FE-2", "age_ms": 86400000, "latest_worklog_n": 5 } ],
  "closedWithoutWorklog": [ { "ticket": "DOC-1", "status": "done" } ],
  "worklogsWithoutTicket":[ { "n": 8, "ticket": "ghost" } ],
  "invalidated":        [ { "n": 2, "via_n": 3, "kind": "ticket" } ],
  "staleHours": 24
}
```

`staleHours` defaults to 24. (No flag in this plan; can be made configurable later.)

---

## Decisions Locked

- **Read on demand**: every API call re-reads files. No in-memory cache. fs reads are cheap relative to JSON serialization for this scale.
- **Polling**: client refreshes `/api/projects/{id}/...` every 5 seconds. Server need not support long-polling.
- **`--target` overrides registry**: when set, only that project is visible (synthesizes a one-item project list using its `config.json`).
- **Ghost row policy**: rows with empty semantic fields (`TicketNonEmpty`/`WorklogNonEmpty` violation) are excluded from `tickets` / `worklog` responses unless they are explicitly invalidated. Invalidated rows are always excluded from tree, surfaced in `insights.invalidated`.
- **Errors**: missing project_id → 404. Failed read (e.g. malformed ledger) → 500 with the verify-style error message.
- **No auth, no CORS**: localhost only. Bind to `127.0.0.1` (not `0.0.0.0`).
- **Port collision**: if the chosen port is busy, exit non-zero with a clear message.

---

## Task Decomposition

7 tasks. Each one TDD cycle (test → impl → pass → commit). Scope-strict: subagents must not modify files outside the listed paths.

---

### Task 1: `internal/viewer/aggregate.go` — projections

**Files:**
- Create: `internal/viewer/aggregate.go`
- Create: `internal/viewer/aggregate_test.go`

This holds the pure, file-system-free logic. Server handlers (Task 2) call these.

Exports:
```go
// LatestTickets returns the latest row per ticket id, excluding rows whose
// `n` is referenced by some other row's `invalidates_n`.
func LatestTickets(rows []ledger.Row) []ledger.Row

// InvalidatedTickets returns the set of ticket-row `n` values that were
// invalidated by a later row (i.e. some row has `invalidates_n = n`).
func InvalidatedNs(rows []ledger.Row) map[int]int // n → via_n

// Tree groups latest tickets by parent_ticket and sorts each bucket by ts desc.
type TreeBucket struct {
	Parent  string       `json:"parent"`
	Tickets []ledger.Row `json:"tickets"`
}
func Tree(latest []ledger.Row) []TreeBucket

// Counts tallies statuses among latest rows.
func StatusCounts(latest []ledger.Row) map[string]int

// Insights mirrors the Node prototype's categories.
type Insights struct {
	ReadyQueue            []ledger.Row    `json:"readyQueue"`
	TopBlockers           []BlockerEntry  `json:"topBlockers"`
	StaleInProgress       []StaleEntry    `json:"staleInProgress"`
	ClosedWithoutWorklog  []ledger.Row    `json:"closedWithoutWorklog"`
	WorklogsWithoutTicket []ledger.Row    `json:"worklogsWithoutTicket"`
	Invalidated           []InvalidEntry  `json:"invalidated"`
	StaleHours            int             `json:"staleHours"`
}
type BlockerEntry struct {
	Ticket     string   `json:"ticket"`
	Dependents []string `json:"dependents"`
	Status     string   `json:"status"`
}
type StaleEntry struct {
	Ticket           string `json:"ticket"`
	Status           string `json:"status"`
	Task             string `json:"task"`
	AgeMS            int64  `json:"age_ms"`
	LatestWorklogN   int    `json:"latest_worklog_n"`
}
type InvalidEntry struct {
	N    int    `json:"n"`
	ViaN int    `json:"via_n"`
	Kind string `json:"kind"` // "ticket" or "worklog"
}
func BuildInsights(ticketRows, worklogRows []ledger.Row, now time.Time, staleHours int) Insights
```

Required tests (each must FAIL first, then PASS after impl):

- `TestLatestTickets_PicksHighestN`: two rows for same ticket, returns the one with greater `n`.
- `TestLatestTickets_DropsInvalidated`: row 1 + row 2 with `invalidates_n: 1` → row 1 not in output, row 2 not in output (it's a companion).
- `TestInvalidatedNs_DetectsCompanion`: returns `{1: 2}` for the above.
- `TestTree_GroupsByParent`: tickets with parent_ticket=BUG and FE → two buckets, alphabetical by parent.
- `TestStatusCounts`: 2 open + 1 done → `{open: 2, done: 1}`.
- `TestInsights_ReadyQueueExcludesBlocked`: open ticket with empty blocked_by enters queue; one with blocked_by populated does not.
- `TestInsights_TopBlockersAggregatesDependents`: ticket BUG-9 blocks BUG-1 and BUG-2 → BlockerEntry has 2 dependents.
- `TestInsights_StaleInProgressByLastTouch`: in_progress ticket with worklog 2 days old → age_ms > 24h.
- `TestInsights_ClosedWithoutWorklog`: done ticket with no worklog row pointing at it → listed.
- `TestInsights_WorklogsWithoutTicket`: worklog whose `ticket` field doesn't match any ticket id → listed (ticket field present-but-orphan).
- `TestInsights_InvalidatedReportsCompanion`: ghost row + companion → invalidated has `{N:1, ViaN:2, Kind:"ticket"}`.

Commit: `feat(viewer): aggregate projections for view`.

---

### Task 2: `internal/viewer/server.go` — HTTP handlers

**Files:**
- Create: `internal/viewer/server.go`
- Create: `internal/viewer/server_test.go`

Exports:
```go
// Server wires routes to the aggregation layer.
type Server struct {
	Registry func() (registry.Registry, error)        // injected so tests don't touch $HOME
	LoadProject func(projectID string) (Project, error) // resolves project_id → config + ledger paths
	Now func() time.Time
	StaleHours int
}

// NewServer constructs a Server for a global registry.
func NewServer(reg *registry.Store) *Server

// NewSingleProjectServer constructs a Server that only sees the project at targetDir.
func NewSingleProjectServer(targetDir string) (*Server, error)

// Handler returns the http.Handler with both /api/* and /, /assets/* mounted.
func (s *Server) Handler() http.Handler

// Project bundles config + ledger row data.
type Project struct {
	Config   config.Config
	Goal     ledger.Goal
	Tickets  []ledger.Row
	Worklog  []ledger.Row
	Display  string
	Missing  bool
}
```

Routes:
- `GET /api/projects`
- `GET /api/projects/{project_id}`
- `GET /api/projects/{project_id}/tickets`
- `GET /api/projects/{project_id}/worklog`
- `GET /api/projects/{project_id}/insights`
- `GET /` and `GET /assets/*` — serve embedded SPA assets

Use stdlib `net/http.ServeMux` plus a tiny dispatcher that extracts `project_id` from the URL.

Tests (httptest.Server based):
- `TestServer_ProjectsListsRegistered`: stub Registry → list contains the project_id and display.
- `TestServer_GETProjectIDReturns404OnUnknown`.
- `TestServer_TicketsTreeShape`: known project with one ticket → `{tree: [{parent:"ROOT", tickets:[{ticket:"x",...}]}]}`.
- `TestServer_WorklogReverseChronological`: worklog with ts 10:00 and 10:05 → response rows in 10:05, 10:00 order.
- `TestServer_InsightsHasReadyQueue`: open ticket with no blocked_by → readyQueue non-empty.
- `TestServer_SingleProjectMode`: NewSingleProjectServer for a temp dir with a valid ledger → `/api/projects` returns exactly one item.

Commit: `feat(viewer): HTTP handlers for projects/tickets/worklog/insights`.

---

### Task 3: `internal/viewer/assets/*` — frontend SPA

**Files:**
- Create: `internal/viewer/assets/index.html`
- Create: `internal/viewer/assets/app.js`
- Create: `internal/viewer/assets/style.css`

The frontend is vanilla JS. No build step. Three files.

`index.html` — shell with two columns and a polling timer:

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>ledger-kit</title>
<link rel="stylesheet" href="/assets/style.css">
</head>
<body>
<aside id="projects"><h1>Projects</h1><ul id="project-list"></ul></aside>
<main id="detail">
  <section id="goal-card"><h2>Goal</h2><div id="goal-body"></div></section>
  <section id="stats-card"><h2>Status</h2><div id="stats-body"></div></section>
  <section id="tree-card"><h2>Tickets</h2><div id="tree-body"></div></section>
  <section id="worklog-card"><h2>Worklog</h2><ul id="worklog-body"></ul></section>
  <section id="insights-card"><h2>Insights</h2><div id="insights-body"></div></section>
</main>
<script src="/assets/app.js"></script>
</body>
</html>
```

`app.js` — fetches `/api/projects` then loads detail for selected project; polls every 5s. Pure DOM manipulation, no framework. Handles `?project=<id>` query param for deep links.

Required UX rules (encoded as code):
- Sidebar shows `display` field. Highlights selected entry.
- Right pane sections render goal summary, status counts as horizontal bar, parent-grouped ticket tree, worklog timeline newest-first, insights cards.
- Insights section: 6 cards (readyQueue / topBlockers / staleInProgress / closedWithoutWorklog / worklogsWithoutTicket / invalidated). Each card hides itself when empty.
- Ghost tickets are not shown in tree but the count appears in the invalidated card.

`style.css` — minimal grid: sidebar 280px fixed, main fills remainder. Card sections have soft border + 16px padding. Mono font for ticket ids. No external fonts/CDN.

No unit tests for the frontend — coverage comes via Task 6's e2e HTTP shape tests.

Commit: `feat(viewer): vanilla SPA assets for dashboard`.

---

### Task 4: `internal/viewer/embed.go` — embed FS wiring

**Files:**
- Create: `internal/viewer/embed.go`

```go
package viewer

import (
	"embed"
	"io/fs"
)

//go:embed assets
var assetsFS embed.FS

// Assets returns the embedded /assets directory as a fs.FS rooted at "assets".
func Assets() fs.FS {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}
```

Update `Server.Handler()` (Task 2) to mount `http.FileServer(http.FS(Assets()))` at `/assets/` and serve `index.html` at `/`.

Verify: `go build ./...`. Add `go test ./internal/viewer/... -v` to confirm asset served:
```go
func TestServer_ServesIndex(t *testing.T) { ... GET / => 200 with content-type text/html and body containing "<title>ledger-kit</title>" ... }
```

Commit: `feat(viewer): embed assets and wire static routes`.

---

### Task 5: `cmd/view.go` — CLI wiring

**Files:**
- Create: `cmd/view.go`

```go
package cmd

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hgwk/ledger-kit/internal/viewer"
)

func init() {
	Commands["view"] = RunViewCLI
}

func RunViewCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("view")
	port := fs.Int("port", 3030, "")
	target := fs.String("target", "", "single-project mode: serve only this directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var srv *viewer.Server
	var err error
	if *target != "" {
		abs, _ := filepath.Abs(*target)
		srv, err = viewer.NewSingleProjectServer(abs)
	} else {
		store, _, sErr := DefaultRegistry()
		if sErr != nil {
			fmt.Fprintln(stderr, sErr)
			return 1
		}
		srv = viewer.NewServer(store)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "cannot bind %s: %v\n", addr, err)
		return 1
	}
	fmt.Fprintf(stdout, "ledger-kit view listening on http://%s\n", addr)
	if err := http.Serve(ln, srv.Handler()); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	_ = os.Stdin // silence unused import if Go 1.22+ tooling complains
	return 0
}
```

No new unit test for the CLI itself — the e2e test in Task 6 exercises this surface. (If the implementer wants a tiny "flag parsing rejects unknown flags" test, fine, but not required.)

Commit: `feat(view): subcommand wiring`.

---

### Task 6: `e2e/view_test.go` — HTTP smoke

**Files:**
- Create: `e2e/view_test.go`

Test plan: build binary, spin up `ledger-kit view --port <free> --target <fixture>` in the background, GET each endpoint, assert shape. Clean up the process.

```go
func TestView_EndToEnd(t *testing.T) {
	bin := buildBinary(t)
	work := t.TempDir()
	home := t.TempDir()

	// Seed a small ledger via init + a couple of ticket appends.
	env := append(os.Environ(), "LEDGER_KIT_HOME="+home, "LEDGER_AGENT=codex")
	mustRun(t, bin, env, "init", "--target", work, "--slug", "viewfix")
	mustRunStdin(t, bin, env,
		`{"ticket":"BUG-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`,
		"ticket", "add", "--target", work, "--json", "@-")

	port := freePort(t)
	cmd := exec.Command(bin, "view", "--port", fmt.Sprint(port), "--target", work)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatalf("view start: %v", err)
	}
	defer cmd.Process.Kill()
	waitForPort(t, port)

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	// /api/projects: must have one entry.
	var projects []map[string]any
	getJSON(t, base+"/api/projects", &projects)
	if len(projects) != 1 {
		t.Fatalf("want 1 project, got %d", len(projects))
	}
	pid := projects[0]["project_id"].(string)

	// /api/projects/{id}/tickets: tree has BUG bucket with one ticket.
	var ticketsResp map[string]any
	getJSON(t, base+"/api/projects/"+pid+"/tickets", &ticketsResp)
	tree, _ := ticketsResp["tree"].([]any)
	if len(tree) == 0 {
		t.Fatalf("empty tree: %+v", ticketsResp)
	}

	// /api/projects/{id}/insights: readyQueue includes BUG-1.
	var insights map[string]any
	getJSON(t, base+"/api/projects/"+pid+"/insights", &insights)
	rq, _ := insights["readyQueue"].([]any)
	if len(rq) == 0 {
		t.Fatalf("readyQueue empty: %+v", insights)
	}

	// /api/projects/{id}/worklog: empty but well-formed.
	var wl map[string]any
	getJSON(t, base+"/api/projects/"+pid+"/worklog", &wl)
	if _, ok := wl["rows"]; !ok {
		t.Fatalf("worklog missing rows: %+v", wl)
	}

	// Static: GET / returns the index page.
	resp := mustGET(t, base+"/")
	if !strings.Contains(resp, "<title>ledger-kit</title>") {
		t.Fatalf("index page missing title: %s", resp)
	}
}
```

Helpers (`mustRun`, `mustRunStdin`, `freePort`, `waitForPort`, `getJSON`, `mustGET`) are defined in this same file; keep them tight. `waitForPort` should poll the address up to 5s.

Commit: `test(e2e): viewer HTTP surface`.

---

### Task 7: README + final smoke

**Files:**
- Modify: `README.md`

Add usage block:
```markdown

## Viewing your projects

```bash
ledger-kit view                 # serves http://127.0.0.1:3030, all registered projects
ledger-kit view --port 8080     # custom port
ledger-kit view --target .      # single-project mode for the current directory
```

The dashboard polls every 5 seconds. Closing the terminal stops the server.
```

Final smoke:
```
go test ./... -count=1 -race
go vet ./...
gofmt -l .
```

Commit: `docs(readme): viewer usage`.

---

## Self-Review Checklist

- [ ] `go test ./...` green
- [ ] `go vet ./...` clean
- [ ] `gofmt -l .` empty
- [ ] `ledger-kit view --target /tmp/some-init-dir` boots and serves index page + JSON endpoints
- [ ] `/api/projects` returns the registered set (or single-project view when `--target` set)
- [ ] Tickets endpoint groups by parent, excludes invalidated rows
- [ ] Insights endpoint matches Node prototype categories
- [ ] Embedded assets actually shipped in the binary (verified by running the binary from a directory that has no `assets/` on disk)
- [ ] No new third-party dependencies (`go list -m all` shows only stdlib)

---

## Out of Scope (deferred to Plan 4)

- Auth / multi-user
- Websocket / SSE push
- Cross-project queries ("show all open tickets across projects")
- Editing from the UI
- Search / filter UI (read-only viewer in this plan)
- Theming / dark mode toggle
- Daemon mode (`ledger-kit view` runs in foreground; Plan 4 may add a systemd/launchd recipe)
