package cmd

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/hgwk/ldgr/internal/viewer"
)

func init() {
	Commands["view"] = RunViewCLI
}

// RunViewCLI starts the read-only viewer on localhost.
func RunViewCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("view")
	port := fs.Int("port", 3030, "")
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

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "cannot bind %s: %v\n", addr, err)
		return 1
	}
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
