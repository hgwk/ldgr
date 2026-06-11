package verify

import (
	"fmt"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func checkCommonEnvelope(rep *Report, file string, line int, r ledger.Row, prevTS *string) {
	if n, ok := r["n"].(float64); !ok || int(n) != line {
		rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "N_NOT_CONSECUTIVE", Message: fmt.Sprintf("n must equal %d, got %v", line, r["n"])})
	}
	ts, _ := r["ts"].(string)
	if !isoRe.MatchString(ts) {
		rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "TS_NOT_ISO", Message: "ts not ISO8601 UTC: " + ts})
	} else if *prevTS != "" && compareTS(ts, *prevTS) < 0 {
		rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: "TS_NOT_INCREASING", Message: "ts decreases relative to previous row"})
	}
	*prevTS = ts
}

func checkStringEnum(rep *Report, file string, line int, r map[string]any, key string, enum map[string]struct{}, code string) {
	v, _ := r[key].(string)
	if v == "" {
		return
	}
	if _, ok := enum[v]; !ok {
		rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: code, Message: "invalid " + key + ": " + v})
	}
}

func checkStringEnumWarn(rep *Report, file string, line int, r map[string]any, key string, enum map[string]struct{}, code string) {
	v, _ := r[key].(string)
	if v == "" {
		return
	}
	if _, ok := enum[v]; !ok {
		rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: code, Message: "unknown " + key + ": " + v})
	}
}

func checkStateEnum(rep *Report, file string, line int, r map[string]any) {
	state, _ := r["state"].(string)
	if state == "" {
		return
	}
	if _, ok := ledger.StateEnum[state]; ok {
		return
	}
	if isLegacyStateValue(state) {
		rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: "LEGACY_STATE_VALUE", Message: "legacy state value in state-model row: " + state})
		return
	}
	rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "INVALID_STATE", Message: "invalid state: " + state})
}

func isLegacyStateValue(state string) bool {
	if _, ok := ledger.StatusEnum[state]; ok {
		return true
	}
	switch state {
	case "open", "in_progress", "audit_ready", "changes_requested", "cancelled", "review_ready", "planned", "claimed":
		return true
	default:
		return false
	}
}

func isSoftEventField(field string) bool {
	return field == "summary" || field == "notes"
}

func checkStateAuditRule(rep *Report, line int, r ledger.Row, event map[string]any) {
	state, _ := r["state"].(string)
	switch state {
	case "done":
		if !stateAuditRoleIsPass(r, event) {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "WEAK_DONE", Message: "state=done should be an auditor/audit pass row"})
		}
		if !stateAuditResultIs(r, event, "pass") {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "WEAK_DONE", Message: "state=done should record pass result"})
		}
		if !hasReviewedN(r, event) {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "WEAK_DONE", Message: "state=done should record reviewed_n"})
		}
		if !hasNonEmptyEvidence(r) {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "WEAK_DONE", Message: "state=done should record non-empty evidence"})
		}
		if !hasGitCompletionEvidence(r) {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "DONE_MISSING_GIT_EVIDENCE", Message: "state=done should record commit:<sha>, pr:<url-or-number>, or no_commit:<reason> evidence"})
		}
	case "rework":
		if !stateAuditRoleIsPass(r, event) {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "REWORK_WEAK", Message: "state=rework should be an auditor/audit row"})
		}
		if !stateAuditResultIs(r, event, "changes_requested") {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "REWORK_WEAK", Message: "state=rework should record changes_requested result"})
		}
		if !hasReviewedN(r, event) {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "REWORK_WEAK", Message: "state=rework should record reviewed_n"})
		}
		if notes, _ := event["notes"].(string); strings.TrimSpace(notes) == "" {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "REWORK_WEAK", Message: "state=rework should record event.notes"})
		}
	}
}

func stateAuditRoleIsPass(r ledger.Row, event map[string]any) bool {
	role, _ := event["role"].(string)
	if role == "auditor" || role == "reviewer" || role == "audit" {
		return true
	}
	topRole := stringField(r, "role")
	return topRole == "audit" || topRole == "auditor" || topRole == "reviewer"
}

func stateAuditResultIs(r ledger.Row, event map[string]any, want string) bool {
	result, _ := event["result"].(string)
	if result == want {
		return true
	}
	auditResult := stringField(r, "audit_result")
	return auditResult == want
}

func hasReviewedN(r ledger.Row, event map[string]any) bool {
	if _, ok := numberAsInt(event["reviewed_n"]); ok {
		return true
	}
	if _, ok := numberAsInt(r["reviewed_n"]); ok {
		return true
	}
	if _, ok := numberAsInt(event["reviewed_worklog_n"]); ok {
		return true
	}
	if _, ok := numberAsInt(r["reviewed_worklog_n"]); ok {
		return true
	}
	return false
}

func latestStateTickets(rows []ledger.Row) map[string]ledger.Row {
	out := map[string]ledger.Row{}
	for _, r := range rows {
		id, _ := r["id"].(string)
		if id == "" {
			continue
		}
		n, _ := r["n"].(float64)
		if cur, ok := out[id]; ok {
			cn, _ := cur["n"].(float64)
			if n <= cn {
				continue
			}
		}
		out[id] = r
	}
	return out
}
