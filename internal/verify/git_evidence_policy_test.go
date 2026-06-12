package verify

import "testing"

func TestGitEvidencePolicyFailPromotesWarning(t *testing.T) {
	rep := Report{Warns: []Issue{
		{Code: "DONE_MISSING_GIT_EVIDENCE"},
		{Code: "OTHER"},
	}}
	applyGitEvidencePolicy(&rep, "fail")
	if !hasFailCode(rep, "DONE_MISSING_GIT_EVIDENCE") {
		t.Fatalf("expected git evidence warning promoted to fail: %+v", rep)
	}
	if !hasWarnCode(rep, "OTHER") || hasWarnCode(rep, "DONE_MISSING_GIT_EVIDENCE") {
		t.Fatalf("unexpected remaining warnings: %+v", rep.Warns)
	}
}

func TestGitEvidencePolicyOffRemovesIssues(t *testing.T) {
	rep := Report{
		Warns: []Issue{{Code: "DONE_MISSING_GIT_EVIDENCE"}, {Code: "OTHER"}},
		Fails: []Issue{{Code: "DONE_MISSING_GIT_EVIDENCE"}, {Code: "FAIL"}},
	}
	applyGitEvidencePolicy(&rep, "off")
	if hasWarnCode(rep, "DONE_MISSING_GIT_EVIDENCE") || hasFailCode(rep, "DONE_MISSING_GIT_EVIDENCE") {
		t.Fatalf("expected git evidence issues removed: %+v", rep)
	}
	if !hasWarnCode(rep, "OTHER") || !hasFailCode(rep, "FAIL") {
		t.Fatalf("expected unrelated issues preserved: %+v", rep)
	}
}
