package legacy

import (
	"regexp"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

var isoRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)

// NormalizeTickets walks the input rows, returns normalized rows + counts +
// human-readable warnings. parents is config.Parents (used for prefix inference).
func NormalizeTickets(in []ledger.Row, parents []string, now string) ([]ledger.Row, Counts, []string) {
	allowed := setOf(parents)
	out := make([]ledger.Row, 0, len(in))
	var counts Counts
	var warns []string
	prevTS := ""
	for i, raw := range in {
		r := copyRow(raw)
		// n consecutive from 1
		want := i + 1
		got, _ := r["n"].(float64)
		if int(got) != want {
			counts.NReassigned++
		}
		r["n"] = want
		// ts ISO + non-decreasing
		ts, _ := r["ts"].(string)
		if !isoRe.MatchString(ts) || (prevTS != "" && legacyCompareTS(ts, prevTS) < 0) {
			r["ts"] = now
			counts.TSReplaced++
			warns = append(warns, "ts replaced on legacy ticket row")
		}
		prevTS, _ = r["ts"].(string)
		// agent default
		if a, _ := r["agent"].(string); a == "" {
			r["agent"] = "legacy"
			counts.AgentDefaulted++
		}
		// parent inference
		if p, _ := r["parent_ticket"].(string); p == "" {
			id, _ := r["ticket"].(string)
			inferred := inferParent(id, allowed)
			r["parent_ticket"] = inferred
			counts.ParentInferred++
		}
		// branch missing → ""
		if _, ok := r["branch"]; !ok {
			r["branch"] = ""
		}
		// ghost detection (post-normalization)
		if isGhostTicket(r) {
			counts.GhostTickets++
		}
		out = append(out, r)
	}
	return out, counts, warns
}

// NormalizeWorklog mirrors NormalizeTickets but for worklog rows. ticket is optional.
func NormalizeWorklog(in []ledger.Row, now string) ([]ledger.Row, Counts, []string) {
	out := make([]ledger.Row, 0, len(in))
	var counts Counts
	var warns []string
	prevTS := ""
	for i, raw := range in {
		r := copyRow(raw)
		want := i + 1
		got, _ := r["n"].(float64)
		if int(got) != want {
			counts.NReassigned++
		}
		r["n"] = want
		ts, _ := r["ts"].(string)
		if !isoRe.MatchString(ts) || (prevTS != "" && legacyCompareTS(ts, prevTS) < 0) {
			r["ts"] = now
			counts.TSReplaced++
			warns = append(warns, "ts replaced on legacy worklog row")
		}
		prevTS, _ = r["ts"].(string)
		if a, _ := r["agent"].(string); a == "" {
			r["agent"] = "legacy"
			counts.AgentDefaulted++
		}
		// Drop empty-string ticket field entirely (ticket is optional in worklog).
		if t, ok := r["ticket"].(string); ok && t == "" {
			delete(r, "ticket")
		}
		// Ensure required envelope present (empty strings allowed for branch/commit/notes).
		for _, f := range []string{"branch", "commit", "notes"} {
			if _, ok := r[f]; !ok {
				r[f] = ""
			}
		}
		if isGhostWorklog(r) {
			counts.GhostWorklog++
		}
		out = append(out, r)
	}
	return out, counts, warns
}

func inferParent(ticketID string, allowed map[string]struct{}) string {
	if i := strings.IndexByte(ticketID, '-'); i > 0 {
		prefix := ticketID[:i]
		if _, ok := allowed[prefix]; ok {
			return prefix
		}
	}
	return "LEGACY"
}

func isGhostTicket(r ledger.Row) bool {
	for _, f := range ledger.TicketNonEmpty {
		v, _ := r[f].(string)
		if v == "" {
			return true
		}
	}
	return false
}

func isGhostWorklog(r ledger.Row) bool {
	for _, f := range ledger.WorklogNonEmpty {
		v, _ := r[f].(string)
		if v == "" {
			return true
		}
	}
	return false
}

func setOf(in []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, v := range in {
		out[v] = struct{}{}
	}
	return out
}

func legacyCompareTS(a, b string) int {
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

func copyRow(in ledger.Row) ledger.Row {
	out := make(ledger.Row, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
