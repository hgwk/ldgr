package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestView_EndToEnd(t *testing.T) {
	bin := buildBinary(t)
	work := t.TempDir()
	home := t.TempDir()

	env := append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")

	// Init the project.
	mustRun(t, bin, env, "init", "--target", work, "--slug", "viewfix")

	// Append a single ticket so the dashboard has something to render.
	ticketJSON := `{"ticket":"BUG-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	mustRunStdin(t, bin, env, ticketJSON, "ticket", "add", "--target", work, "--json", "@-")

	// Spawn the viewer.
	port := freePort(t)
	cmd := exec.Command(bin, "view", "--port", fmt.Sprint(port), "--target", work)
	cmd.Env = env
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("view start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	waitForPort(t, port)

	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	// /api/projects: single entry.
	var projects []map[string]any
	getJSON(t, base+"/api/projects", &projects)
	if len(projects) != 1 {
		t.Fatalf("want 1 project, got %d: %v\nstderr=%s", len(projects), projects, stderr)
	}
	pid, _ := projects[0]["project_id"].(string)
	if pid == "" {
		t.Fatalf("missing project_id: %+v", projects[0])
	}

	// Project detail.
	var detail map[string]any
	getJSON(t, base+"/api/projects/"+pid, &detail)
	if detail["project_id"] != pid {
		t.Fatalf("detail project_id wrong: %+v", detail)
	}

	// Tickets tree.
	var tickets map[string]any
	getJSON(t, base+"/api/projects/"+pid+"/tickets", &tickets)
	tree, _ := tickets["tree"].([]any)
	if len(tree) == 0 {
		t.Fatalf("empty tree: %+v", tickets)
	}

	// Worklog (empty but well-formed).
	var wl map[string]any
	getJSON(t, base+"/api/projects/"+pid+"/worklog", &wl)
	if _, ok := wl["rows"]; !ok {
		t.Fatalf("worklog missing rows key: %+v", wl)
	}

	// Insights.
	var ins map[string]any
	getJSON(t, base+"/api/projects/"+pid+"/insights", &ins)
	rq, _ := ins["readyQueue"].([]any)
	if len(rq) == 0 {
		t.Fatalf("readyQueue empty: %+v", ins)
	}

	// Static / index.
	body := mustGET(t, base+"/")
	if !strings.Contains(body, "<title>ldgr</title>") {
		t.Fatalf("index missing title: %s", body)
	}
}

func mustRun(t *testing.T, bin string, env []string, args ...string) {
	t.Helper()
	c := exec.Command(bin, args...)
	c.Env = env
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", bin, args, err, out)
	}
}

func mustRunStdin(t *testing.T, bin string, env []string, stdin string, args ...string) {
	t.Helper()
	c := exec.Command(bin, args...)
	c.Env = env
	c.Stdin = strings.NewReader(stdin)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", bin, args, err, out)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForPort(t *testing.T, port int) {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("port %d never came up", port)
}

func getJSON(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s: status %d\n%s", url, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}

func mustGET(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s: status %d\n%s", url, resp.StatusCode, body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return string(body)
}
