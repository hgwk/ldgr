# ldgr Hooks, Instructions, Release Plan (Plan 4B)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make `ldgr` installable in real repos and shippable as prebuilt binaries:
- `ldgr hooks install|uninstall` — idempotent `.git/hooks/pre-commit` integration that preserves existing hooks.
- `ldgr instructions install|uninstall` — short pointer in `AGENTS.md` / `CLAUDE.md`, full guidance in `ledger/instructions/`.
- `.github/workflows/release.yml` — runs the full quality gate (test/vet/gofmt) and ships darwin/linux arm64/amd64 binaries on tag push.

**Architecture:**
- `cmd/hooks.go` owns hook install/uninstall with marker-aware insertion and a single backup file.
- `cmd/instructions.go` writes ledger-owned instruction bodies and a small marker block in `AGENTS.md`/`CLAUDE.md`. Templates live under `templates/instructions/`.
- The release workflow uses stdlib-only Go build matrix; no external actions beyond `actions/checkout`, `actions/setup-go`, and `softprops/action-gh-release` (or equivalent stdlib-friendly release uploader).

**Tech Stack:** Go 1.22+ stdlib only at runtime. GitHub Actions YAML for CI.

**Spec reference:** §8 (hooks), §9 (instructions).

**Predecessors:** Plan 1 (Foundation), Plan 1.1 (hardening), Plan 2 (legacy import), Plan 3 (viewer), Plan 4A (guidance). Rename pass `ledger-kit → ldgr` already landed.

---

## Hard Acceptance Criteria

1. `ldgr hooks install` makes `.git/hooks/pre-commit` exit non-zero when `ldgr verify` fails. Existing user hook content is preserved. Re-running is a no-op (no duplicate marker block). A single `.git/hooks/pre-commit.ldgr.bak` snapshot of the pre-existing file is created on first install.
2. `ldgr hooks uninstall` removes only the LDGR marker block (and the leading shebang if we added one). If the resulting file is empty, it's deleted; otherwise existing user lines remain.
3. `ldgr instructions install` creates `ledger/instructions/AGENTS.ldgr.md` and `ledger/instructions/CLAUDE.ldgr.md`. If `AGENTS.md` / `CLAUDE.md` exist, prepend a marker block pointing at the ledger-owned file. If they don't exist, create stubs that contain only the marker block.
4. `ldgr instructions install` is idempotent: a second run rewrites only the ledger-owned bodies (so updates to the embedded text propagate) and leaves the marker blocks unchanged.
5. `ldgr instructions install` recognises the historical `LEDGER_KIT_START` / `LEDGER_KIT_END` marker pair as equivalent to the new `LDGR_START` / `LDGR_END` for idempotency and rewrites the old block in place. No silent duplication.
6. `ldgr instructions uninstall` removes only the marker block; existing user content stays. `ledger/instructions/*.ldgr.md` is deleted unless the user passes `--keep-bodies`.
7. `.github/workflows/release.yml` runs on `v*` tag push: executes `go test ./... -race`, `go vet ./...`, and `gofmt -l .` (fails if non-empty); then builds darwin-amd64, darwin-arm64, linux-amd64, linux-arm64; uploads `ldgr_<version>_<os>_<arch>.tar.gz` artifacts containing the binary, README, and LICENSE.
8. `go test ./... -count=1 -race`, `go vet ./...`, `gofmt -l .` clean at the end of every task.

---

## Decisions Locked

