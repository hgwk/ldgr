package verify

import "strings"

func applyGitEvidencePolicy(rep *Report, policy string) {
	switch normalizeGitEvidencePolicy(policy) {
	case "off":
		rep.Warns = filterIssuesByCode(rep.Warns, "DONE_MISSING_GIT_EVIDENCE", false)
		rep.Fails = filterIssuesByCode(rep.Fails, "DONE_MISSING_GIT_EVIDENCE", false)
	case "fail":
		keptWarns := rep.Warns[:0]
		for _, issue := range rep.Warns {
			if issue.Code == "DONE_MISSING_GIT_EVIDENCE" {
				rep.Fails = append(rep.Fails, issue)
				continue
			}
			keptWarns = append(keptWarns, issue)
		}
		rep.Warns = keptWarns
	}
}

func normalizeGitEvidencePolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "off", "none", "ignore":
		return "off"
	case "fail", "error", "required":
		return "fail"
	default:
		return "warn"
	}
}

func filterIssuesByCode(in []Issue, code string, keepMatch bool) []Issue {
	out := in[:0]
	for _, issue := range in {
		if (issue.Code == code) == keepMatch {
			out = append(out, issue)
		}
	}
	return out
}
