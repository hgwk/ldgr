package verify

import (
	"fmt"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func checkStateBlockers(rep *Report, tickets []ledger.Row) {
	latest := latestStateTickets(tickets)
	for id, t := range latest {
		state, _ := t["state"].(string)
		if state == "done" || state == "dropped" {
			continue
		}
		line, _ := numberAsInt(t["n"])
		blockers := stringSliceField(t, "blocked_by")
		if state == "blocked" && len(blockers) == 0 {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "BLOCKED_NO_BLOCKERS", Message: fmt.Sprintf("[BLOCKED_NO_BLOCKERS] %s", id)})
			continue
		}
		unresolved := 0
		for _, blockerID := range blockers {
			blocker := latest[blockerID]
			blockerState, _ := blocker["state"].(string)
			if blockerState == "done" || blockerState == "dropped" {
				rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "STALE_BLOCKER", Message: fmt.Sprintf("[STALE_BLOCKER] %s: %s is closed", id, blockerID)})
				continue
			}
			unresolved++
		}
		if state == "blocked" && unresolved == 0 {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "BLOCKED_NO_BLOCKERS", Message: fmt.Sprintf("[BLOCKED_NO_BLOCKERS] %s has no unresolved blockers", id)})
		}
	}
}

func checkWorklogCommands(rep *Report, rows []ledger.Row) {
	for _, r := range rows {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		if len(stringSliceField(r, "commands")) > 0 {
			continue
		}
		line, _ := numberAsInt(r["n"])
		rep.Warns = append(rep.Warns, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "WORKLOG_COMMANDS_EMPTY", Message: "[WORKLOG_COMMANDS_EMPTY] worklog row has no verification command"})
	}
}

func checkClaimPathConflicts(rep *Report, tickets []ledger.Row, stateMode bool) {
	latest := latestTicketRows(tickets, stateMode)
	type owner struct {
		id   string
		line int
	}
	claims := map[string]owner{}
	for id, r := range latest {
		if !isActiveClaimState(rowStatus(r, stateMode), stateMode) {
			continue
		}
		line, _ := numberAsInt(r["n"])
		for _, p := range stringSliceField(r, "paths") {
			path := normalizeClaimPath(p)
			if path == "" {
				continue
			}
			prev, ok := claims[path]
			if !ok {
				claims[path] = owner{id: id, line: line}
				continue
			}
			if prev.id == id {
				continue
			}
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/tickets.jsonl",
				Line:    line,
				Code:    "CLAIM_PATH_CONFLICT",
				Message: fmt.Sprintf("[CLAIM_PATH_CONFLICT] %s and %s both claim %s", prev.id, id, path),
			})
		}
	}
}

func checkReviewEvidenceQuality(rep *Report, tickets []ledger.Row, stateMode bool) {
	for _, r := range tickets {
		status := rowStatus(r, stateMode)
		if stateMode {
			if status != "review" {
				continue
			}
		} else if status != "audit_ready" {
			continue
		}
		if hasUsefulEvidence(r) {
			continue
		}
		line, _ := numberAsInt(r["n"])
		id := rowID(r, stateMode)
		rep.Warns = append(rep.Warns, Issue{
			File:    "ledger/tickets.jsonl",
			Line:    line,
			Code:    "REVIEW_EVIDENCE_WEAK",
			Message: fmt.Sprintf("[REVIEW_EVIDENCE_WEAK] %s review needs concrete evidence", id),
		})
	}
}

func checkReviewTestEvidence(rep *Report, tickets []ledger.Row, stateMode bool) {
	for _, r := range tickets {
		status := rowStatus(r, stateMode)
		if status != "review" && status != "done" && status != "audit_ready" {
			continue
		}
		evidence, _ := r["evidence"].([]any)
		if ledger.HasTestEvidence(evidence) {
			continue
		}
		line, _ := numberAsInt(r["n"])
		id := rowID(r, stateMode)
		code := "REVIEW_TEST_EVIDENCE_MISSING"
		if status == "done" {
			code = "DONE_TEST_EVIDENCE_MISSING"
		}
		rep.Warns = append(rep.Warns, Issue{
			File:    "ledger/tickets.jsonl",
			Line:    line,
			Code:    code,
			Message: fmt.Sprintf("[%s] %s %s row should include test evidence", code, id, status),
		})
	}
}

