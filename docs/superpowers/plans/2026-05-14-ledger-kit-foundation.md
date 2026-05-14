# ledger-kit Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundational Go binary `ledger-kit` covering MVP phases 1–4: bootstrap, internal libraries, write commands (init/ticket/worklog/goal), and verify. After this plan the binary supports a complete write+validate workflow for a single project; viewer, legacy import, hooks, and release packaging are deferred to Plans 2–4.

**Architecture:** Single Go binary with a subcommand dispatcher in package `cmd`. `main.go` is intentionally thin: it calls `cmd.Dispatch(os.Args[1:], os.Stdout, os.Stderr)`. Internal packages (`internal/...`) hold reusable logic; `cmd/` holds subcommand entry points and the registry map. Stdlib-only. All ledger writes go through `internal/ledger` which enforces the lock protocol and monotonic `n`. Registry writes share the same lock primitive.

**Tech Stack:** Go 1.22+, standard library only (`crypto/rand`, `encoding/json`, `os`, `os/exec`, `flag`, `testing`). No third-party deps.

**Spec reference:** `docs/superpowers/specs/2026-05-14-ledger-kit-go-design.md`

---

## File Structure

```
ledger-kit/                              # project root
  go.mod
  main.go                                   # thin entrypoint; delegates to cmd.Dispatch
  cmd/
    dispatch.go dispatch_test.go
    init.go
    ticket.go
    worklog.go
    goal.go
    verify.go
    list.go
    unregister.go
    registry.go
  internal/
    ids/         ids.go ids_test.go         # project_id generation (128-bit hex)
    agent/       agent.go agent_test.go     # §5.1 priority resolver
    gitutil/     gitutil.go                 # best-effort branch detection (no test — wraps exec)
    locks/       locks.go locks_test.go     # O_EXCL + stale timeout
    jsonio/      jsonio.go jsonio_test.go   # atomic JSON read/write
    ledger/
      types.go                              # Ticket, Worklog, Goal structs + category enum
      jsonl.go jsonl_test.go                # read all rows from JSONL
      append.go append_test.go              # append with lock + monotonic n
    config/      config.go config_test.go   # ledger/config.json
    registry/    registry.go registry_test.go  # ~/.ledger-kit/registry.json
    verify/      verify.go verify_test.go   # fail/warn separation
  templates/
    AGENTS.ledger-kit.md                    # ledger-owned agent instructions (Plan 3/4)
    CLAUDE.ledger-kit.md                    # ledger-owned Claude include body (Plan 3/4)
```

Each file has one responsibility. Tests live next to the file they exercise.

**Conventions used throughout this plan:**
- Module path: `github.com/hgwk/ledger-kit`
- Target directory for the repo: `./`. This directory already contains the spec/plan and Node prototype; do not delete them. Add the Go module files alongside the existing docs.
- Commits use Conventional Commits (`feat:`, `test:`, `chore:`).
- Every `Run:` line shows the exact command. Every test step includes the actual test code; every implementation step includes the actual code.

**Dispatcher correction:** all command registration belongs to package `cmd` via `cmd.Commands`/`cmd.Dispatch`. `main.go` must remain a thin entrypoint after Task 2. If a later task says "wire into `main.go`" or "register into main's dispatcher", interpret that as adding an entry to `cmd.Commands` from a file in package `cmd`.

**Instruction installation correction:** use RTK-style reference mode by default. `instructions install` should create ledger-owned bodies under `ledger/instructions/*.ledger-kit.md` and prepend only a small marker pointer to `AGENTS.md`/`CLAUDE.md`. Claude uses `@ledger/instructions/CLAUDE.ledger-kit.md`; Codex/AGENTS uses `Read and follow: ledger/instructions/AGENTS.ledger-kit.md` unless include support is confirmed. Inline prose injection is compatibility-only.

---

## Phase 0 — Bootstrap

### Task 1: Initialize existing project directory as Go module

**Files:**
- Create: `./go.mod`
- Create: `./.gitignore`
- Create: `./README.md`

- [ ] **Step 1: Enter the existing directory and initialize git if needed**

Run:
```
mkdir -p .
cd .
test -d .git || git init -b main
```

Do not remove existing `docs/`, `install.mjs`, `llm.md`, or `templates/`; they are the Node prototype and design material being replaced incrementally.

- [ ] **Step 2: Create go.mod**

Run from inside the new directory:
```
go mod init github.com/hgwk/ledger-kit
```

Verify the resulting `go.mod` contains:
```
module github.com/hgwk/ledger-kit

go 1.22
```

If the Go version line shows something newer, that is fine — leave it.

- [ ] **Step 3: Create `.gitignore`**

Write or merge into `./.gitignore`:
```
# build artifacts
/ledger-kit
/dist/

# Go
*.test
*.out

# OS
.DS_Store
```

- [ ] **Step 4: Create `README.md` stub**

Write `./README.md` if it does not already exist:
```markdown
# ledger-kit

Append-only project ledger for LLM agents. Multi-project unified view.

See `docs/superpowers/specs/` in the design repo for the full spec.
```

- [ ] **Step 5: Commit**

```
git add go.mod .gitignore README.md
git commit -m "chore: bootstrap go module"
```

---

### Task 2: Subcommand dispatcher skeleton

**Files:**
- Create: `./main.go`
- Create: `./cmd/dispatch.go`
- Create: `./cmd/dispatch_test.go`

- [ ] **Step 1: Write failing test for unknown subcommand error**

Write `cmd/dispatch_test.go`:
```go
package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestDispatch_UnknownSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	code := Dispatch([]string{"nope"}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit, got 0")
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Fatalf("expected 'unknown subcommand' in stderr, got: %q", stderr.String())
	}
}

func TestDispatch_NoArgs_PrintsUsage(t *testing.T) {
	var stderr bytes.Buffer
	code := Dispatch(nil, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit, got 0")
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("expected usage in stderr, got: %q", stderr.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL with `undefined: Dispatch`.

- [ ] **Step 3: Write minimal `cmd/dispatch.go` and `main.go`**

`cmd/dispatch.go`:
```go
package cmd

import (
	"fmt"
	"io"
)

// Handler is a CLI subcommand entry point.
type Handler func(args []string, stdout, stderr io.Writer) int

// Commands maps top-level subcommand names to handlers. Later tasks add
// entries here from files in this same package.
var Commands = map[string]Handler{}

func Dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: ledger-kit <subcommand> [args]")
		return 2
	}
	name := args[0]
	handler, ok := Commands[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown subcommand: %s\n", name)
		return 2
	}
	return handler(args[1:], stdout, stderr)
}
```

`main.go`:
```go
package main

import (
	"os"

	"github.com/hgwk/ledger-kit/cmd"
)

