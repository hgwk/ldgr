package guidance

import (
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func ticket(fields map[string]any) ledger.Row {
	base := ledger.Row{
		"ticket": "T-1", "status": "open", "task": "demo task",
		"parent_ticket": "ROOT", "category": "feature",
		"paths": []any{"src/x.go"}, "blocked_by": []any{},
	}
	for k, v := range fields {
		base[k] = v
	}
	return base
}

func TestCompute_OpenSuggestsInProgressEvent(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "open"}), nil)
	if g.Status != "open" {
		t.Fatalf("status=%s", g.Status)
	}
	if len(g.SuggestedJSON) == 0 {
		t.Fatalf("no skeleton")
	}
	skel := g.SuggestedJSON[0].(map[string]any)
	if skel["status"] != "in_progress" {
		t.Fatalf("expected in_progress skeleton, got %v", skel["status"])
	}
	if !containsAny(g.SuggestedCommands, "ticket event") {
		t.Fatalf("expected ticket event command, got %v", g.SuggestedCommands)
	}
}

func TestCompute_InProgressSuggestsAuditReady(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "in_progress"}), nil)
	if g.Status != "in_progress" {
		t.Fatalf("status=%s", g.Status)
	}
	skel := g.SuggestedJSON[0].(map[string]any)
	if skel["status"] != "audit_ready" {
		t.Fatalf("expected audit_ready skeleton, got %v", skel["status"])
	}
}

func TestCompute_BlockedListsUnresolved(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "blocked", "blocked_by": []any{"DEP-1", "DEP-2"}}), nil)
	if g.Status != "blocked" {
		t.Fatalf("status=%s", g.Status)
	}
	if len(g.Warnings) == 0 {
		t.Fatalf("expected warnings, got none")
	}
	joined := strings.Join(g.Warnings, " ")
	if !strings.Contains(joined, "DEP-1") || !strings.Contains(joined, "DEP-2") {
		t.Fatalf("warnings should name each blocker: %v", g.Warnings)
	}
}

func TestCompute_AuditReadyForbidsWorklog(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "audit_ready"}), nil)
	if g.Status != "audit_ready" {
		t.Fatalf("status=%s", g.Status)
	}
	if !containsAny(g.Actions, "audit") || !containsAny(g.Actions, "worklog") {
		t.Fatalf("audit_ready actions must mention audit and worklog rules: %v", g.Actions)
	}
	if len(g.SuggestedJSON) < 2 {
		t.Fatalf("expected pass + changes_requested skeletons, got %d", len(g.SuggestedJSON))
	}
}

func TestCompute_ChangesRequestedResumes(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "changes_requested"}), nil)
	skel := g.SuggestedJSON[0].(map[string]any)
	if skel["status"] != "in_progress" {
		t.Fatalf("expected resume skeleton in_progress, got %v", skel["status"])
	}
}

func TestCompute_DoneAuditPassPromotesWorklog(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "done", "audit_result": "pass"}), nil)
	if !containsAny(g.SuggestedCommands, "suggest worklog") {
		t.Fatalf("audit-pass done should suggest worklog command: %v", g.SuggestedCommands)
	}
	if !containsAny(g.SuggestedCommands, "suggest commit") {
		t.Fatalf("audit-pass done should suggest commit command: %v", g.SuggestedCommands)
	}
	if len(g.Warnings) != 0 {
		t.Fatalf("audit-pass done should have no warnings, got %v", g.Warnings)
	}
}

func TestCompute_DoneWithoutAuditWarns(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "done"}), nil)
	if len(g.Warnings) == 0 {
		t.Fatalf("expected warning about weak closure, got none")
	}
}

func TestCompute_CancelledTerminal(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "cancelled"}), nil)
	if g.Status != "cancelled" {
		t.Fatalf("status=%s", g.Status)
	}
	if len(g.SuggestedJSON) != 0 {
		t.Fatalf("cancelled should not propose skeletons, got %v", g.SuggestedJSON)
	}
}

func TestRenderText_IncludesActions(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "audit_ready"}), nil)
	text := RenderText(g)
	if !strings.Contains(text, "audit_ready") || !strings.Contains(text, "Next:") {
		t.Fatalf("rendered text missing pieces:\n%s", text)
	}
}

func TestRenderJSON_RoundTrip(t *testing.T) {
	g := Compute(ticket(map[string]any{"status": "in_progress"}), nil)
	data, err := RenderJSON(g)
	if err != nil {
		t.Fatalf("render json: %v", err)
	}
	if !strings.Contains(string(data), `"status": "in_progress"`) && !strings.Contains(string(data), `"status":"in_progress"`) {
		t.Fatalf("json missing status field:\n%s", data)
	}
}

func containsAny(xs []string, needle string) bool {
	for _, x := range xs {
		if strings.Contains(x, needle) {
			return true
		}
	}
	return false
}