- **Marker name moves to LDGR.** New blocks use `<!-- LDGR_START -->` / `<!-- LDGR_END -->`. The installer recognises legacy `LEDGER_KIT_START` markers for idempotency + migration.
- **Reference instruction mode is default.** AGENTS.md/CLAUDE.md hold only a short pointer; full text lives in `ledger/instructions/*.ldgr.md`. No `--inline` flag in this plan.
- **No symlink mode.** Out of scope (filesystem portability + Windows risk).
- **Hooks file format:** shebang on line 1 (added if missing), then marker block, then any pre-existing user content. The marker block is the FIRST runnable block to guarantee `exit 1` short-circuits user hooks that ignore failures.
- **One backup, one shot.** Hooks install creates `.ldgr.bak` only if no backup already exists; subsequent installs preserve the very first snapshot. Uninstall removes the backup once the hook is clean.
- **No release artifact for ledger-kit alias.** The rename is clean. Users on the old name install via `ledger-kit → ldgr` symlink themselves if they want it.
- **Workflow uses GITHUB_TOKEN only.** No third-party tokens, no signing in v0; that's a follow-up.

---

## File Structure

```
ldgr/
  cmd/
    hooks.go              hooks_test.go
    instructions.go       instructions_test.go
  templates/
    instructions/
      AGENTS.ldgr.md
      CLAUDE.ldgr.md
  .github/workflows/release.yml
```

---

## Task Decomposition

5 tasks. Each one TDD cycle (test → impl → pass → commit) except Task 2 (templates only) and Task 4 (CI YAML).

---

### Task 1: `cmd/hooks.go` — install/uninstall

**Files:**
- Create: `cmd/hooks.go`
- Create: `cmd/hooks_test.go`

#### Step 1: Write `cmd/hooks_test.go`

```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeHook(t *testing.T, dir, content string) {
	t.Helper()
	p := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(p, "pre-commit"), []byte(content), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readHook(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".git", "hooks", "pre-commit"))
	if err != nil {
		return ""
	}
	return string(data)
}

func TestHooksInstall_CreatesNewHookWithMarker(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)

	if code := RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}
	hook := readHook(t, dir)
	if !strings.HasPrefix(hook, "#!") {
		t.Fatalf("hook should start with shebang: %q", hook[:20])
	}
	if !strings.Contains(hook, "LDGR_START") || !strings.Contains(hook, "LDGR_END") {
		t.Fatalf("missing markers: %s", hook)
	}
	if !strings.Contains(hook, "ldgr verify") {
		t.Fatalf("missing verify call: %s", hook)
	}
}

func TestHooksInstall_PreservesExistingHook(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "#!/usr/bin/env bash\necho user hook ran\n")

	if code := RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}
	hook := readHook(t, dir)
	if !strings.Contains(hook, "user hook ran") {
		t.Fatalf("existing user content lost: %s", hook)
	}
	if !strings.Contains(hook, "LDGR_START") {
		t.Fatalf("missing ldgr marker: %s", hook)
	}
	// Backup must exist.
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-commit.ldgr.bak")); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

func TestHooksInstall_Idempotent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	first := readHook(t, dir)
	RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	second := readHook(t, dir)
	if first != second {
		t.Fatalf("re-install changed hook:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestHooksUninstall_PreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "#!/usr/bin/env bash\necho user hook ran\n")
	RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	if code := RunHooksCLI([]string{"uninstall", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("uninstall failed")
	}
	hook := readHook(t, dir)
	if !strings.Contains(hook, "user hook ran") {
		t.Fatalf("user content lost: %s", hook)
	}
	if strings.Contains(hook, "LDGR_START") {
		t.Fatalf("marker survived uninstall: %s", hook)
	}
}

func TestHooksUninstall_DeletesEmptyHook(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	if code := RunHooksCLI([]string{"uninstall", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("uninstall failed")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-commit")); !os.IsNotExist(err) {
		t.Fatalf("hook should be deleted when only ldgr block remained, stat err=%v", err)
	}
}
```

#### Step 2: Verify FAIL

`go test ./cmd/... -run TestHooks` — `undefined: RunHooksCLI`.

#### Step 3: Write `cmd/hooks.go`

