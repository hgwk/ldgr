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

	ticketJSON := `{"ticket":"t1","parent_ticket":"ROOT","role":"impl","status":"open","task":"do thing","scope":"repo","paths":[],"blocked_by":[]}`
	c := exec.Command(bin, "ticket", "add", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(ticketJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("ticket add: %v\n%s", err, out)
	}

	eventJSON := `{"ticket":"t1","status":"done","notes":"shipped"}`
	c = exec.Command(bin, "ticket", "event", "--target", work, "--json", "@-")
	c.Env = append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")
	c.Stdin = strings.NewReader(eventJSON)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("ticket event: %v\n%s", err, out)
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
