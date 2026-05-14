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