```go
package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Commands["hooks"] = RunHooksCLI
}

const (
	hookMarkerStart = "# >>> LDGR_HOOK_START >>>"
	hookMarkerEnd   = "# <<< LDGR_HOOK_END <<<"
	hookBackupSfx   = ".ldgr.bak"
)

func hookBlock() string {
	return hookMarkerStart + "\n" +
		"ldgr verify || exit 1\n" +
		hookMarkerEnd + "\n"
}

// RunHooksCLI implements `ldgr hooks install|uninstall`.
func RunHooksCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr hooks <install|uninstall> [--target PATH]")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("hooks " + sub)
	target := fs.String("target", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	dir := resolveTarget(*target)
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	switch sub {
	case "install":
		return runHooksInstall(hookPath, stdout, stderr)
	case "uninstall":
		return runHooksUninstall(hookPath, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown hooks subcommand: %s\n", sub)
		return 2
	}
}

func runHooksInstall(hookPath string, stdout, stderr io.Writer) int {
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	existing, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if bytes.Contains(existing, []byte(hookMarkerStart)) {
		// Idempotent: marker already present.
		fmt.Fprintln(stdout, "hooks already installed")
		return 0
	}
	// Take a single backup if user content existed and no backup yet.
	if len(existing) > 0 {
		bak := hookPath + hookBackupSfx
		if _, err := os.Stat(bak); os.IsNotExist(err) {
			if err := os.WriteFile(bak, existing, 0o755); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
	}
	var content bytes.Buffer
	if len(existing) > 0 && bytes.HasPrefix(existing, []byte("#!")) {
		// Preserve user shebang; insert marker block right after the shebang line.
		nl := bytes.IndexByte(existing, '\n')
		if nl < 0 {
			nl = len(existing)
		}
		content.Write(existing[:nl+1])
		content.WriteString(hookBlock())
		content.Write(existing[nl+1:])
	} else {
		content.WriteString("#!/usr/bin/env bash\n")
		content.WriteString(hookBlock())
		content.Write(existing)
	}
	if err := os.WriteFile(hookPath, content.Bytes(), 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "installed pre-commit hook at %s\n", hookPath)
	return 0
}

func runHooksUninstall(hookPath string, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(stdout, "no hook to uninstall")
			return 0
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	cleaned := removeHookBlock(string(data))
	// If only the shebang line remains (or empty), delete the file entirely.
	trimmed := strings.TrimSpace(cleaned)
	if trimmed == "" || trimmed == "#!/usr/bin/env bash" {
		if err := os.Remove(hookPath); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		// Best-effort backup cleanup.
		_ = os.Remove(hookPath + hookBackupSfx)
		fmt.Fprintln(stdout, "removed pre-commit hook")
		return 0
	}
	if err := os.WriteFile(hookPath, []byte(cleaned), 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	_ = os.Remove(hookPath + hookBackupSfx)
	fmt.Fprintln(stdout, "removed ldgr hook block; user content preserved")
	return 0
}

func removeHookBlock(s string) string {
	start := strings.Index(s, hookMarkerStart)
	if start < 0 {
		return s
	}
	end := strings.Index(s[start:], hookMarkerEnd)
	if end < 0 {
		return s
	}
	end = start + end + len(hookMarkerEnd)
	// Eat the trailing newline if present so we don't leave a blank line.
	if end < len(s) && s[end] == '\n' {
		end++
	}
	return s[:start] + s[end:]
}
```

#### Step 4: Tests pass

`go test ./cmd/... -run TestHooks -v -race`.

#### Step 5: Full suite

`go test ./... -count=1`.

#### Step 6: Commit

```
git add cmd/hooks.go cmd/hooks_test.go
git commit -m "feat(hooks): install/uninstall pre-commit verify with marker idempotency"
```

---

### Task 2: instruction templates

**Files:**
- Create: `templates/instructions/AGENTS.ldgr.md`
- Create: `templates/instructions/CLAUDE.ldgr.md`

These are static text. The installer (Task 3) embeds and writes them.

#### Step 1: Write `templates/instructions/AGENTS.ldgr.md`

