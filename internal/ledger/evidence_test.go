package ledger

import "testing"

func TestEvidenceTestKind(t *testing.T) {
	cases := []struct {
		in   string
		kind string
		ok   bool
	}{
		{"test:browser: drawer closes", "browser", true},
		{"test:not_run: no browser", "not_run", true},
		{"go test ./...", "unit", true},
		{"cargo clippy --all-targets", "lint", true},
		{"commit:abc123", "", false},
		{"ok", "", false},
	}
	for _, tc := range cases {
		got, ok := EvidenceTestKind(tc.in)
		if got != tc.kind || ok != tc.ok {
			t.Fatalf("EvidenceTestKind(%q)=(%q,%v), want (%q,%v)", tc.in, got, ok, tc.kind, tc.ok)
		}
	}
}

func TestHasTestEvidence(t *testing.T) {
	if !HasTestEvidence([]any{"commit:abc123", "test:smoke: cli"}) {
		t.Fatal("expected smoke evidence to count as test evidence")
	}
	if HasTestEvidence([]any{"test:not_run: unavailable", "commit:abc123"}) {
		t.Fatal("not_run should not count as passing test evidence")
	}
	if !HasAnyTestEvidence([]any{"test:not_run: unavailable"}) {
		t.Fatal("not_run should count as review limitation evidence")
	}
}
