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

func TestDispatch_Help(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}} {
		var stdout, stderr bytes.Buffer
		code := Dispatch(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("Dispatch(%v) exit = %d, stderr = %q", args, code, stderr.String())
		}
		if got := stdout.String(); !strings.Contains(got, "Subcommands:") || !strings.Contains(got, "view") {
			t.Fatalf("Dispatch(%v) help output incomplete: %q", args, got)
		}
	}
}

func TestDispatch_Version(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-V"}} {
		var stdout, stderr bytes.Buffer
		code := Dispatch(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("Dispatch(%v) exit = %d, stderr = %q", args, code, stderr.String())
		}
		if got, want := stdout.String(), "ldgr "+Version+"\n"; got != want {
			t.Fatalf("Dispatch(%v) stdout = %q, want %q", args, got, want)
		}
	}
}