```markdown
# ldgr — agent instructions

This repo uses **ldgr** (formerly `ledger-kit`) as the operational source of
truth for goals, tickets, and worklogs. Every multi-step task must flow through
the ledger so agents and humans share a single timeline.

## Ground rules

- **Append-only.** Never edit existing rows in `ledger/tickets.jsonl` or
  `ledger/worklog.jsonl`. Append a new row instead.
- **Latest row wins.** The most recent row per `ticket` is the current state.
- **`done` means audit-pass.** A bare `status=done` is weak; pair it with
  `role=audit`, `audit_result=pass`, and `evidence`.
- **Worklog after audit.** `ldgr worklog add` is for shipped deliveries
  recorded after an audit-pass `done` row, not "I think I'm done" notes.
- **Guidance is always one command away:** `ldgr next --ticket <id>`.

## Daily commands

```bash
ldgr next --ticket <id>                # what should I do next on this ticket?
ldgr ticket add   --json @-            # create a new ticket
ldgr ticket event --json @-            # state transition or correction
ldgr worklog add  --json @-            # record shipped delivery (after audit pass)
ldgr verify                            # validate the ledger
ldgr view                              # multi-project dashboard (http://127.0.0.1:3030)
```

## Lifecycle

```
open → in_progress → audit_ready → done
                  ↘ changes_requested → in_progress
```

If you get stuck on what to do, run `ldgr next --ticket <id> --format json` and
follow the suggested command + skeleton.
```

#### Step 2: Write `templates/instructions/CLAUDE.ldgr.md`

```markdown
# Claude — ldgr operating notes

These are the rules the agent in this repo must respect. Read once, then keep
the daily commands at hand.

## Source of truth

`ledger/` holds the operational state. Treat the four files as canonical:

- `ledger/goal.json` — current objective.
- `ledger/tickets.jsonl` — append-only ticket lifecycle.
- `ledger/worklog.jsonl` — append-only completed-delivery log.
- `ledger/config.json` — repo identity + parent rules.

You do not edit these files directly. Use the CLI:

```bash
ldgr ticket add   --json @-
ldgr ticket event --json @-
ldgr worklog add  --json @-
ldgr goal set     --json @-
```

After every write, stderr contains the next concrete action. Read it.

## Lifecycle the binary will enforce

1. Implementation: `open → in_progress → audit_ready` (you).
2. Audit: append `role=audit` row with `audit_result=pass` (done) or
   `changes_requested`.
3. Worklog: only after the audit-pass `done` row.
4. Commit/PR: `ldgr suggest commit --ticket <id>` gives you the Conventional
   Commit line + verification block.

## When confused

```bash
ldgr next --ticket <id>
ldgr suggest worklog --ticket <id>
ldgr suggest commit  --ticket <id>
```

These read the ledger and tell you exactly which JSON to feed back into
`--json @-`.
```

#### Step 3: Commit

```
git add templates/instructions/
git commit -m "feat(instructions): ldgr-owned AGENTS / CLAUDE bodies"
```

---

### Task 3: `cmd/instructions.go` — install/uninstall

**Files:**
- Create: `cmd/instructions.go`
- Create: `cmd/instructions_test.go`

#### Step 1: Write `cmd/instructions_test.go`

