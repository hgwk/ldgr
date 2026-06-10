package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/ledger"
)

func checkRepoFileGuardrails(rep *Report, targetDir string) {
	out, err := exec.Command("git", "-C", targetDir, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		path := porcelainPath(line)
		if path == "" {
			continue
		}
		if isGeneratedOrBuildArtifact(path) {
			rep.Warns = append(rep.Warns, Issue{
				File:    path,
				Code:    "GENERATED_ARTIFACT_DIRTY",
				Message: "[GENERATED_ARTIFACT_DIRTY] generated/build artifact is dirty or untracked",
			})
		}
		if isSecretLikePath(path) {
			rep.Warns = append(rep.Warns, Issue{
				File:    path,
				Code:    "SECRET_FILE_DIRTY",
				Message: "[SECRET_FILE_DIRTY] env/secret-like file is dirty or untracked",
			})
		}
	}
}

func checkArchivedProjectActiveTickets(rep *Report, cfg config.Config, tickets []ledger.Row, stateMode bool) {
	status := strings.ToLower(strings.TrimSpace(cfg.Status))
	if status != "archived" && status != "closed" {
		return
	}
	for id, r := range latestTicketRows(tickets, stateMode) {
		if !isActiveClaimState(rowStatus(r, stateMode), stateMode) {
			continue
		}
		line, _ := numberAsInt(r["n"])
		rep.Warns = append(rep.Warns, Issue{
			File:    "ledger/tickets.jsonl",
			Line:    line,
			Code:    "ARCHIVED_PROJECT_ACTIVE_TICKET",
			Message: fmt.Sprintf("[ARCHIVED_PROJECT_ACTIVE_TICKET] project status=%s but %s is %s", status, id, rowStatus(r, stateMode)),
		})
	}
}

func checkLocalVerifierDrift(rep *Report, targetDir string) {
	data, err := os.ReadFile(filepath.Join(targetDir, "package.json"))
	if err != nil {
		return
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return
	}
	for name, script := range pkg.Scripts {
		lowerName := strings.ToLower(name)
		lowerScript := strings.ToLower(script)
		if !strings.Contains(lowerName, "ledger") && !strings.Contains(lowerName, "verify") {
			continue
		}
		if !strings.Contains(lowerScript, "ledger") || strings.Contains(lowerScript, "ldgr verify") {
			continue
		}
		rep.Warns = append(rep.Warns, Issue{
			File:    "package.json",
			Code:    "PROJECT_LOCAL_VERIFIER",
			Message: fmt.Sprintf("[PROJECT_LOCAL_VERIFIER] script %q uses project-local ledger verification; compare with `ldgr verify` before treating results as authoritative", name),
		})
	}
}