func main() {
	os.Exit(cmd.Dispatch(os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add main.go cmd/dispatch.go cmd/dispatch_test.go
git commit -m "feat: subcommand dispatcher skeleton"
```

---

## Phase 1 — Internal libraries

### Task 3: `internal/ids` — project_id generation

**Files:**
- Create: `internal/ids/ids.go`
- Create: `internal/ids/ids_test.go`

- [ ] **Step 1: Write failing test**

`internal/ids/ids_test.go`:
```go
package ids

import (
	"regexp"
	"testing"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestNewProjectID_Format(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := NewProjectID()
		if !hex32.MatchString(id) {
			t.Fatalf("project_id %q does not match 32-char lowercase hex", id)
		}
	}
}

func TestNewProjectID_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewProjectID()
		if seen[id] {
			t.Fatalf("duplicate id at iteration %d: %s", i, id)
		}
		seen[id] = true
	}
}

func TestDisplay(t *testing.T) {
	got := Display("myapp", "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c")
	want := "myapp-9f8a7c"
	if got != want {
		t.Fatalf("Display = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ids/...`
Expected: FAIL with `undefined: NewProjectID`.

- [ ] **Step 3: Implement `internal/ids/ids.go`**

```go
// Package ids generates and formats project identifiers.
package ids

import (
	"crypto/rand"
	"encoding/hex"
)

// NewProjectID returns a 32-character lowercase hex string backed by
// 128 random bits from crypto/rand. The ledger spec forbids ULID due to
// stdlib-only constraints; sortability is not required here.
func NewProjectID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure on a healthy system is fatal: we have no
		// reasonable fallback. Panic so callers don't proceed with a
		// predictable id.
		panic("ids: crypto/rand failure: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// Display formats the human-visible project handle as "<slug>-<first 6 of id>".
func Display(slug, projectID string) string {
	if len(projectID) < 6 {
		return slug
	}
	return slug + "-" + projectID[:6]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ids/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/ids
git commit -m "feat(ids): project_id generation and display"
```

---

### Task 4: `internal/agent` — agent resolver

**Files:**
- Create: `internal/agent/agent.go`
- Create: `internal/agent/agent_test.go`

Spec reference: §5.1.

- [ ] **Step 1: Write failing test**

`internal/agent/agent_test.go`:
```go
package agent

import (
	"strings"
	"testing"
)

func TestResolve_ExplicitWins(t *testing.T) {
	env := map[string]string{"LEDGER_AGENT": "codex", "USER": "alice"}
	got, warn, err := Resolve("claude-from-json", env)
	if err != nil || warn != "" || got != "claude-from-json" {
		t.Fatalf("got=%q warn=%q err=%v", got, warn, err)
	}
}

func TestResolve_LedgerAgentEnv(t *testing.T) {
	env := map[string]string{"LEDGER_AGENT": "codex", "USER": "alice"}
	got, warn, err := Resolve("", env)
	if err != nil || warn != "" || got != "codex" {
		t.Fatalf("got=%q warn=%q err=%v", got, warn, err)
	}
}

func TestResolve_DetectCodex(t *testing.T) {
	env := map[string]string{"CODEX_SESSION": "1"}
	got, _, err := Resolve("", env)
	if err != nil || got != "codex" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolve_DetectClaude(t *testing.T) {
	env := map[string]string{"CLAUDECODE": "1"}
	got, _, err := Resolve("", env)
	if err != nil || got != "claude" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolve_DetectCursor(t *testing.T) {
	env := map[string]string{"CURSOR_AGENT": "1"}
	got, _, err := Resolve("", env)
	if err != nil || got != "cursor" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolve_FallbackToUserWarns(t *testing.T) {
	env := map[string]string{"USER": "alice"}
	got, warn, err := Resolve("", env)
	if err != nil || got != "alice" || warn == "" {
		t.Fatalf("got=%q warn=%q err=%v", got, warn, err)
	}
	if !strings.Contains(warn, "USER") {
		t.Fatalf("warn should mention USER fallback, got %q", warn)
	}
}

func TestResolve_NothingFails(t *testing.T) {
	env := map[string]string{}
	_, _, err := Resolve("", env)
	if err == nil {
		t.Fatalf("expected error when nothing can resolve agent")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/...`
Expected: FAIL with `undefined: Resolve`.

- [ ] **Step 3: Implement `internal/agent/agent.go`**

```go
// Package agent resolves the writer's identity using the priority defined
// in the spec §5.1: explicit JSON > LEDGER_AGENT > known env detection >
// $USER (with warning) > error.
package agent

import (
	"errors"
	"strings"
)

// ErrUnresolved is returned when no source produced an agent value.
var ErrUnresolved = errors.New("agent: could not resolve (set LEDGER_AGENT or include \"agent\" in input)")

// Resolve returns (agent, warning, error). warning is non-empty when a
// less-preferred source was used and the caller should surface it on stderr.
func Resolve(fromJSON string, env map[string]string) (string, string, error) {
	if fromJSON != "" {
		return fromJSON, "", nil
	}
	if v := env["LEDGER_AGENT"]; v != "" {
		return v, "", nil
	}
	if detected := detect(env); detected != "" {
		return detected, "", nil
	}
	if u := env["USER"]; u != "" {
		return u, "agent resolved from $USER; set LEDGER_AGENT to silence this warning", nil
	}
	return "", "", ErrUnresolved
}

func detect(env map[string]string) string {
	for k := range env {
		switch {
		case strings.HasPrefix(k, "CODEX_"):
			return "codex"
		case k == "CLAUDECODE" || strings.HasPrefix(k, "CLAUDE_CODE_"):
			return "claude"
		case strings.HasPrefix(k, "CURSOR_"):
			return "cursor"
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/agent
git commit -m "feat(agent): resolver with priority chain per spec 5.1"
```

---

### Task 5: `internal/gitutil` — best-effort git introspection

**Files:**
- Create: `internal/gitutil/gitutil.go`

This wraps `git` CLI calls. We don't test it directly (running git in tests is flaky); callers can stub by passing the values explicitly.

- [ ] **Step 1: Write `internal/gitutil/gitutil.go`**

```go
// Package gitutil wraps best-effort calls to the git CLI. All functions
// return empty string + nil error when git is unavailable or the directory
// is not a working tree — callers treat empty as "unknown".
package gitutil

import (
	"os/exec"
	"strings"
)

// CurrentBranch returns the abbreviated symbolic ref of HEAD or "" if
// detached / not a repo / git missing.
func CurrentBranch(dir string) string {
	out, err := run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(out)
	if branch == "HEAD" {
		return ""
	}
	return branch
}

// IsWorkTree returns true if dir is inside a git working tree.
func IsWorkTree(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```
git add internal/gitutil
git commit -m "feat(gitutil): branch and worktree detection"
```

---

### Task 6: `internal/locks` — O_EXCL lock with stale timeout

**Files:**
- Create: `internal/locks/locks.go`
- Create: `internal/locks/locks_test.go`

Spec reference: §3.6.

- [ ] **Step 1: Write failing test**

`internal/locks/locks_test.go`:
```go
package locks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquire_Releases(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, ".lock")

	release, err := Acquire(lp, Options{})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := os.Stat(lp); err != nil {
		t.Fatalf("lock file should exist while held: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(lp); !os.IsNotExist(err) {
		t.Fatalf("lock file should be removed after release, stat err=%v", err)
	}
}

func TestAcquire_FailsWhenHeld(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, ".lock")

	release, err := Acquire(lp, Options{})
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer release()

	// Short retry budget so the test is fast.
	_, err = Acquire(lp, Options{TotalWait: 100 * time.Millisecond, RetryEvery: 25 * time.Millisecond})
	if err == nil {
		t.Fatalf("expected busy error")
	}
}

func TestAcquire_ReclaimsStaleLock(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, ".lock")

	// Plant a stale lock file (mtime far in the past).
	if err := os.WriteFile(lp, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("plant: %v", err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(lp, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	release, err := Acquire(lp, Options{StaleAfter: 30 * time.Second})
	if err != nil {
		t.Fatalf("expected stale reclaim to succeed: %v", err)
	}
	defer release()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/locks/...`
Expected: FAIL with `undefined: Acquire`.

- [ ] **Step 3: Implement `internal/locks/locks.go`**

```go
// Package locks implements the file lock protocol from spec §3.6:
// O_EXCL create with a 30s stale timeout and bounded retry.
package locks

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Options tunes lock behaviour. Zero values use the spec defaults.
type Options struct {
	StaleAfter time.Duration // default 30s
	TotalWait  time.Duration // default 5s
	RetryEvery time.Duration // default 50ms
}

func (o Options) withDefaults() Options {
	if o.StaleAfter == 0 {
		o.StaleAfter = 30 * time.Second
	}
	if o.TotalWait == 0 {
		o.TotalWait = 5 * time.Second
	}
	if o.RetryEvery == 0 {
		o.RetryEvery = 50 * time.Millisecond
	}
	return o
}

// Acquire returns a release function. Caller must invoke release() exactly
// once; using defer is recommended.
func Acquire(path string, opts Options) (release func() error, err error) {
	opts = opts.withDefaults()
	deadline := time.Now().Add(opts.TotalWait)

	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			// Write metadata inside the lock for debuggability.
			host, _ := os.Hostname()
			fmt.Fprintf(f, "pid=%d host=%s ts=%s\n", os.Getpid(), host, time.Now().UTC().Format(time.RFC3339))
			f.Close()
			return func() error { return os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		// Lock present — check stale.
		if info, statErr := os.Stat(path); statErr == nil {
			if time.Since(info.ModTime()) > opts.StaleAfter {
				_ = os.Remove(path)
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("ledger busy: lock at %s held by another process", path)
		}
		time.Sleep(opts.RetryEvery)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/locks/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/locks
git commit -m "feat(locks): O_EXCL lock with stale timeout"
```

---

### Task 7: `internal/jsonio` — atomic JSON read/write

**Files:**
- Create: `internal/jsonio/jsonio.go`
- Create: `internal/jsonio/jsonio_test.go`

- [ ] **Step 1: Write failing test**

`internal/jsonio/jsonio_test.go`:
```go
package jsonio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteThenRead(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.json")
	in := map[string]any{"a": "b", "n": float64(1)}
	if err := WriteJSON(p, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out map[string]any
	if err := ReadJSON(p, &out); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out["a"] != "b" || out["n"] != float64(1) {
		t.Fatalf("roundtrip mismatch: %v", out)
	}
}

func TestWriteJSON_AtomicNoPartial(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.json")
	// First write establishes file.
	if err := WriteJSON(p, map[string]string{"a": "1"}); err != nil {
		t.Fatalf("write1: %v", err)
	}
	// Second write replaces atomically (we can't easily induce a partial
	// write in a unit test, but verify no .tmp file is left behind).
	if err := WriteJSON(p, map[string]string{"a": "2"}); err != nil {
		t.Fatalf("write2: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/jsonio/...`
Expected: FAIL with undefined `WriteJSON`/`ReadJSON`.

- [ ] **Step 3: Implement `internal/jsonio/jsonio.go`**

```go
// Package jsonio centralizes JSON read/write with pretty formatting and
// atomic rename for snapshot files (config.json, goal.json, registry.json).
package jsonio

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ReadJSON unmarshals path into v. Returns os.ErrNotExist when missing.
func ReadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// WriteJSON marshals v with 2-space indent and a trailing newline, writes
// to a sibling temp file, then renames into place.
func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/jsonio/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/jsonio
git commit -m "feat(jsonio): atomic json read/write helpers"
```

---

### Task 8: `internal/ledger/types.go` — Ticket, Worklog, Goal structs

**Files:**
- Create: `internal/ledger/types.go`

Uses spec §3.3, §3.4, §3.5 schemas. We define `Row` as a `map[string]any` underneath to preserve unknown fields on round-trip; the structs are for typed access where needed.

- [ ] **Step 1: Write `internal/ledger/types.go`**

```go
// Package ledger holds the typed data model and JSONL persistence for
// tickets.jsonl, worklog.jsonl, and goal.json.
package ledger

// Row is the canonical wire form: an ordered, unknown-field-preserving
// JSON object. We marshal/unmarshal as map[string]any so that round-trip
// preserves anything the writer included that the current binary doesn't
// know about (forward compatibility).
type Row map[string]any

// Required field sets per spec §3.4 / §3.5.
var (
	TicketRequired = []string{
		"n", "ts", "parent_ticket", "ticket", "agent", "role", "status",
		"task", "scope", "paths", "blocked_by", "branch",
	}
	// "ticket" is intentionally absent — it is optional in worklog rows (§3.5).
	WorklogRequired = []string{
		"n", "ts", "agent", "task", "scope", "result", "paths", "commands",
		"notes", "branch", "commit",
	}
)

// Non-empty semantic string fields. branch/commit must exist where required
// but may be empty when git state is unavailable.
var (
	TicketNonEmpty = []string{
		"parent_ticket", "ticket", "agent", "role", "status", "task", "scope",
	}
	WorklogNonEmpty = []string{
		"agent", "task", "scope", "result",
	}
)

// StatusEnum lists the legal values for ticket.status (§3.4).
var StatusEnum = map[string]struct{}{
	"open":        {},
	"in_progress": {},
	"blocked":     {},
	"done":        {},
	"cancelled":   {},
}

// CategoryEnum lists recommended values for ticket.category. category is
// optional for backward compatibility, but verify warns on missing/unknown
// category for latest active tickets.
var CategoryEnum = map[string]struct{}{
	"feature":  {},
	"bug":      {},
	"docs":     {},
	"ops":      {},
	"design":   {},
	"test":     {},
	"infra":    {},
	"research": {},
	"demo":     {},
	"release":  {},
	"cleanup":  {},
}

// Goal mirrors ledger/goal.json. unknown fields are preserved through Row,
// but Goal exposes the documented shape for command convenience.
type Goal struct {
	SchemaVersion   int      `json:"schema_version"`
	Track           string   `json:"track"`
	Version         string   `json:"version"`
	Updated         string   `json:"updated"`
	SourceOfTruth   string   `json:"source_of_truth"`
	Summary         string   `json:"summary"`
	SuccessCriteria []string `json:"success_criteria"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```
git add internal/ledger/types.go
git commit -m "feat(ledger): row type and required field manifests"
```

---

### Task 9: `internal/ledger/jsonl.go` — read all rows

**Files:**
- Create: `internal/ledger/jsonl.go`
- Create: `internal/ledger/jsonl_test.go`

- [ ] **Step 1: Write failing test**

`internal/ledger/jsonl_test.go`:
```go
package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRows_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows, err := ReadRows(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestReadRows_MissingFile(t *testing.T) {
	dir := t.TempDir()
	rows, err := ReadRows(filepath.Join(dir, "absent.jsonl"))
	if err != nil {
		t.Fatalf("read of missing file should be nil error, got %v", err)
	}
	if rows != nil && len(rows) != 0 {
		t.Fatalf("expected empty rows for missing file")
	}
}

func TestReadRows_MultipleRows(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.jsonl")
	content := `{"n":1,"ticket":"a"}
{"n":2,"ticket":"b"}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows, err := ReadRows(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 2 || rows[0]["ticket"] != "a" || rows[1]["ticket"] != "b" {
		t.Fatalf("unexpected rows: %v", rows)
	}
}

func TestReadRows_SkipsBlankLines(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.jsonl")
	content := "\n{\"n\":1}\n\n{\"n\":2}\n"
	os.WriteFile(p, []byte(content), 0o644)
	rows, err := ReadRows(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestReadRows_ReportsParseErrorLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.jsonl")
	content := `{"n":1}
not json
`
	os.WriteFile(p, []byte(content), 0o644)
	_, err := ReadRows(p)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	// Error message should mention the line number for debuggability.
	if got := err.Error(); !contains(got, "line 2") {
		t.Fatalf("error should mention line 2, got %q", got)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ledger/...`
Expected: FAIL with `undefined: ReadRows`.

- [ ] **Step 3: Implement `internal/ledger/jsonl.go`**

```go
package ledger

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ReadRows reads a JSONL file and returns one Row per non-empty line.
// A missing file is treated as zero rows (not an error). Parse errors
// include the 1-based line number in the message.
func ReadRows(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Tickets/worklog lines can be larger than the default 64KiB buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var rows []Row
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var r Row
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("parse error at line %d: %w", line, err)
		}
		rows = append(rows, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ledger/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/ledger/jsonl.go internal/ledger/jsonl_test.go
git commit -m "feat(ledger): JSONL reader with line-numbered parse errors"
```

---

### Task 10: `internal/ledger/append.go` — append with lock + monotonic n

**Files:**
- Create: `internal/ledger/append.go`
- Create: `internal/ledger/append_test.go`

- [ ] **Step 1: Write failing test**

`internal/ledger/append_test.go`:
```go
package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppend_AssignsN(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")

	r1, err := Append(p, lock, Row{"ticket": "a"})
	if err != nil {
		t.Fatalf("append1: %v", err)
	}
	if r1["n"].(int) != 1 {
		t.Fatalf("expected n=1, got %v", r1["n"])
	}
	r2, err := Append(p, lock, Row{"ticket": "b"})
	if err != nil {
		t.Fatalf("append2: %v", err)
	}
	if r2["n"].(int) != 2 {
		t.Fatalf("expected n=2, got %v", r2["n"])
	}
}

func TestAppend_PersistsToFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")

	Append(p, lock, Row{"ticket": "a"})
	Append(p, lock, Row{"ticket": "b"})

	rows, err := ReadRows(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// n is decoded from JSON as float64 — verify both values present.
	if rows[0]["n"].(float64) != 1 || rows[1]["n"].(float64) != 2 {
		t.Fatalf("n values wrong: %v %v", rows[0]["n"], rows[1]["n"])
	}
}

func TestAppend_OverridesCallerSuppliedN(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")

	r, err := Append(p, lock, Row{"ticket": "a", "n": 99})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if r["n"].(int) != 1 {
		t.Fatalf("Append must assign n authoritatively, got %v", r["n"])
	}
}

func TestAppend_RemovesLockOnSuccess(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")
	Append(p, lock, Row{"ticket": "a"})
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatalf("lock should be gone after Append, stat err=%v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ledger/...`
Expected: FAIL with `undefined: Append`.

- [ ] **Step 3: Implement `internal/ledger/append.go`**

```go
package ledger

import (
	"encoding/json"
	"os"

	"github.com/hgwk/ledger-kit/internal/locks"
)

// Append acquires lockPath, reads jsonlPath to determine the next n,
// writes the normalized row, and releases the lock. The returned Row is
// the normalized form (with n assigned). Caller is responsible for
// supplying ts and other auto-fields before calling Append; Append owns
// only n and the lock.
func Append(jsonlPath, lockPath string, row Row) (Row, error) {
	release, err := locks.Acquire(lockPath, locks.Options{})
	if err != nil {
		return nil, err
	}
	defer release()

	rows, err := ReadRows(jsonlPath)
	if err != nil {
		return nil, err
	}
	next := len(rows) + 1
	row["n"] = next

	data, err := json.Marshal(row)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(jsonlPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return nil, err
	}
	return row, f.Close()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ledger/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/ledger/append.go internal/ledger/append_test.go
git commit -m "feat(ledger): append with lock and monotonic n assignment"
```

---

### Task 11: `internal/config` — load/save ledger/config.json

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test**

`internal/config/config_test.go`:
```go
package config

import (
	"path/filepath"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	in := Config{
		SchemaVersion:    1,
		ProjectID:        "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c",
		Slug:             "myapp",
		Name:             "My App",
		Parents:          []string{"ROOT", "BUG"},
		BranchConvention: "work/{ticket}",
		LogGoalChanges:   false,
	}
	if err := Save(p, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ProjectID != in.ProjectID || got.Slug != in.Slug || len(got.Parents) != 2 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestDefault(t *testing.T) {
	c := Default("myapp", "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c", "")
	if c.SchemaVersion != 1 {
		t.Fatalf("schema_version should default to 1, got %d", c.SchemaVersion)
	}
	if c.Slug != "myapp" || c.Name != "myapp" {
		t.Fatalf("name should default to slug, got slug=%s name=%s", c.Slug, c.Name)
	}
	if c.BranchConvention != "work/{ticket}" {
		t.Fatalf("unexpected branch convention: %s", c.BranchConvention)
	}
	if len(c.Parents) == 0 {
		t.Fatalf("default parents should be non-empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/config/config.go`**

```go
// Package config handles ledger/config.json: the per-repo operational
// metadata. Spec §3.2.
package config

import "github.com/hgwk/ledger-kit/internal/jsonio"

type Config struct {
	SchemaVersion    int      `json:"schema_version"`
	ProjectID        string   `json:"project_id"`
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Parents          []string `json:"parents"`
	BranchConvention string   `json:"branch_convention"`
	LogGoalChanges   bool     `json:"log_goal_changes"`
}

// DefaultParents is the seed parent set per spec §3.2.
var DefaultParents = []string{"ROOT", "DOC", "FE", "BE", "OPS", "DEMO", "BUG", "LEGACY"}

// Default builds a fresh Config for a new project. name falls back to slug
// when empty.
func Default(slug, projectID, name string) Config {
	if name == "" {
		name = slug
	}
	return Config{
		SchemaVersion:    1,
		ProjectID:        projectID,
		Slug:             slug,
		Name:             name,
		Parents:          append([]string(nil), DefaultParents...),
		BranchConvention: "work/{ticket}",
		LogGoalChanges:   false,
	}
}

func Load(path string) (Config, error) {
	var c Config
	err := jsonio.ReadJSON(path, &c)
	return c, err
}

func Save(path string, c Config) error {
	return jsonio.WriteJSON(path, c)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/config
git commit -m "feat(config): ledger/config.json load/save with defaults"
```

---

### Task 12: `internal/registry` — registry load/save with lock + merge

**Files:**
- Create: `internal/registry/registry.go`
- Create: `internal/registry/registry_test.go`

Spec reference: §7.1.

- [ ] **Step 1: Write failing test**

`internal/registry/registry_test.go`:
```go
package registry

import (
	"path/filepath"
	"testing"
)

func TestRegister_NewProject(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	p := Project{
		ProjectID: "id1", Slug: "a", Name: "A",
		Paths:     []string{"/p/a"},
	}
	if err := store.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}

	reg, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(reg.Projects) != 1 || reg.Projects[0].ProjectID != "id1" {
		t.Fatalf("unexpected reg: %+v", reg)
	}
}

func TestRegister_AppendsPathToExistingProject(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Slug: "a", Paths: []string{"/p/a"}})
	store.Register(Project{ProjectID: "id1", Slug: "a", Paths: []string{"/p/a2"}})

	reg, _ := store.Load()
	if len(reg.Projects) != 1 {
		t.Fatalf("should still be 1 project, got %d", len(reg.Projects))
	}
	if len(reg.Projects[0].Paths) != 2 {
		t.Fatalf("expected 2 paths, got %v", reg.Projects[0].Paths)
	}
}

func TestRegister_DedupesPaths(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a"}})
	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a"}})

	reg, _ := store.Load()
	if len(reg.Projects[0].Paths) != 1 {
		t.Fatalf("duplicate path should be deduped: %v", reg.Projects[0].Paths)
	}
}

func TestUnregisterPath(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a", "/p/b"}})
	if err := store.UnregisterPath("/p/a"); err != nil {
		t.Fatalf("unregister path: %v", err)
	}
	reg, _ := store.Load()
	if len(reg.Projects) != 1 || len(reg.Projects[0].Paths) != 1 || reg.Projects[0].Paths[0] != "/p/b" {
		t.Fatalf("unexpected state: %+v", reg)
	}
}

func TestUnregisterPath_LastPathRemovesProject(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a"}})
	store.UnregisterPath("/p/a")

	reg, _ := store.Load()
	if len(reg.Projects) != 0 {
		t.Fatalf("project should be removed when last path goes: %+v", reg)
	}
}

func TestUnregisterByID(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a", "/p/b"}})
	store.UnregisterID("id1")

	reg, _ := store.Load()
	if len(reg.Projects) != 0 {
		t.Fatalf("project should be removed by id: %+v", reg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/registry/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/registry/registry.go`**

```go
// Package registry manages ~/.ledger-kit/registry.json. Spec §7.1.
package registry

import (
	"errors"
	"os"
	"time"

	"github.com/hgwk/ledger-kit/internal/jsonio"
	"github.com/hgwk/ledger-kit/internal/locks"
)

type Project struct {
	ProjectID    string   `json:"project_id"`
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Paths        []string `json:"paths"`
	RegisteredAt string   `json:"registered_at"`
	LastSeen     string   `json:"last_seen"`
}

type Registry struct {
	Version  int       `json:"version"`
	Projects []Project `json:"projects"`
}

type Store struct {
	path string
	lock string
}

func New(path, lockPath string) *Store {
	return &Store{path: path, lock: lockPath}
}

func (s *Store) Load() (Registry, error) {
	var r Registry
	err := jsonio.ReadJSON(s.path, &r)
	if errors.Is(err, os.ErrNotExist) {
		return Registry{Version: 1}, nil
	}
	if err != nil {
		return r, err
	}
	if r.Version == 0 {
		r.Version = 1
	}
	return r, nil
}

func (s *Store) save(r Registry) error {
	return jsonio.WriteJSON(s.path, r)
}

func (s *Store) Register(p Project) error {
	release, err := locks.Acquire(s.lock, locks.Options{})
	if err != nil {
		return err
	}
	defer release()

	r, err := s.Load()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	idx := indexByID(r.Projects, p.ProjectID)
	if idx == -1 {
		if p.RegisteredAt == "" {
			p.RegisteredAt = now
		}
		p.LastSeen = now
		p.Paths = dedupe(p.Paths)
		r.Projects = append(r.Projects, p)
	} else {
		existing := &r.Projects[idx]
		existing.Slug = nonEmpty(p.Slug, existing.Slug)
		existing.Name = nonEmpty(p.Name, existing.Name)
		existing.Paths = dedupe(append(existing.Paths, p.Paths...))
		existing.LastSeen = now
	}
	r.Version = 1
	return s.save(r)
}

func (s *Store) UnregisterPath(path string) error {
	release, err := locks.Acquire(s.lock, locks.Options{})
	if err != nil {
		return err
	}
	defer release()

	r, err := s.Load()
	if err != nil {
		return err
	}
	out := r.Projects[:0]
	for _, p := range r.Projects {
		kept := kept(p.Paths, path)
		if len(kept) == 0 {
			continue
		}
		p.Paths = kept
		out = append(out, p)
	}
	r.Projects = out
	return s.save(r)
}

func (s *Store) UnregisterID(id string) error {
	release, err := locks.Acquire(s.lock, locks.Options{})
	if err != nil {
		return err
	}
	defer release()

	r, err := s.Load()
	if err != nil {
		return err
	}
	out := r.Projects[:0]
	for _, p := range r.Projects {
		if p.ProjectID == id {
			continue
		}
		out = append(out, p)
	}
	r.Projects = out
	return s.save(r)
}

func indexByID(ps []Project, id string) int {
	for i, p := range ps {
		if p.ProjectID == id {
			return i
		}
	}
	return -1
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func kept(in []string, drop string) []string {
	out := in[:0]
	for _, v := range in {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/registry/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/registry
git commit -m "feat(registry): load/save with merge-by-project_id"
```

---

## Phase 2 — `init` command

### Task 13: cmd/init wiring + path resolution

**Files:**
- Create: `cmd/init.go`
- Create: `cmd/init_test.go`
- Modify: `main.go` (register the subcommand)

We split init's logic into a testable function `RunInit(targetDir, opts, registryStore) error` so tests don't need to touch `$HOME`.

- [ ] **Step 1: Write failing test**

`cmd/init_test.go`:
```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hgwk/ledger-kit/internal/registry"
)

func TestRunInit_CreatesFiles(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))

	err := RunInit(target, InitOpts{Slug: "myapp"}, store)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	mustExist := []string{
		"ledger/config.json",
		"ledger/goal.json",
		"ledger/tickets.jsonl",
		"ledger/worklog.jsonl",
		"ledger/instructions/AGENTS.ledger-kit.md",
		"ledger/instructions/CLAUDE.ledger-kit.md",
	}
	for _, p := range mustExist {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}
}

func TestRunInit_RegistersProject(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))

	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	r, _ := store.Load()
	if len(r.Projects) != 1 {
		t.Fatalf("expected 1 project registered, got %d", len(r.Projects))
	}
	if r.Projects[0].Slug != "myapp" {
		t.Fatalf("slug mismatch: %s", r.Projects[0].Slug)
	}
	if r.Projects[0].Paths[0] != target {
		t.Fatalf("path mismatch: %v", r.Projects[0].Paths)
	}
}

func TestRunInit_IsIdempotent(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))

	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Capture project_id so we can confirm second init preserves it.
	r1, _ := store.Load()
	id := r1.Projects[0].ProjectID

	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("second: %v", err)
	}
	r2, _ := store.Load()
	if len(r2.Projects) != 1 {
		t.Fatalf("second init should not create a new project entry, got %d", len(r2.Projects))
	}
	if r2.Projects[0].ProjectID != id {
		t.Fatalf("project_id should be preserved across re-init: %s vs %s", id, r2.Projects[0].ProjectID)
	}
}

func TestRunInit_UpdatesGitignore(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))

	RunInit(target, InitOpts{Slug: "myapp"}, store)

	data, err := os.ReadFile(filepath.Join(target, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, needle := range []string{"ledger/.lock", "ledger/.backup/", "ledger/import-errors.jsonl"} {
		if !contains(string(data), needle) {
			t.Fatalf(".gitignore missing %q; got:\n%s", needle, data)
		}
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && idx(s, sub) >= 0 }
func idx(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL with `undefined: RunInit`.

- [ ] **Step 3: Implement `cmd/init.go`**

```go
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hgwk/ledger-kit/internal/config"
	"github.com/hgwk/ledger-kit/internal/ids"
	"github.com/hgwk/ledger-kit/internal/jsonio"
	"github.com/hgwk/ledger-kit/internal/ledger"
	"github.com/hgwk/ledger-kit/internal/registry"
)

type InitOpts struct {
	Slug string
	Name string
}

// RunInit creates ledger/* in targetDir and registers it. Re-running on an
// already-initialized directory is a no-op for the data files and re-adds
// the path in the registry idempotently.
func RunInit(targetDir string, opts InitOpts, store *registry.Store) error {
	slug := opts.Slug
	if slug == "" {
		slug = filepath.Base(targetDir)
	}

	ledgerDir := filepath.Join(targetDir, "ledger")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		return err
	}

	configPath := filepath.Join(ledgerDir, "config.json")
	var cfg config.Config
	if existing, err := config.Load(configPath); err == nil && existing.ProjectID != "" {
		cfg = existing
	} else if errors.Is(err, os.ErrNotExist) || existing.ProjectID == "" {
		cfg = config.Default(slug, ids.NewProjectID(), opts.Name)
		if err := config.Save(configPath, cfg); err != nil {
			return err
		}
	} else {
		return err
	}

	if err := ensureEmpty(filepath.Join(ledgerDir, "tickets.jsonl")); err != nil {
		return err
	}
	if err := ensureEmpty(filepath.Join(ledgerDir, "worklog.jsonl")); err != nil {
		return err
	}
	if err := ensureGoal(filepath.Join(ledgerDir, "goal.json")); err != nil {
		return err
	}
	if err := ensureInstructions(filepath.Join(ledgerDir, "instructions")); err != nil {
		return err
	}
	if err := ensureGitignore(filepath.Join(targetDir, ".gitignore")); err != nil {
		return err
	}

	return store.Register(registry.Project{
		ProjectID: cfg.ProjectID,
		Slug:      cfg.Slug,
		Name:      cfg.Name,
		Paths:     []string{targetDir},
	})
}

// runInitCLI is the subcommand entry registered into main's dispatcher.
func runInitCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("init")
	target := fs.String("target", "", "target directory (defaults to cwd)")
	slug := fs.String("slug", "", "project slug (defaults to dir name)")
	name := fs.String("name", "", "project display name (defaults to slug)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := *target
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	store, err := DefaultRegistry()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := RunInit(abs, InitOpts{Slug: *slug, Name: *name}, store); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "initialized %s\n", abs)
	return 0
}

func ensureEmpty(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}

func ensureGoal(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	g := ledger.Goal{
		SchemaVersion:   1,
		Track:           "project",
		Version:         "0.1.0",
		Updated:         time.Now().UTC().Format(time.RFC3339),
		SourceOfTruth:   "README.md",
		Summary:         "Fill this goal snapshot with the current project objective.",
		SuccessCriteria: []string{},
	}
	return jsonio.WriteJSON(path, g)
}

func ensureInstructions(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		"AGENTS.ledger-kit.md": agentInstructionBody(),
		"CLAUDE.ledger-kit.md": claudeInstructionBody(),
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func agentInstructionBody() string {
	return `# Ledger Kit Instructions

Use ledger-kit commands rather than editing ledger files directly.

- Ticket events: ledger-kit ticket add/event --json @-
- Worklogs: ledger-kit worklog add --json @-
- Goal: ledger-kit goal set/show
- Verify before handoff: ledger-kit verify

Rules:
- append-only: never edit or delete existing rows
- parent_ticket is hierarchy; blocked_by is dependency
- include category on new ticket rows when possible
- claim/handoff via ticket event before overlapping edits
`
}

func claudeInstructionBody() string {
	return agentInstructionBody()
}

func ensureGitignore(path string) error {
	required := []string{
		"ledger/.lock",
		"ledger/.backup/",
		"ledger/import-errors.jsonl",
		"ledger/legacy/",
	}
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	out := existing
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	for _, line := range required {
		if !lineContains(existing, line) {
			out += line + "\n"
		}
	}
	if out == existing {
		return nil
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

func lineContains(haystack, needle string) bool {
	for _, line := range strings.Split(haystack, "\n") {
		if strings.TrimSpace(line) == needle {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Create `cmd/registry_path.go` and `cmd/flagset.go` (shared helpers)**

`cmd/registry_path.go`:
```go
package cmd

import (
	"os"
	"path/filepath"

	"github.com/hgwk/ledger-kit/internal/registry"
)

// DefaultRegistry returns a Store rooted at $HOME/.ledger-kit/registry.json.
// LEDGER_KIT_HOME overrides the directory (used by tests).
func DefaultRegistry() (*registry.Store, error) {
	home := os.Getenv("LEDGER_KIT_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		home = filepath.Join(h, ".ledger-kit")
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, err
	}
	return registry.New(
		filepath.Join(home, "registry.json"),
		filepath.Join(home, "registry.lock"),
	), nil
}
```

`cmd/flagset.go`:
```go
package cmd

import "flag"

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	return fs
}
```

- [ ] **Step 5: Register the subcommand in `cmd.Commands`**

In `cmd/init.go`, export the entry point and register it from package `cmd`:

```go
func init() {
	Commands["init"] = RunInitCLI
}

// RunInitCLI is the subcommand entry registered by package cmd.
func RunInitCLI(args []string, stdout, stderr io.Writer) int {
	return runInitCLI(args, stdout, stderr)
}
```

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```
git add cmd
git commit -m "feat(init): create ledger seed and register project"
```

---

## Phase 3 — Write commands

### Task 14: `cmd/ticket add`

**Files:**
- Create: `cmd/ticket.go`
- Create: `cmd/ticket_test.go`
- Create: `cmd/jsoninput.go` (shared `--json` parsing)
- Modify: `main.go`

- [ ] **Step 1: Implement shared JSON input helper**

`cmd/jsoninput.go`:
```go
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadJSONInput resolves --json values: '<inline>', '@-' (stdin), '@<path>'.
func ReadJSONInput(spec string, stdin io.Reader) (map[string]any, error) {
	if spec == "" {
		return nil, errors.New("--json is required")
	}
	var data []byte
	switch {
	case spec == "@-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, err
		}
		data = b
	case strings.HasPrefix(spec, "@"):
		b, err := os.ReadFile(spec[1:])
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", spec[1:], err)
		}
		data = b
	default:
		data = []byte(spec)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse --json: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 2: Write failing test for `ticket add`**

`cmd/ticket_test.go`:
```go
package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ledger-kit/internal/ledger"
	"github.com/hgwk/ledger-kit/internal/registry"
)

func mustInit(t *testing.T) (target string, store *registry.Store) {
	t.Helper()
	target = t.TempDir()
	regDir := t.TempDir()
	store = registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))
	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("init: %v", err)
	}
	return
}

func TestTicketAdd_AppendsRow(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	in := map[string]any{
		"ticket":        "demo-1",
		"parent_ticket": "ROOT",
		"role":          "impl",
		"status":        "open",
		"task":          "Demo task",
		"scope":         "repo",
		"paths":         []any{"src/x.go"},
		"blocked_by":    []any{},
	}
	body, _ := json.Marshal(in)

	var out, errb bytes.Buffer
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, errb.String())
	}

	rows, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r["ticket"] != "demo-1" || r["agent"] != "codex" || r["status"] != "open" {
		t.Fatalf("row content wrong: %+v", r)
	}
	if r["n"].(float64) != 1 {
		t.Fatalf("expected n=1, got %v", r["n"])
	}
	if _, ok := r["ts"]; !ok {
		t.Fatalf("ts should be auto-filled")
	}
}

func TestTicketAdd_RejectsDuplicateID(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	in := map[string]any{
		"ticket": "demo-1", "parent_ticket": "ROOT", "role": "impl",
		"status": "open", "task": "t", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(in)

	out := &bytes.Buffer{}
	errb := &bytes.Buffer{}
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), out, errb); code != 0 {
		t.Fatalf("first add failed: %s", errb.String())
	}
	out.Reset()
	errb.Reset()
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), out, errb)
	if code == 0 {
		t.Fatalf("expected failure on duplicate ticket")
	}
	if !strings.Contains(errb.String(), "already exists") {
		t.Fatalf("stderr should explain duplicate, got: %s", errb.String())
	}
}

func TestTicketAdd_MissingRequiredFails(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{"ticket": "demo-1"} // missing most required fields
	body, _ := json.Marshal(in)
	errb := &bytes.Buffer{}
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, errb)
	if code == 0 {
		t.Fatalf("expected failure due to missing required fields")
	}
	if !strings.Contains(errb.String(), "missing required") {
		t.Fatalf("stderr should mention missing required: %s", errb.String())
	}
}

func TestTicketAdd_FromFile(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{
		"ticket": "demo-2", "parent_ticket": "ROOT", "role": "impl",
		"status": "open", "task": "t", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(in)
	tmp := filepath.Join(t.TempDir(), "in.json")
	os.WriteFile(tmp, body, 0o644)

	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@" + tmp}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("from file failed")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL with `undefined: RunTicketCLI`.

- [ ] **Step 4: Implement `cmd/ticket.go`**

```go
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hgwk/ledger-kit/internal/agent"
	"github.com/hgwk/ledger-kit/internal/gitutil"
	"github.com/hgwk/ledger-kit/internal/ledger"
)

// RunTicketCLI is the entry for `ledger-kit ticket ...`.
func RunTicketCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ledger-kit ticket <add|event> [flags]")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return runTicketAdd(rest, stdin, stdout, stderr)
	case "event":
		return runTicketEvent(rest, stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown ticket subcommand: %s\n", sub)
		return 2
	}
}

func runTicketAdd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("ticket add")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	row, err := normalizeTicketAdd(dir, input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	path := filepath.Join(dir, "ledger", "tickets.jsonl")
	lock := filepath.Join(dir, "ledger", ".lock")
	out, err := ledger.Append(path, lock, ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}

func normalizeTicketAdd(dir string, input map[string]any) (map[string]any, error) {
	ticket, _ := input["ticket"].(string)
	if ticket == "" {
		return nil, errors.New("ticket: field 'ticket' is required")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r["ticket"] == ticket {
			return nil, fmt.Errorf("ticket %q already exists (use `ticket event` to update)", ticket)
		}
	}

	resolved, err := autoFields(dir, input)
	if err != nil {
		return nil, err
	}
	if err := requireFields(resolved, ledger.TicketRequired, "ticket"); err != nil {
		return nil, err
	}
	return resolved, nil
}

func runTicketEvent(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("ticket event")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	row, err := normalizeTicketEvent(dir, input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	out, err := ledger.Append(filepath.Join(dir, "ledger", "tickets.jsonl"), filepath.Join(dir, "ledger", ".lock"), ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}

func normalizeTicketEvent(dir string, input map[string]any) (map[string]any, error) {
	ticket, _ := input["ticket"].(string)
	if ticket == "" {
		return nil, errors.New("ticket event: field 'ticket' is required")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	// Latest row for this ticket = base.
	var base map[string]any
	for _, r := range rows {
		if r["ticket"] == ticket {
			base = map[string]any(r)
		}
	}
	if base == nil {
		return nil, fmt.Errorf("ticket %q does not exist (use `ticket add` first)", ticket)
	}
	// Shallow overlay: input fields replace base fields wholesale.
	for k, v := range input {
		base[k] = v
	}
	// n and ts must be re-derived; clear so autoFields/Append set them.
	delete(base, "n")
	base["ts"] = ""

	resolved, err := autoFields(dir, base)
	if err != nil {
		return nil, err
	}
	if err := requireFields(resolved, ledger.TicketRequired, "ticket"); err != nil {
		return nil, err
	}
	return resolved, nil
}

// autoFields fills agent, ts, branch when the caller did not supply them.
// n is assigned later by ledger.Append.
func autoFields(dir string, in map[string]any) (map[string]any, error) {
	if _, ok := in["ts"]; !ok || in["ts"] == "" {
		in["ts"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}
	if _, ok := in["branch"]; !ok {
		in["branch"] = gitutil.CurrentBranch(dir)
	}
	envMap := envAsMap()
	fromJSON, _ := in["agent"].(string)
	resolved, warn, err := agent.Resolve(fromJSON, envMap)
	if err != nil {
		return nil, err
	}
	in["agent"] = resolved
	if warn != "" {
		fmt.Fprintln(os.Stderr, "warning:", warn)
	}
	return in, nil
}

func envAsMap() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				out[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return out
}

func requireFields(row map[string]any, required []string, kind string) error {
	for _, f := range required {
		v, ok := row[f]
		if !ok || isEmpty(v) {
			return fmt.Errorf("%s: missing required field %q", kind, f)
		}
	}
	return nil
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	}
	return false
}

func resolveTarget(target string) string {
	if target == "" {
		wd, _ := os.Getwd()
		return wd
	}
	abs, _ := filepath.Abs(target)
	return abs
}

func encErr(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
```

- [ ] **Step 5: Wire `ticket` into `main.go`**

In `main.go` `subcommands` map, add:
```go
"ticket": func(args []string, stdout, stderr io.Writer) int {
	return cmd.RunTicketCLI(args, os.Stdin, stdout, stderr)
},
```

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```
git add cmd main.go
git commit -m "feat(ticket): add and event subcommands with carry-forward"
```

---

### Task 15: `cmd/worklog add`

**Files:**
- Create: `cmd/worklog.go`
- Create: `cmd/worklog_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing test**

`cmd/worklog_test.go`:
```go
package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hgwk/ledger-kit/internal/ledger"
)

func TestWorklogAdd_AppendsRow(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	in := map[string]any{
		"ticket":   "demo-1",
		"task":     "demo work",
		"scope":    "repo",
		"result":   "done",
		"paths":    []any{},
		"commands": []any{"go test"},
	}
	body, _ := json.Marshal(in)
	errb := &bytes.Buffer{}
	if code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, errb); code != 0 {
		t.Fatalf("add failed: %s", errb.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestWorklogAdd_TicketOptional(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	in := map[string]any{
		"task":     "goal change",
		"scope":    "ledger",
		"result":   "Updated goal.",
		"paths":    []any{"ledger/goal.json"},
		"commands": []any{},
	}
	body, _ := json.Marshal(in)
	errb := &bytes.Buffer{}
	if code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, errb); code != 0 {
		t.Fatalf("add without ticket should be accepted: %s", errb.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL with `undefined: RunWorklogCLI`.

- [ ] **Step 3: Implement `cmd/worklog.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/hgwk/ledger-kit/internal/ledger"
)

func RunWorklogCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "add" {
		fmt.Fprintln(stderr, "usage: ledger-kit worklog add --json @-")
		return 2
	}
	fs := newFlagSet("worklog add")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	row, err := autoFields(dir, input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if _, hasCommit := row["commit"]; !hasCommit {
		row["commit"] = ""
	}
	if err := requireFields(row, ledger.WorklogRequired, "worklog"); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	out, err := ledger.Append(filepath.Join(dir, "ledger", "worklog.jsonl"), filepath.Join(dir, "ledger", ".lock"), ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}
```

- [ ] **Step 4: Wire into `main.go`**

```go
"worklog": func(args []string, stdout, stderr io.Writer) int {
	return cmd.RunWorklogCLI(args, os.Stdin, stdout, stderr)
},
```

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```
git add cmd main.go
git commit -m "feat(worklog): add subcommand"
```

---

### Task 16: `cmd/goal set` and `cmd/goal show`

**Files:**
- Create: `cmd/goal.go`
- Create: `cmd/goal_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing test**

`cmd/goal_test.go`:
```go
package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ledger-kit/internal/jsonio"
	"github.com/hgwk/ledger-kit/internal/ledger"
)

func TestGoalSet_OverwritesGoalJSON(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{"summary": "new goal", "success_criteria": []any{"a"}}
	body, _ := json.Marshal(in)
	if code := RunGoalCLI([]string{"set", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("set failed")
	}
	var g ledger.Goal
	if err := jsonio.ReadJSON(filepath.Join(target, "ledger", "goal.json"), &g); err != nil {
		t.Fatalf("read goal: %v", err)
	}
	if g.Summary != "new goal" || len(g.SuccessCriteria) != 1 || g.SuccessCriteria[0] != "a" {
		t.Fatalf("goal not applied: %+v", g)
	}
}

func TestGoalSet_LogFlagWritesWorklog(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{"summary": "v2"}
	body, _ := json.Marshal(in)
	if code := RunGoalCLI([]string{"set", "--target", target, "--json", "@-", "--log"}, bytes.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("set --log failed")
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if len(rows) != 1 {
		t.Fatalf("expected one worklog row, got %d", len(rows))
	}
	if rows[0]["task"] != "goal set" {
		t.Fatalf("worklog task should be 'goal set', got %v", rows[0]["task"])
	}
}

func TestGoalShow_PrintsGoal(t *testing.T) {
	target, _ := mustInit(t)
	out := &bytes.Buffer{}
	if code := RunGoalCLI([]string{"show", "--target", target}, &bytes.Buffer{}, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("show failed")
	}
	if !strings.Contains(out.String(), "schema_version") {
		t.Fatalf("show output missing schema_version, got: %s", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL with `undefined: RunGoalCLI`.

- [ ] **Step 3: Implement `cmd/goal.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hgwk/ledger-kit/internal/config"
	"github.com/hgwk/ledger-kit/internal/jsonio"
	"github.com/hgwk/ledger-kit/internal/ledger"
)

func RunGoalCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ledger-kit goal <set|show>")
		return 2
	}
	switch args[0] {
	case "set":
		return runGoalSet(args[1:], stdin, stdout, stderr)
	case "show":
		return runGoalShow(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown goal subcommand: %s\n", args[0])
		return 2
	}
}

func runGoalSet(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("goal set")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	logFlag := fs.Bool("log", false, "also append a worklog row recording the change")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	goalPath := filepath.Join(dir, "ledger", "goal.json")
	var existing ledger.Goal
	_ = jsonio.ReadJSON(goalPath, &existing)
	merged := mergeGoal(existing, input)
	merged.Updated = time.Now().UTC().Format(time.RFC3339)
	if merged.SchemaVersion == 0 {
		merged.SchemaVersion = 1
	}
	if err := jsonio.WriteJSON(goalPath, merged); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	cfg, _ := config.Load(filepath.Join(dir, "ledger", "config.json"))
	shouldLog := *logFlag || cfg.LogGoalChanges
	if shouldLog {
		row := map[string]any{
			"task":     "goal set",
			"scope":    "ledger",
			"result":   "Updated project goal snapshot.",
			"paths":    []any{"ledger/goal.json"},
			"commands": []any{"ledger-kit goal set"},
		}
		row, err = autoFields(dir, row)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := requireFields(row, ledger.WorklogRequired, "worklog"); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		row["commit"] = ""
		if _, err := ledger.Append(filepath.Join(dir, "ledger", "worklog.jsonl"), filepath.Join(dir, "ledger", ".lock"), ledger.Row(row)); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(merged), stderr)
}

func runGoalShow(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("goal show")
	target := fs.String("target", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)
	data, err := os.ReadFile(filepath.Join(dir, "ledger", "goal.json"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	stdout.Write(data)
	return 0
}

func mergeGoal(existing ledger.Goal, input map[string]any) ledger.Goal {
	out := existing
	if v, ok := input["schema_version"].(float64); ok {
		out.SchemaVersion = int(v)
	}
	if v, ok := input["track"].(string); ok {
		out.Track = v
	}
	if v, ok := input["version"].(string); ok {
		out.Version = v
	}
	if v, ok := input["source_of_truth"].(string); ok {
		out.SourceOfTruth = v
	}
	if v, ok := input["summary"].(string); ok {
		out.Summary = v
	}
	if v, ok := input["success_criteria"].([]any); ok {
		out.SuccessCriteria = out.SuccessCriteria[:0]
		for _, s := range v {
			if str, ok := s.(string); ok {
				out.SuccessCriteria = append(out.SuccessCriteria, str)
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Wire into `main.go`**

```go
"goal": func(args []string, stdout, stderr io.Writer) int {
	return cmd.RunGoalCLI(args, os.Stdin, stdout, stderr)
},
```

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```
git add cmd main.go
git commit -m "feat(goal): set (with --log) and show"
```

---

## Phase 4 — `verify`

### Task 17: `internal/verify` — Fail/Warn separation

**Files:**
- Create: `internal/verify/verify.go`
- Create: `internal/verify/verify_test.go`

Spec reference: §6.

- [ ] **Step 1: Write failing test**

`internal/verify/verify_test.go`:
```go
package verify

import (
	"path/filepath"
	"testing"

	"github.com/hgwk/ledger-kit/internal/config"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := ensureParent(p); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := writeFile(p, content); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return dir
}

func TestVerify_EmptyLedgerPasses(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":  validConfigJSON(),
		"ledger/goal.json":    validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": "",
	})
	report, err := Run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(report.Fails) != 0 {
		t.Fatalf("expected no fails, got %v", report.Fails)
	}
}

func TestVerify_NGapFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":3,"ts":"2026-05-14T10:01:00Z","ticket":"b","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected n gap fail")
	}
}

func TestVerify_NonDecreasingTsFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:01:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:00:00Z","ticket":"b","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected ts non-decreasing fail")
	}
}

func TestVerify_BadStatusFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"weird","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected status enum fail")
	}
}

func TestVerify_GhostTicketFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected ghost ticket/task fail")
	}
}

func TestVerify_MissingCategoryWarnsOnActiveTicket(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Warns) == 0 {
		t.Fatalf("expected missing category warning")
	}
}

func TestVerify_StaleBlockerWarns(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"done-ticket","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"done","task":"done","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:01:00Z","ticket":"blocked-ticket","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"blocked","task":"blocked","scope":"repo","paths":[],"blocked_by":["done-ticket"],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Warns) == 0 {
		t.Fatalf("expected stale blocker warning")
	}
}

func TestVerify_OrphanWorklogIsWarn(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSON(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"ghost","agent":"codex","task":"t","scope":"repo","result":"r","paths":[],"commands":[],"branch":"","commit":""}
`,
	})
	report, _ := Run(dir)
	if len(report.Fails) != 0 {
		t.Fatalf("orphan worklog should not fail by default: %v", report.Fails)
	}
	if len(report.Warns) == 0 {
		t.Fatalf("expected orphan warn")
	}
}

func TestVerify_StrictPromotesWarns(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSON(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"ghost","agent":"codex","task":"t","scope":"repo","result":"r","paths":[],"commands":[],"branch":"","commit":""}
`,
	})
	report, _ := RunStrict(dir, true)
	if len(report.Fails) == 0 {
		t.Fatalf("strict mode should fail on warn")
	}
}

func TestVerify_BadConfigFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": `{"schema_version":1}`, // missing required fields
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected config schema fail")
	}
}

// --- fixtures ---

func validConfigJSON() string {
	c := config.Default("myapp", "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c", "")
	return mustJSON(c)
}

func validGoalJSON() string {
	return `{"schema_version":1,"track":"project","version":"0.1.0","updated":"2026-05-14T00:00:00Z","source_of_truth":"README.md","summary":"x","success_criteria":[]}`
}
```

- [ ] **Step 2: Write the small test helpers used above**

Append to `internal/verify/verify_test.go`:
```go
import (
	"encoding/json"
	"os"
)

func ensureParent(p string) error {
	return os.MkdirAll(filepath.Dir(p), 0o755)
}
func writeFile(p, c string) error { return os.WriteFile(p, []byte(c), 0o644) }
func mustJSON(v any) string       { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }
```

(Combine all imports into a single block — keep idiomatic Go.)

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/verify/...`
Expected: FAIL with `undefined: Run`.

- [ ] **Step 4: Implement `internal/verify/verify.go`**

```go
// Package verify implements ledger validation per spec §6.
// Fail = blocks commits; Warn = surfaced but exit 0 unless --strict.
package verify

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/hgwk/ledger-kit/internal/config"
	"github.com/hgwk/ledger-kit/internal/jsonio"
	"github.com/hgwk/ledger-kit/internal/ledger"
)

type Issue struct {
	File    string
	Line    int    // 0 when whole-file or n/a
	Message string
}

type Report struct {
	Fails []Issue
	Warns []Issue
}

func Run(targetDir string) (Report, error) {
	return runWith(targetDir, false)
}

func RunStrict(targetDir string, strict bool) (Report, error) {
	return runWith(targetDir, strict)
}

func runWith(targetDir string, strict bool) (Report, error) {
	var rep Report

	cfgPath := filepath.Join(targetDir, "ledger", "config.json")
	var cfg config.Config
	if err := jsonio.ReadJSON(cfgPath, &cfg); err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/config.json", Message: "cannot read: " + err.Error()})
	} else {
		if cfg.SchemaVersion == 0 || cfg.ProjectID == "" || cfg.Slug == "" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/config.json", Message: "missing required fields (schema_version/project_id/slug)"})
		}
	}

	goalPath := filepath.Join(targetDir, "ledger", "goal.json")
	var g ledger.Goal
	if err := jsonio.ReadJSON(goalPath, &g); err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/goal.json", Message: "cannot read: " + err.Error()})
	} else if g.SchemaVersion == 0 {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/goal.json", Message: "schema_version required"})
	}

	ticketRows, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", "tickets.jsonl"))
	if err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Message: err.Error()})
	}
	checkRows(&rep, "ledger/tickets.jsonl", ticketRows, ledger.TicketRequired, true)

	worklogRows, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", "worklog.jsonl"))
	if err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Message: err.Error()})
	}
	checkRows(&rep, "ledger/worklog.jsonl", worklogRows, ledger.WorklogRequired, false)

	checkOrphans(&rep, ticketRows, worklogRows)
	checkBlockers(&rep, ticketRows)
	checkParents(&rep, ticketRows, cfg.Parents)

	if strict {
		rep.Fails = append(rep.Fails, rep.Warns...)
		rep.Warns = nil
	}
	return rep, nil
}

var isoRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)

func checkRows(rep *Report, file string, rows []ledger.Row, required []string, isTicket bool) {
	prevTS := ""
	for i, r := range rows {
		line := i + 1
		// n consecutive from 1
		if n, ok := r["n"].(float64); !ok || int(n) != line {
			rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: fmt.Sprintf("n must equal %d, got %v", line, r["n"])})
		}
		// required fields
		for _, f := range required {
			if _, ok := r[f]; !ok {
				rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "missing required field: " + f})
			}
		}
		nonEmpty := ledger.WorklogNonEmpty
		if isTicket {
			nonEmpty = ledger.TicketNonEmpty
		}
		for _, f := range nonEmpty {
			if v, ok := r[f].(string); !ok || v == "" {
				rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "field must be non-empty: " + f})
			}
		}
		if !isTicket {
			if v, ok := r["ticket"].(string); ok && v == "" {
				rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "field must be non-empty when present: ticket"})
			}
		}
		// ts iso + non-decreasing
		ts, _ := r["ts"].(string)
		if !isoRe.MatchString(ts) {
			rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "ts not ISO8601 UTC: " + ts})
		} else if prevTS != "" && ts < prevTS {
			rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "ts decreases relative to previous row"})
		}
		prevTS = ts
		// status enum (tickets only)
		if isTicket {
			if s, ok := r["status"].(string); ok {
				if _, valid := ledger.StatusEnum[s]; !valid {
					rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "invalid status: " + s})
				}
			}
			if c, ok := r["category"].(string); !ok || c == "" {
				rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Message: "missing category"})
			} else if _, valid := ledger.CategoryEnum[c]; !valid {
				rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Message: "unknown category: " + c})
			}
		}
	}
}

func checkOrphans(rep *Report, tickets, worklog []ledger.Row) {
	known := map[string]struct{}{}
	for _, t := range tickets {
		if id, ok := t["ticket"].(string); ok {
			known[id] = struct{}{}
		}
	}
	for i, w := range worklog {
		id, hasTicket := w["ticket"].(string)
		if !hasTicket || id == "" {
			continue // ticket is optional for worklog
		}
		if _, ok := known[id]; !ok {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/worklog.jsonl", Line: i + 1, Message: "orphan worklog: ticket not found in tickets.jsonl: " + id})
		}
	}
}

func checkBlockers(rep *Report, tickets []ledger.Row) {
	latest := map[string]ledger.Row{}
	for _, t := range tickets {
		if id, ok := t["ticket"].(string); ok && id != "" {
			latest[id] = t
		}
	}
	for i, t := range tickets {
		status, _ := t["status"].(string)
		if status == "done" || status == "cancelled" {
			continue
		}
		blocked, _ := t["blocked_by"].([]any)
		if status == "blocked" && len(blocked) == 0 {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: i + 1, Message: "blocked ticket has empty blocked_by"})
			continue
		}
		unresolved := 0
		for _, raw := range blocked {
			id, _ := raw.(string)
			b := latest[id]
			bs, _ := b["status"].(string)
			if bs == "done" || bs == "cancelled" {
				rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: i + 1, Message: "stale blocker is already closed: " + id})
				continue
			}
			unresolved++
		}
		if status == "blocked" && unresolved == 0 {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: i + 1, Message: "blocked ticket has no unresolved blockers"})
		}
	}
}

func checkParents(rep *Report, tickets []ledger.Row, parents []string) {
	if len(parents) == 0 {
		return
	}
	allowed := map[string]struct{}{}
	for _, p := range parents {
		allowed[p] = struct{}{}
	}
	for i, t := range tickets {
		p, _ := t["parent_ticket"].(string)
		if p == "" {
			continue
		}
		if _, ok := allowed[p]; !ok {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: i + 1, Message: "unknown parent_ticket: " + p})
		}
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/verify/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```
git add internal/verify
git commit -m "feat(verify): fail/warn separation with strict promotion"
```

---

### Task 18: `cmd/verify` wiring

**Files:**
- Create: `cmd/verify.go`
- Modify: `main.go`

- [ ] **Step 1: Write `cmd/verify.go`**

```go
package cmd

import (
	"fmt"
	"io"

	"github.com/hgwk/ledger-kit/internal/verify"
)

func RunVerifyCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("verify")
	target := fs.String("target", "", "")
	strict := fs.Bool("strict", false, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)
	rep, err := verify.RunStrict(dir, *strict)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, w := range rep.Warns {
		fmt.Fprintf(stdout, "warn  %s:%d %s\n", w.File, w.Line, w.Message)
	}
	for _, f := range rep.Fails {
		fmt.Fprintf(stderr, "fail  %s:%d %s\n", f.File, f.Line, f.Message)
	}
	if len(rep.Fails) > 0 {
		return 1
	}
	if len(rep.Warns) == 0 && len(rep.Fails) == 0 {
		fmt.Fprintln(stdout, "ok")
	}
	return 0
}
```

- [ ] **Step 2: Wire into `main.go`**

```go
"verify": func(args []string, stdout, stderr io.Writer) int {
	return cmd.RunVerifyCLI(args, stdout, stderr)
},
```

- [ ] **Step 3: Manually verify end-to-end**

Run:
```
go build -o /tmp/ledger-kit . && /tmp/ledger-kit init --target /tmp/ledger-demo --slug demo && /tmp/ledger-kit verify --target /tmp/ledger-demo
```
Expected: `ok`.

- [ ] **Step 4: Commit**

```
git add cmd main.go
git commit -m "feat(verify): subcommand wiring"
```

---

## Phase 5 — Registry management commands

### Task 19: `cmd/list` (with `--prune`)

**Files:**
- Create: `cmd/list.go`
- Create: `cmd/list_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing test**

`cmd/list_test.go`:
```go
package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ledger-kit/internal/registry"
)

func TestList_PrintsRegistered(t *testing.T) {
	target, store := mustInit(t)
	out := &bytes.Buffer{}
	if code := RunListCLI([]string{}, store, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("list failed")
	}
	if !strings.Contains(out.String(), target) {
		t.Fatalf("expected %q in output, got: %s", target, out.String())
	}
}

func TestList_Prune_RemovesMissingPaths(t *testing.T) {
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))
	store.Register(registry.Project{ProjectID: "id1", Slug: "ghost", Paths: []string{"/nonexistent/path/x"}})

	out := &bytes.Buffer{}
	if code := RunListCLI([]string{"--prune"}, store, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("prune failed")
	}
	r, _ := store.Load()
	if len(r.Projects) != 0 {
		t.Fatalf("expected pruned, got %d projects", len(r.Projects))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL.

- [ ] **Step 3: Implement `cmd/list.go`**

```go
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hgwk/ledger-kit/internal/ids"
	"github.com/hgwk/ledger-kit/internal/registry"
)

func RunListCLI(args []string, store *registry.Store, stdout, stderr io.Writer) int {
	fs := newFlagSet("list")
	prune := fs.Bool("prune", false, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r, err := store.Load()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *prune {
		for _, p := range r.Projects {
			for _, path := range p.Paths {
				if !configExists(path) {
					if err := store.UnregisterPath(path); err != nil {
						fmt.Fprintln(stderr, err)
						return 1
					}
					fmt.Fprintf(stdout, "pruned %s (path missing or no ledger/config.json)\n", path)
				}
			}
		}
		r, _ = store.Load()
	}
	for _, p := range r.Projects {
		fmt.Fprintf(stdout, "%s\n", ids.Display(p.Slug, p.ProjectID))
		for _, path := range p.Paths {
			fmt.Fprintf(stdout, "  %s\n", path)
		}
	}
	return 0
}

func configExists(repoPath string) bool {
	_, err := os.Stat(filepath.Join(repoPath, "ledger", "config.json"))
	return err == nil
}
```

- [ ] **Step 4: Wire into `main.go`**

```go
"list": func(args []string, stdout, stderr io.Writer) int {
	store, err := cmd.DefaultRegistry()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return cmd.RunListCLI(args, store, stdout, stderr)
},
```

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```
git add cmd main.go
git commit -m "feat(list): list registered projects with --prune"
```

---

### Task 20: `cmd/unregister`

**Files:**
- Create: `cmd/unregister.go`
- Create: `cmd/unregister_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing test**

`cmd/unregister_test.go`:
```go
package cmd

import (
	"bytes"
	"testing"
)

func TestUnregister_ByPath(t *testing.T) {
	target, store := mustInit(t)
	if code := RunUnregisterCLI([]string{"--path", target}, store, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("unregister failed")
	}
	r, _ := store.Load()
	if len(r.Projects) != 0 {
		t.Fatalf("expected unregistered, got %+v", r)
	}
}

func TestUnregister_ByID(t *testing.T) {
	_, store := mustInit(t)
	r, _ := store.Load()
	id := r.Projects[0].ProjectID
	if code := RunUnregisterCLI([]string{"--project-id", id}, store, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("unregister failed")
	}
	r, _ = store.Load()
	if len(r.Projects) != 0 {
		t.Fatalf("expected unregistered")
	}
}

func TestUnregister_RequiresOneFlag(t *testing.T) {
	_, store := mustInit(t)
	errb := &bytes.Buffer{}
	code := RunUnregisterCLI([]string{}, store, &bytes.Buffer{}, errb)
	if code == 0 {
		t.Fatalf("expected error without flag")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL.

- [ ] **Step 3: Implement `cmd/unregister.go`**

```go
package cmd

import (
	"fmt"
	"io"

	"github.com/hgwk/ledger-kit/internal/registry"
)

func RunUnregisterCLI(args []string, store *registry.Store, stdout, stderr io.Writer) int {
	fs := newFlagSet("unregister")
	path := fs.String("path", "", "")
	id := fs.String("project-id", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if (*path == "" && *id == "") || (*path != "" && *id != "") {
		fmt.Fprintln(stderr, "specify exactly one of --path or --project-id")
		return 2
	}
	if *path != "" {
		if err := store.UnregisterPath(*path); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		if err := store.UnregisterID(*id); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprintln(stdout, "ok")
	return 0
}
```

- [ ] **Step 4: Wire into `main.go`**

```go
"unregister": func(args []string, stdout, stderr io.Writer) int {
	store, err := cmd.DefaultRegistry()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return cmd.RunUnregisterCLI(args, store, stdout, stderr)
},
```

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```
git add cmd main.go
git commit -m "feat(unregister): by --path or --project-id"
```

---

### Task 21: `cmd/registry repair`

**Files:**
- Create: `cmd/registry.go`
- Create: `cmd/registry_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing test**

`cmd/registry_test.go`:
```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hgwk/ledger-kit/internal/registry"
)

func TestRegistryRepair_BacksUpAndRebuilds(t *testing.T) {
	regDir := t.TempDir()
	regPath := filepath.Join(regDir, "registry.json")
	// Plant a corrupt registry.
	os.WriteFile(regPath, []byte("not json"), 0o644)

	store := registry.New(regPath, filepath.Join(regDir, "registry.lock"))
	out := &bytes.Buffer{}
	if code := RunRegistryCLI([]string{"repair"}, store, regPath, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("repair failed")
	}

	// Original backed up under regDir as registry.corrupt-<timestamp>.json
	entries, _ := os.ReadDir(regDir)
	foundBackup := false
	for _, e := range entries {
		if name := e.Name(); name != "registry.json" && name != "registry.lock" && filepath.Ext(name) == ".json" {
			foundBackup = true
		}
	}
	if !foundBackup {
		t.Fatalf("expected backup file in %s; got: %v", regDir, entries)
	}

	// New registry is fresh.
	r, err := store.Load()
	if err != nil {
		t.Fatalf("load after repair: %v", err)
	}
	if len(r.Projects) != 0 || r.Version != 1 {
		t.Fatalf("expected fresh registry, got %+v", r)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL.

- [ ] **Step 3: Implement `cmd/registry.go`**

```go
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hgwk/ledger-kit/internal/jsonio"
	"github.com/hgwk/ledger-kit/internal/registry"
)

func RunRegistryCLI(args []string, store *registry.Store, registryPath string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "repair" {
		fmt.Fprintln(stderr, "usage: ledger-kit registry repair")
		return 2
	}
	// Back up whatever is there now.
	data, err := os.ReadFile(registryPath)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(data) > 0 {
		bak := filepath.Join(filepath.Dir(registryPath), fmt.Sprintf("registry.corrupt-%s.json", time.Now().UTC().Format("20060102-150405")))
		if err := os.WriteFile(bak, data, 0o644); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "backed up old registry to %s\n", bak)
	}
	if err := jsonio.WriteJSON(registryPath, registry.Registry{Version: 1}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, "registry rebuilt (empty)")
	return 0
}
```

- [ ] **Step 4: Wire into `main.go`**

Since `registry` needs the registry path to write the backup, expose it from `DefaultRegistry`:

In `cmd/registry_path.go`, change `DefaultRegistry` to also return the path:

```go
func DefaultRegistry() (*registry.Store, string, error) {
	home := os.Getenv("LEDGER_KIT_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, "", err
		}
		home = filepath.Join(h, ".ledger-kit")
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, "", err
	}
	regPath := filepath.Join(home, "registry.json")
	lockPath := filepath.Join(home, "registry.lock")
	return registry.New(regPath, lockPath), regPath, nil
}
```

Update **all callers** of `DefaultRegistry()` in `cmd/init.go`, `cmd/list.go`, `cmd/unregister.go`, and `main.go`'s dispatcher entries to handle the extra return value (the registry path is only used by `registry repair`; others can ignore it with `_`).

In `main.go`:
```go
"registry": func(args []string, stdout, stderr io.Writer) int {
	store, regPath, err := cmd.DefaultRegistry()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return cmd.RunRegistryCLI(args, store, regPath, stdout, stderr)
},
```

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```
git add cmd main.go
git commit -m "feat(registry): repair command with backup"
```

---

## Phase 6 — End-to-end smoke

### Task 22: End-to-end CLI smoke test

**Files:**
- Create: `e2e/e2e_test.go`

Exercise the built binary against a temp directory to confirm the whole pipeline works.

- [ ] **Step 1: Write the e2e test**

`e2e/e2e_test.go`:
```go
package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "ledger-kit")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = repoRoot(t)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("build: %v\n%s", err, errb.String())
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	return filepath.Dir(wd) // e2e/ -> repo
}