```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const legacyBlock = `<!-- LEDGER_KIT_START -->
old pointer body
<!-- LEDGER_KIT_END -->
`

func TestInstructionsInstall_CreatesBodiesAndPointer(t *testing.T) {
	dir := t.TempDir()
	if code := RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}
	for _, p := range []string{"ledger/instructions/AGENTS.ldgr.md", "ledger/instructions/CLAUDE.ldgr.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	for _, p := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
		if !strings.Contains(string(data), "LDGR_START") {
			t.Fatalf("missing pointer in %s: %s", p, data)
		}
	}
}

func TestInstructionsInstall_PreservesExistingMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# My project\nuser content\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(data), "user content") {
		t.Fatalf("user content lost: %s", data)
	}
	if !strings.Contains(string(data), "LDGR_START") {
		t.Fatalf("missing marker: %s", data)
	}
}

func TestInstructionsInstall_Idempotent(t *testing.T) {
	dir := t.TempDir()
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	first, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	second, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(first) != string(second) {
		t.Fatalf("re-install changed pointer:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestInstructionsInstall_MigratesLegacyMarker(t *testing.T) {
	dir := t.TempDir()
	body := legacyBlock + "# project\nuser content\n"
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644)
	if code := RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if strings.Contains(string(data), "LEDGER_KIT_START") {
		t.Fatalf("legacy marker should be migrated, still present: %s", data)
	}
	if !strings.Contains(string(data), "LDGR_START") {
		t.Fatalf("new marker missing: %s", data)
	}
	if !strings.Contains(string(data), "user content") {
		t.Fatalf("user content lost: %s", data)
	}
}

func TestInstructionsUninstall_RemovesPointer(t *testing.T) {
	dir := t.TempDir()
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	if code := RunInstructionsCLI([]string{"uninstall", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("uninstall failed")
	}
	for _, p := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := os.ReadFile(filepath.Join(dir, p))
		if err == nil && strings.Contains(string(data), "LDGR_START") {
			t.Fatalf("marker survived uninstall in %s: %s", p, data)
		}
	}
	// Bodies removed by default.
	if _, err := os.Stat(filepath.Join(dir, "ledger", "instructions", "AGENTS.ldgr.md")); !os.IsNotExist(err) {
		t.Fatalf("body should be removed: err=%v", err)
	}
}

func TestInstructionsUninstall_KeepBodies(t *testing.T) {
	dir := t.TempDir()
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	RunInstructionsCLI([]string{"uninstall", "--target", dir, "--keep-bodies"}, &bytes.Buffer{}, &bytes.Buffer{})
	if _, err := os.Stat(filepath.Join(dir, "ledger", "instructions", "AGENTS.ldgr.md")); err != nil {
		t.Fatalf("body should be kept: %v", err)
	}
}
```

#### Step 2: Verify FAIL

`go test ./cmd/... -run TestInstructions` — `undefined: RunInstructionsCLI`.

#### Step 3: Write `cmd/instructions.go`

