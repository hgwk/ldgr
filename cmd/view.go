package cmd

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/hgwk/ldgr/internal/viewer"
)

func init() {
	Commands["view"] = RunViewCLI
}

// RunViewCLI starts the read-only viewer on localhost.
func RunViewCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("view")
	port := fs.Int("port", 3030, "port to listen on; 0 asks the OS for an open port")
	target := fs.String("target", "", "single-project mode: serve only this directory")
	noOpen := fs.Bool("no-open", false, "do not open the viewer in a browser")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var srv *viewer.Server
	if *target != "" {
		abs, err := filepath.Abs(*target)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		s, err := viewer.NewSingleProjectServer(abs)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		srv = s
	} else {
		store, _, err := DefaultRegistry()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		srv = viewer.NewServer(store)
	}

	// Reuse an already-running viewer instead of spawning a duplicate on the next
	// free port. Without this, repeated `ldgr view` stacks up servers (3030,
	// 3031, ...) and a new browser tab each time. Only in registry mode and on a
	// fixed port — port 0 always wants a fresh OS-assigned instance.
	if *target == "" && *port != 0 {
		existing := fmt.Sprintf("http://127.0.0.1:%d", *port)
		if isLdgrViewerRunning(existing) {
			// Already running: do not spawn a duplicate and do not pop up another
			// browser tab. Just point at the existing instance and exit.
			fmt.Fprintf(stdout, "ldgr view already running on %s\n", existing)
			return 0
		}
	}

	ln, _, err := listenViewPort(*port)
	if err != nil {
		addr := fmt.Sprintf("127.0.0.1:%d", *port)
		fmt.Fprint(stderr, bindFailureMessage(addr, *port, err))
		return 1
	}
	addr := ln.Addr().String()
	url := fmt.Sprintf("http://%s", addr)
	fmt.Fprintf(stdout, "ldgr view listening on %s\n", url)
	if !*noOpen {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(stderr, "cannot open browser: %v\n", err)
		}
	}
	if err := http.Serve(ln, srv.Handler()); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// isLdgrViewerRunning reports whether an ldgr viewer is already serving at url.
// It fetches the root page and looks for the viewer's title marker, so a foreign
// process holding the port does not get mistaken for our viewer.
func isLdgrViewerRunning(url string) bool {
	client := &http.Client{Timeout: 600 * time.Millisecond}
	resp, err := client.Get(url + "/")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false
	}
	return strings.Contains(string(body), "<title>ldgr</title>")
}

func listenViewPort(start int) (net.Listener, int, error) {
	if start == 0 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, 0, err
		}
		return ln, ln.Addr().(*net.TCPAddr).Port, nil
	}
	var lastErr error
	for port := start; port < start+100 && port <= 65535; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
		if !isAddressInUse(err) {
			return nil, port, err
		}
	}
	return nil, start, lastErr
}

func bindFailureMessage(addr string, port int, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "cannot bind %s: %v\n", addr, err)
	if !isAddressInUse(err) {
		return b.String()
	}
	fmt.Fprintf(&b, "port %d is already in use. An ldgr viewer may already be running at http://%s\n", port, addr)
	if owner := listenOwner(addr); owner != "" {
		fmt.Fprintf(&b, "listener: %s\n", owner)
	}
	fmt.Fprintf(&b, "open the existing viewer, stop the listener, or choose another port: ldgr view --port %d\n", port+1)
	return b.String()
}

func isAddressInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE) || strings.Contains(err.Error(), "address already in use")
}

func listenOwner(addr string) string {
	if _, err := exec.LookPath("lsof"); err != nil {
		return ""
	}
	out, err := exec.Command("lsof", "-nP", "-iTCP@"+addr, "-sTCP:LISTEN", "-FpPc").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	pid := ""
	command := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "p") && len(line) > 1 {
			pid = line[1:]
		}
		if strings.HasPrefix(line, "c") && len(line) > 1 {
			command = line[1:]
		}
	}
	if pid == "" {
		return ""
	}
	if command == "" {
		return "pid " + pid
	}
	return fmt.Sprintf("pid %s (%s)", pid, command)
}

func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{url}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		name = "xdg-open"
		args = []string{url}
	}
	return exec.Command(name, args...).Start()
}
