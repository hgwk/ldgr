package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "ldgr")
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
		c.Env = append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")
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

	// Add ticket with status=open
	ticketJSON := `{"ticket":"t1","parent_ticket":"ROOT","role":"impl","status":"open","task":"do thing","scope":"repo","paths":[],"blocked_by":[]}`
	c := exec.Command(bin, "ticket", "add", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(ticketJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("ticket add: %v\n%s", err, out)
	}

	// Transition to in_progress
	inProgJSON := `{"ticket":"t1","status":"in_progress"}`
	c = exec.Command(bin, "ticket", "event", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(inProgJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("ticket event in_progress: %v\n%s", err, out)
	}

	// Transition to audit_ready with evidence
	auditReadyJSON := `{"ticket":"t1","status":"audit_ready","evidence":["go test ./..."]}`
	c = exec.Command(bin, "ticket", "event", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(auditReadyJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("ticket event audit_ready: %v\n%s", err, out)
	}

	// Look up the audit_ready row n so we can pass reviewed_n
	rows, err := ledger.ReadRows(filepath.Join(work, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("read tickets.jsonl: %v", err)
	}
	auditN := int(rows[len(rows)-1]["n"].(float64))

	// Audit-pass close
	doneJSON := fmt.Sprintf(`{"ticket":"t1","role":"audit","status":"done","audit_result":"pass","evidence":["go test ./..."],"reviewed_n":%d}`, auditN)
	c = exec.Command(bin, "ticket", "event", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(doneJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("ticket event done: %v\n%s", err, out)
	}

	worklogJSON := `{"ticket":"t1","task":"impl","scope":"repo","result":"done","paths":[],"commands":[],"notes":""}`
	c = exec.Command(bin, "worklog", "add", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(worklogJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("worklog add: %v\n%s", err, out)
	}

	if so, se, code := run("verify", "--target", work); code != 0 {
		t.Fatalf("verify: stdout=%s stderr=%s", so, se)
	}
}