func checkSuccessCriteriaCoverage(rep *Report, tickets []ledger.Row, stateMode bool) {
	for _, r := range tickets {
		status := rowStatus(r, stateMode)
		if stateMode {
			if status != "review" && status != "done" {
				continue
			}
		} else if status != "audit_ready" && status != "review_ready" && status != "done" {
			continue
		}
		if len(stringSliceField(r, "acceptance")) > 0 || len(stringSliceField(r, "success_criteria")) > 0 {
			continue
		}
		if containsAny(strings.ToLower(strings.Join(rowTextParts(r), "\n")), "success criteria", "acceptance", "성공 기준", "완료 기준") {
			continue
		}
		line, _ := numberAsInt(r["n"])
		rep.Warns = append(rep.Warns, Issue{
			File:    "ledger/tickets.jsonl",
			Line:    line,
			Code:    "SUCCESS_CRITERIA_MISSING",
			Message: fmt.Sprintf("[SUCCESS_CRITERIA_MISSING] %s review/done row should name the success criteria it satisfies", rowID(r, stateMode)),
		})
	}
}

func checkDecisionContext(rep *Report, tickets []ledger.Row, stateMode bool) {
	for _, r := range tickets {
		status := rowStatus(r, stateMode)
		if status != "blocked" && status != "rework" && status != "changes_requested" {
			continue
		}
		text := strings.ToLower(strings.Join(rowTextParts(r), "\n"))
		if containsAny(text,
			"ambiguous_requirement", "missing_context", "conflicting_instruction", "verification_failed",
			"assumption", "tradeoff", "decision", "need", "blocked", "rework",
			"모호", "전제", "결정", "검증", "막힘",
		) {
			continue
		}
		line, _ := numberAsInt(r["n"])
		rep.Warns = append(rep.Warns, Issue{
			File:    "ledger/tickets.jsonl",
			Line:    line,
			Code:    "DECISION_CONTEXT_WEAK",
			Message: fmt.Sprintf("[DECISION_CONTEXT_WEAK] %s %s row should record the assumption, tradeoff, or blocker taxonomy", rowID(r, stateMode), status),
		})
	}
}

func checkHandoffShape(rep *Report, tickets []ledger.Row, stateMode bool) {
	for _, r := range tickets {
		text, ok := handoffText(r)
		if !ok {
			continue
		}
		lower := strings.ToLower(text)
		missing := []string{}
		if !containsAny(lower, "scope", "path", "paths", "write scope") {
			missing = append(missing, "scope")
		}
		if !containsAny(lower, "verify", "verification", "tested", "test", "commands", "검증") {
			missing = append(missing, "verification")
		}
		if !containsAny(lower, "risk", "risks", "blocker", "blocked", "decision", "위험") {
			missing = append(missing, "risk")
		}
		if len(missing) == 0 {
			continue
		}
		line, _ := numberAsInt(r["n"])
		id := rowID(r, stateMode)
		rep.Warns = append(rep.Warns, Issue{
			File:    "ledger/tickets.jsonl",
			Line:    line,
			Code:    "HANDOFF_INCOMPLETE",
			Message: fmt.Sprintf("[HANDOFF_INCOMPLETE] %s missing %s", id, strings.Join(missing, "/")),
		})
	}
}

func rowTextParts(r ledger.Row) []string {
	parts := []string{}
	for _, key := range []string{"task", "title", "notes", "decision", "audit_notes", "handoff", "handoff_to"} {
		if v := stringField(r, key); strings.TrimSpace(v) != "" {
			parts = append(parts, v)
		}
	}
	if event, ok := r["event"].(map[string]any); ok {
		for _, key := range []string{"summary", "notes", "result"} {
			if v, _ := event[key].(string); strings.TrimSpace(v) != "" {
				parts = append(parts, v)
			}
		}
	}
	return parts
}