```go
package cmd

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Commands["instructions"] = RunInstructionsCLI
}

//go:embed ../templates/instructions/AGENTS.ldgr.md
var agentsBody string

//go:embed ../templates/instructions/CLAUDE.ldgr.md
var claudeBody string

const (
	instrMarkerStart = "<!-- LDGR_START -->"
	instrMarkerEnd   = "<!-- LDGR_END -->"
	legacyStart      = "<!-- LEDGER_KIT_START -->"
	legacyEnd        = "<!-- LEDGER_KIT_END -->"
)

type instructionTarget struct {
	pointerFile string // e.g. AGENTS.md
	bodyRel     string // e.g. ledger/instructions/AGENTS.ldgr.md
	body        string
}

func targets() []instructionTarget {
	return []instructionTarget{
		{"AGENTS.md", "ledger/instructions/AGENTS.ldgr.md", agentsBody},
		{"CLAUDE.md", "ledger/instructions/CLAUDE.ldgr.md", claudeBody},
	}
}

// RunInstructionsCLI implements `ldgr instructions install|uninstall`.
func RunInstructionsCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr instructions <install|uninstall> [--target PATH] [--keep-bodies]")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("instructions " + sub)
	target := fs.String("target", "", "")
	keepBodies := fs.Bool("keep-bodies", false, "uninstall only: leave ledger/instructions/*.ldgr.md in place")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	dir := resolveTarget(*target)
	switch sub {
	case "install":
		return runInstructionsInstall(dir, stdout, stderr)
	case "uninstall":
		return runInstructionsUninstall(dir, *keepBodies, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown instructions subcommand: %s\n", sub)
		return 2
	}
}

func runInstructionsInstall(dir string, stdout, stderr io.Writer) int {
	for _, t := range targets() {
		bodyPath := filepath.Join(dir, t.bodyRel)
		if err := os.MkdirAll(filepath.Dir(bodyPath), 0o755); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := os.WriteFile(bodyPath, []byte(t.body), 0o644); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}

		pointerPath := filepath.Join(dir, t.pointerFile)
		current := ""
		if data, err := os.ReadFile(pointerPath); err == nil {
			current = string(data)
		} else if !os.IsNotExist(err) {
			fmt.Fprintln(stderr, err)
			return 1
		}
		updated := upsertPointer(current, t.bodyRel)
		if updated == current {
			continue
		}
		if err := os.WriteFile(pointerPath, []byte(updated), 0o644); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprintln(stdout, "instructions installed")
	return 0
}

func runInstructionsUninstall(dir string, keepBodies bool, stdout, stderr io.Writer) int {
	for _, t := range targets() {
		pointerPath := filepath.Join(dir, t.pointerFile)
		if data, err := os.ReadFile(pointerPath); err == nil {
			cleaned := removeBlock(string(data), instrMarkerStart, instrMarkerEnd)
			cleaned = removeBlock(cleaned, legacyStart, legacyEnd)
			if strings.TrimSpace(cleaned) == "" {
				_ = os.Remove(pointerPath)
			} else {
				if err := os.WriteFile(pointerPath, []byte(cleaned), 0o644); err != nil {
					fmt.Fprintln(stderr, err)
					return 1
				}
			}
		}
		if !keepBodies {
			_ = os.Remove(filepath.Join(dir, t.bodyRel))
		}
	}
	if !keepBodies {
		_ = os.Remove(filepath.Join(dir, "ledger", "instructions"))
	}
	fmt.Fprintln(stdout, "instructions uninstalled")
	return 0
}

func upsertPointer(current, bodyRel string) string {
	pointer := instrMarkerStart + "\n" +
		"See [`" + bodyRel + "`](" + bodyRel + ") for the full ldgr operating guide.\n" +
		instrMarkerEnd + "\n"
	// Migrate legacy block in place.
	if i := strings.Index(current, legacyStart); i >= 0 {
		if j := strings.Index(current[i:], legacyEnd); j >= 0 {
			end := i + j + len(legacyEnd)
			if end < len(current) && current[end] == '\n' {
				end++
			}
			return current[:i] + pointer + current[end:]
		}
	}
	// Already has new marker → no-op.
	if strings.Contains(current, instrMarkerStart) {
		return current
	}
	// Otherwise prepend.
	if current == "" {
		return pointer
	}
	return pointer + "\n" + current
}

func removeBlock(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return s
	}
	j := strings.Index(s[i:], end)
	if j < 0 {
		return s
	}
	cut := i + j + len(end)
	if cut < len(s) && s[cut] == '\n' {
		cut++
	}
	// Trim a leading blank line if removing the block leaves one.
	out := s[:i] + s[cut:]
	out = strings.TrimPrefix(out, "\n")
	return out
}
```

#### Step 4: Tests pass

`go test ./cmd/... -run TestInstructions -v -race`.

#### Step 5: Full suite

`go test ./... -count=1`.

#### Step 6: Commit

```
git add cmd/instructions.go cmd/instructions_test.go
git commit -m "feat(instructions): install/uninstall with marker migration"
```

---

### Task 4: release workflow

**Files:**
- Create: `.github/workflows/release.yml`
- Create: `LICENSE` if missing (MIT shell — user can replace later)

#### Step 1: Add `LICENSE` if missing

If the repo has no `LICENSE`, write this MIT placeholder. If `LICENSE` already exists, leave it alone.