func TestSmoke_InitTicketWorklogVerify(t *testing.T) {
	bin := buildBinary(t)
	work := t.TempDir()
	home := t.TempDir() // isolated registry

	run := func(args ...string) (string, string, int) {
		c := exec.Command(bin, args...)
		c.Env = append(os.Environ(), "LEDGER_KIT_HOME="+home, "LEDGER_AGENT=codex")
		var so, se bytes.Buffer
		c.Stdout = &so
		c.Stderr = &se
		err := c.Run()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("run error: %v", err)
		}
		return so.String(), se.String(), code
	}

	if _, se, code := run("init", "--target", work, "--slug", "demo"); code != 0 {
		t.Fatalf("init: %s", se)
	}

	ticketJSON := `{"ticket":"t1","parent_ticket":"ROOT","role":"impl","status":"open","task":"do thing","scope":"repo","paths":[],"blocked_by":[]}`
	c := exec.Command(bin, "ticket", "add", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LEDGER_KIT_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(ticketJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("ticket add: %v\n%s", err, out)
	}

	eventJSON := `{"ticket":"t1","status":"done","notes":"shipped"}`
	c = exec.Command(bin, "ticket", "event", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LEDGER_KIT_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(eventJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("ticket event: %v\n%s", err, out)
	}

	worklogJSON := `{"ticket":"t1","task":"impl","scope":"repo","result":"done","paths":[],"commands":[]}`
	c = exec.Command(bin, "worklog", "add", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LEDGER_KIT_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(worklogJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("worklog add: %v\n%s", err, out)
	}

	if so, se, code := run("verify", "--target", work); code != 0 {
		t.Fatalf("verify: stdout=%s stderr=%s", so, se)
	}
}
```

- [ ] **Step 2: Run e2e**

Run: `go test ./e2e/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```
git add e2e
git commit -m "test: end-to-end smoke (init → ticket → event → worklog → verify)"
```

---

## Self-Review Checklist

After implementing all tasks above, run through these checks:

- [ ] `go test ./...` — all green
- [ ] `go vet ./...` — clean
- [ ] `gofmt -l .` — no output (well-formatted)
- [ ] All eight subcommands respond: `init`, `ticket add|event`, `worklog add`, `goal set|show`, `verify`, `list`, `unregister`, `registry repair`
- [ ] `ledger-kit init` is idempotent (re-running on the same dir keeps the same `project_id`)
- [ ] `verify` separates fail vs warn; `--strict` promotes warns
- [ ] All ledger writes go through `ledger.Append` (lock + monotonic n)
- [ ] Spec sections covered by this plan: §3.0 (operational artifacts via init), §3.1–§3.6, §4.1 partial (init/list/unregister/registry repair/verify), §4.2 (ticket/worklog/goal add/event/set/show), §5 (auto fields and agent resolver), §6 partial (verify framework + the fail rules + orphan/unknown-parent warns), §7.1 (registry).
- [ ] **Known §6.2 gaps left for trivial follow-up** (the framework is in place; each is one helper function added to `internal/verify/verify.go`):
  - warn: "closed/done ticket without any matching worklog row"
  - warn: "branch != branch_convention" — only when `gitutil.IsWorkTree(dir)` and `branch != ""`
- [ ] Spec sections explicitly **deferred** to later plans: §7.2 viewer (Plan 3), §8 hooks (Plan 4), §9 instructions (Plan 4), §11 legacy import (Plan 2), §2 release workflow (Plan 4).

---

## Done — what works after this plan

A fresh user can:

```
go install github.com/hgwk/ledger-kit@latest    # (assumes repo published; otherwise go build)
cd /path/to/some/project
ledger-kit init --slug my-project
echo '{"ticket":"t1","parent_ticket":"ROOT","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}' | ledger-kit ticket add --json @-
echo '{"ticket":"t1","status":"done"}' | ledger-kit ticket event --json @-
echo '{"ticket":"t1","task":"impl","scope":"repo","result":"done","paths":[],"commands":[]}' | ledger-kit worklog add --json @-
ledger-kit verify
ledger-kit list
```

Viewer, legacy import, hooks/instructions, and the release pipeline are scoped to the next three plans.
