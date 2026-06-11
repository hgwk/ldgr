package verify

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func compareTS(a, b string) int {
	at, aerr := time.Parse(time.RFC3339Nano, a)
	bt, berr := time.Parse(time.RFC3339Nano, b)
	if aerr == nil && berr == nil {
		if at.Before(bt) {
			return -1
		}
		if at.After(bt) {
			return 1
		}
		return 0
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func stringField(r ledger.Row, key string) string {
	v, _ := r[key].(string)
	return v
}

func rowID(r ledger.Row, stateMode bool) string {
	if stateMode {
		return stringField(r, "id")
	}
	return stringField(r, "ticket")
}

func rowStatus(r ledger.Row, stateMode bool) string {
	if stateMode {
		return stringField(r, "state")
	}
	return stringField(r, "status")
}

func latestTicketRows(rows []ledger.Row, stateMode bool) map[string]ledger.Row {
	out := map[string]ledger.Row{}
	for _, r := range rows {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		id := rowID(r, stateMode)
		if id == "" {
			continue
		}
		n, _ := numberAsInt(r["n"])
		if cur, ok := out[id]; ok {
			cn, _ := numberAsInt(cur["n"])
			if n <= cn {
				continue
			}
		}
		out[id] = r
	}
	return out
}

func stringSliceField(r ledger.Row, key string) []string {
	arr, _ := r[key].([]any)
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

func isActiveClaimState(status string, stateMode bool) bool {
	if stateMode {
		switch status {
		case "ready", "doing", "blocked", "review", "rework":
			return true
		default:
			return false
		}
	}
	switch status {
	case "open", "planned", "claimed", "in_progress", "blocked", "audit_ready", "changes_requested", "review_ready":
		return true
	default:
		return false
	}
}

func normalizeClaimPath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." || path == "/" {
		return ""
	}
	return path
}

func hasNonEmptyEvidence(r ledger.Row) bool {
	arr, _ := r["evidence"].([]any)
	for _, v := range arr {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

func hasUsefulEvidence(r ledger.Row) bool {
	for _, evidence := range stringSliceField(r, "evidence") {
		if isUsefulEvidence(evidence) {
			return true
		}
	}
	return false
}

func isUsefulEvidence(evidence string) bool {
	v := strings.TrimSpace(strings.ToLower(evidence))
	switch v {
	case "", "x", "ok", "done", "test", "tested", "verify", "verified", "yes", "n/a", "na":
		return false
	}
	if len(v) >= 8 {
		return true
	}
	return strings.ContainsAny(v, " ./:-_")
}

func hasGitCompletionEvidence(r ledger.Row) bool {
	for _, evidence := range stringSliceField(r, "evidence") {
		if isGitCompletionEvidence(evidence) {
			return true
		}
	}
	return false
}

func isGitCompletionEvidence(evidence string) bool {
	v := strings.TrimSpace(strings.ToLower(evidence))
	if strings.HasPrefix(v, "commit:") || strings.HasPrefix(v, "pr:") || strings.HasPrefix(v, "no_commit:") {
		return len(strings.TrimSpace(v[strings.Index(v, ":")+1:])) > 0
	}
	return strings.HasPrefix(v, "https://github.com/") && strings.Contains(v, "/pull/")
}

func handoffText(r ledger.Row) (string, bool) {
	parts := []string{}
	for _, key := range []string{"handoff", "handoff_to", "notes", "audit_notes"} {
		if v := stringField(r, key); v != "" {
			parts = append(parts, v)
		}
	}
	if event, ok := r["event"].(map[string]any); ok {
		for _, key := range []string{"summary", "notes"} {
			if v, _ := event[key].(string); strings.TrimSpace(v) != "" {
				parts = append(parts, v)
			}
		}
	}
	text := strings.Join(parts, "\n")
	if !containsAny(strings.ToLower(text), "handoff", "handover", "인계") {
		return "", false
	}
	return text, true
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func porcelainPath(line string) string {
	if len(line) < 4 {
		return ""
	}
	path := strings.TrimSpace(line[3:])
	if idx := strings.LastIndex(path, " -> "); idx >= 0 {
		path = path[idx+4:]
	}
	return strings.Trim(path, `"`)
}

func isGeneratedOrBuildArtifact(path string) bool {
	p := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(p)
	if strings.HasSuffix(base, ".tsbuildinfo") {
		return true
	}
	if strings.Contains(base, "generated") || strings.Contains(base, ".gen.") {
		return true
	}
	for _, marker := range []string{
		"dist/", "build/", "coverage/", ".next/", "out/", "target/", "node_modules/",
	} {
		if strings.HasPrefix(p, marker) || strings.Contains(p, "/"+marker) {
			return true
		}
	}
	return false
}

func isSecretLikePath(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))
	switch base {
	case ".env.example", ".env.sample", ".env.template":
		return false
	case ".env", "id_rsa", "id_ed25519":
		return true
	}
	if strings.HasPrefix(base, ".env.") {
		return true
	}
	if strings.Contains(base, "secret") || strings.Contains(base, "private_key") {
		return true
	}
	for _, suffix := range []string{".pem", ".key", ".p12", ".pfx"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

func hasPositiveNumber(r ledger.Row, key string) bool {
	_, ok := numberAsInt(r[key])
	return ok
}

func numberAsInt(v any) (int, bool) {
	switch v := v.(type) {
	case float64:
		if v > 0 && v == float64(int(v)) {
			return int(v), true
		}
	case int:
		if v > 0 {
			return v, true
		}
	}
	return 0, false
}