```
MIT License

Copyright (c) 2026 hgwk

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

#### Step 2: Write `.github/workflows/release.yml`

```yaml
name: release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  quality:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true
      - name: gofmt
        run: |
          set -euo pipefail
          unformatted=$(gofmt -l .)
          if [ -n "$unformatted" ]; then
            echo "::error::gofmt failed:"
            echo "$unformatted"
            exit 1
          fi
      - name: go vet
        run: go vet ./...
      - name: go test
        run: go test ./... -count=1 -race

  build:
    needs: quality
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - goos: darwin
            goarch: amd64
          - goos: darwin
            goarch: arm64
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true
      - name: build
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          CGO_ENABLED: "0"
        run: |
          set -euo pipefail
          version="${GITHUB_REF_NAME#v}"
          out="dist/ldgr_${version}_${GOOS}_${GOARCH}"
          mkdir -p "$out"
          go build -trimpath -ldflags "-s -w" -o "$out/ldgr" .
          cp README.md LICENSE "$out/"
          tar -C dist -czf "${out}.tar.gz" "$(basename "$out")"
      - name: upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: ldgr_${{ matrix.goos }}_${{ matrix.goarch }}
          path: dist/*.tar.gz

  release:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          path: dist
          merge-multiple: true
      - name: attach to release
        uses: softprops/action-gh-release@v2
        with:
          files: dist/*.tar.gz
```

#### Step 3: Sanity check the YAML locally

You can't run GitHub Actions locally, but you should at least:

```bash
# Validate YAML loads.
python3 -c "import sys, yaml; yaml.safe_load(open('.github/workflows/release.yml'))"
# Build cross-arch once to verify the matrix command works in principle.
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/ldgr-darwin-arm64 .
GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/ldgr-linux-amd64 .
file /tmp/ldgr-darwin-arm64 /tmp/ldgr-linux-amd64
rm /tmp/ldgr-darwin-arm64 /tmp/ldgr-linux-amd64
```

(`file` should report both binaries with their correct target OS/arch.)

#### Step 4: Commit

```
git add .github/workflows/release.yml LICENSE
git commit -m "chore(release): tagged GitHub Actions build matrix"
```

---

### Task 5: README + final smoke

**Files:**
- Modify: `README.md`

Append after the existing sections:

```markdown

## Install

Until v1 is tagged, build from source:

```bash
go install github.com/hgwk/ldgr@latest
```

After v1, GitHub Releases will publish `ldgr_<version>_<os>_<arch>.tar.gz`
artifacts for darwin/linux × arm64/amd64.

## Integrate into a repo

```bash
ldgr init                                # seed ledger/* in the current repo
ldgr hooks install                       # pre-commit verify
ldgr instructions install                # AGENTS.md / CLAUDE.md pointer + ledger-owned bodies
ldgr view --target .                     # dashboard for this project only
```

Uninstall:

```bash
ldgr instructions uninstall              # remove pointer + bodies
ldgr instructions uninstall --keep-bodies
ldgr hooks uninstall                     # remove only the LDGR hook block
```
```

Final smoke:

```bash
go test ./... -count=1 -race
go vet ./...
gofmt -l .
```

Commit:

```
git add README.md
git commit -m "docs(readme): install / hooks / instructions usage"
```

---

## Self-Review Checklist

- [ ] `hooks install` is idempotent and preserves user content.
- [ ] `hooks uninstall` removes only the LDGR marker block and deletes the file if empty.
- [ ] `instructions install` creates both bodies and prepends pointer blocks; legacy `LEDGER_KIT_START` is migrated in place.
- [ ] `instructions uninstall` removes pointer block and (unless `--keep-bodies`) the body files.
- [ ] `release.yml` runs test/vet/gofmt before any build and uploads 4 platform archives.
- [ ] Local cross-compile reproduces the matrix builds.
- [ ] Final `go test / go vet / gofmt -l .` clean.

---

## Out of Scope

- Symlink instruction mode.
- Code signing / notarization.
- Homebrew tap publication (follow-up plan).
- Windows builds (deferred until there's demonstrated demand).
- MCP server.
