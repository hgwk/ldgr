package ledger

import "strings"

// EvidenceTestKind returns the display kind for verification evidence.
func EvidenceTestKind(evidence string) (string, bool) {
	value := strings.TrimSpace(strings.ToLower(evidence))
	if value == "" {
		return "", false
	}
	if strings.HasPrefix(value, "test:") {
		rest := strings.TrimPrefix(value, "test:")
		kind, _, _ := strings.Cut(rest, ":")
		kind = strings.TrimSpace(kind)
		if kind == "" {
			return "", false
		}
		return kind, true
	}
	switch {
	case strings.Contains(value, "playwright") || strings.Contains(value, "browser"):
		return "browser", true
	case strings.Contains(value, "smoke"):
		return "smoke", true
	case strings.Contains(value, "e2e"):
		return "e2e", true
	case strings.Contains(value, "integration"):
		return "integration", true
	case strings.Contains(value, "typecheck") || strings.Contains(value, "tsc"):
		return "typecheck", true
	case strings.Contains(value, "lint") || strings.Contains(value, "clippy"):
		return "lint", true
	case strings.Contains(value, "go test") || strings.Contains(value, "cargo test") ||
		strings.Contains(value, "npm test") || strings.Contains(value, "pnpm test") ||
		strings.Contains(value, "yarn test") || strings.Contains(value, "bun test"):
		return "unit", true
	default:
		return "", false
	}
}

func HasTestEvidence(evidence []any) bool {
	for _, item := range evidence {
		text, ok := item.(string)
		if !ok {
			continue
		}
		kind, ok := EvidenceTestKind(text)
		if ok && kind != "not_run" {
			return true
		}
	}
	return false
}

func HasAnyTestEvidence(evidence []any) bool {
	for _, item := range evidence {
		text, ok := item.(string)
		if !ok {
			continue
		}
		if _, ok := EvidenceTestKind(text); ok {
			return true
		}
	}
	return false
}

func HasOnlyNotRunTestEvidence(evidence []any) bool {
	seen := false
	for _, item := range evidence {
		text, ok := item.(string)
		if !ok {
			continue
		}
		kind, ok := EvidenceTestKind(text)
		if !ok {
			continue
		}
		seen = true
		if kind != "not_run" {
			return false
		}
	}
	return seen
}
