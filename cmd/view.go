package cmd

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"

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
	fmt.Fprintf(stdout, "ldgr view listening on http://%s\n", addr)
	if err := http.Serve(ln, srv.Handler()); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
