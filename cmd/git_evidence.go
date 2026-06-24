package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hgwk/ldgr/internal/config"
)

func warnMissingGitCompletionEvidence(row map[string]any, stderr io.Writer) {
	if rowStatus(row) != "done" || hasGitCompletionEvidence(row) {
		return
	}
	id := rowID(row)
	if id == "" {
		id = "ticket"
	}
	fmt.Fprintf(stderr, "warning: %s is done without Git evidence; add evidence commit:<sha>, pr:<url-or-number>, or no_commit:<reason>\n", id)
}

func enforceGitCompletionEvidence(dir string, row map[string]any) error {
	if rowStatus(row) != "done" {
		return nil
	}
	cfg, err := config.Load(filepath.Join(dir, "ledger", "config.json"))
	if err != nil || strings.ToLower(strings.TrimSpace(cfg.GitEvidence)) != "fail" {
		return nil
	}
	if hasCommittedGitCompletionEvidence(row) {
		return nil
	}
	id := rowID(row)
	if id == "" {
		id = "ticket"
	}
	return fmt.Errorf("%s is done without committed Git evidence; add evidence commit:<sha> or pr:<url-or-number>, or set ledger/config.json git_evidence to warn/off", id)
}

func hasGitCompletionEvidence(row map[string]any) bool {
	for _, evidence := range stringListField(row, "evidence") {
		if isGitCompletionEvidence(evidence) {
			return true
		}
	}
	return false
}

func isGitCompletionEvidence(evidence string) bool {
	v := strings.TrimSpace(strings.ToLower(evidence))
	if strings.HasPrefix(v, "commit:") || strings.HasPrefix(v, "pr:") || strings.HasPrefix(v, "no_commit:") {
		i := strings.Index(v, ":")
		return i >= 0 && strings.TrimSpace(v[i+1:]) != ""
	}
	return strings.HasPrefix(v, "https://github.com/") && strings.Contains(v, "/pull/")
}

func hasCommittedGitCompletionEvidence(row map[string]any) bool {
	for _, evidence := range stringListField(row, "evidence") {
		if isCommittedGitCompletionEvidence(evidence) {
			return true
		}
	}
	return false
}

func isCommittedGitCompletionEvidence(evidence string) bool {
	v := strings.TrimSpace(strings.ToLower(evidence))
	if strings.HasPrefix(v, "commit:") || strings.HasPrefix(v, "pr:") {
		i := strings.Index(v, ":")
		return i >= 0 && strings.TrimSpace(v[i+1:]) != ""
	}
	return strings.HasPrefix(v, "https://github.com/") && strings.Contains(v, "/pull/")
}

func stringListField(row map[string]any, key string) []string {
	arr, _ := row[key].([]any)
	out := []string{}
	for _, raw := range arr {
		s, _ := raw.(string)
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func rowID(row map[string]any) string {
	if id, _ := row["id"].(string); id != "" {
		return id
	}
	id, _ := row["ticket"].(string)
	return id
}

func rowStatus(row map[string]any) string {
	if state, _ := row["state"].(string); state != "" {
		return state
	}
	status, _ := row["status"].(string)
	return status
}
