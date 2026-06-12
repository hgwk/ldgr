package cmd

import (
	"net"
	"testing"
)

func TestListenViewPort_ChoosesNextPortWhenRequestedPortIsBusy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	next, selected, err := listenViewPort(port)
	if err != nil {
		t.Fatalf("listenViewPort: %v", err)
	}
	defer next.Close()
	if selected == port {
		t.Fatalf("selected busy port %d", port)
	}
}

func TestListenViewPort_PortZeroUsesOSAssignedPort(t *testing.T) {
	ln, selected, err := listenViewPort(0)
	if err != nil {
		t.Fatalf("listenViewPort: %v", err)
	}
	defer ln.Close()
	if selected <= 0 {
		t.Fatalf("selected invalid port %d", selected)
	}
}
