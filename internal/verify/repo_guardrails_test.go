package verify

import (
	"os/exec"
	"testing"
)

func TestVerify_WarnsOnDirtyGeneratedAndSecretFiles(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSONState(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": "",
		"dist/app.js":          "console.log('generated')\n",
		".env.local":           "TOKEN=secret\n",
	})
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	report, _ := Run(dir)
	if !hasWarnCode(report, "GENERATED_ARTIFACT_DIRTY") {
		t.Fatalf("expected GENERATED_ARTIFACT_DIRTY warn, got %+v", report.Warns)
	}
	if !hasWarnCode(report, "SECRET_FILE_DIRTY") {
		t.Fatalf("expected SECRET_FILE_DIRTY warn, got %+v", report.Warns)
	}
}
