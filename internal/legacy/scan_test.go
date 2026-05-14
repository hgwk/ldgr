package legacy

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestScan_DetectsAllKinds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"x","task":"t","status":"open","ts":"2026-05-14T10:00:00Z","parent_ticket":"ROOT","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	writeFile(t, dir, "agent-worklog.jsonl", `{"n":1,"ticket":"x","task":"t","scope":"repo","result":"r","ts":"2026-05-14T10:00:00Z","agent":"codex","paths":[],"commands":[],"notes":"","branch":"","commit":""}`+"\n")
	writeFile(t, dir, "goal.json", `{"schema_version":1,"summary":"hi"}`)

	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	exists := map[SourceKind]bool{}
	for _, s := range srcs {
		if s.Exists {
			exists[s.Kind] = true
		}
	}
	for _, k := range []SourceKind{SourceLegacyTickets, SourceLegacyWorklog, SourceLegacyGoal} {
		if !exists[k] {
			t.Fatalf("expected kind %v to be detected", k)
		}
	}
}

func TestScan_MissingFilesAreFine(t *testing.T) {
	dir := t.TempDir()
	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, s := range srcs {
		if s.Exists {
			t.Fatalf("nothing should exist in empty dir, got %v", s)
		}
	}
}

func TestScan_PreservesParseErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"x","task":"t"}
not json
{"n":3,"ticket":"y","task":"t"}
`)
	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var src Source
	for _, s := range srcs {
		if s.Kind == SourceLegacyTickets {
			src = s
		}
	}
	if !src.Exists {
		t.Fatalf("tickets source should exist")
	}
	if len(src.Rows) != 2 {
		t.Fatalf("expected 2 good rows, got %d", len(src.Rows))
	}
	if len(src.ParseErrs) != 1 || src.ParseErrs[0].Line != 2 {
		t.Fatalf("expected one parse error on line 2, got %+v", src.ParseErrs)
	}
}

func TestScan_DetectsCurrentLedger(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ledger/tickets.jsonl", `{"n":1,"ticket":"x"}`+"\n")
	writeFile(t, dir, "ledger/goal.json", `{"schema_version":1}`)
	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	got := map[SourceKind]bool{}
	for _, s := range srcs {
		if s.Exists {
			got[s.Kind] = true
		}
	}
	if !got[SourceCurrentTickets] || !got[SourceCurrentGoal] {
		t.Fatalf("expected current ledger files detected, got %v", got)
	}
}
