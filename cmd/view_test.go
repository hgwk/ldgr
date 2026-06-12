package cmd

import (
	"bytes"
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestView_PortInUsePrintsActionableHint(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	var stderr bytes.Buffer
	code := RunViewCLI([]string{"--port", strconv.Itoa(port), "--no-open"}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected bind failure")
	}
	msg := stderr.String()
	for _, want := range []string{"already in use", "http://127.0.0.1:", "ldgr view --port"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("stderr missing %q: %s", want, msg)
		}
	}
}
