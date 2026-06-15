package viewer

import (
	"net/http"
	"sort"

	"github.com/hgwk/ldgr/internal/ledger"
)

func (s *Server) handleTicketDetail(w http.ResponseWriter, r *http.Request, proj Project, ticketID string) {
	// Collect all non-companion rows for this ticket, oldest first.
	history := make([]ledger.Row, 0)
	stateMode := usesStateTicketRows(proj.Tickets)
	for _, row := range proj.Tickets {
		if stateMode {
			if id, _ := row["id"].(string); id != ticketID {
				continue
			}
		} else {
			if id, _ := row["ticket"].(string); id != ticketID {
				continue
			}
		}
		if _, isCompanion := row["invalidates_n"]; isCompanion {
			continue
		}
		history = append(history, row)
	}
	if len(history) == 0 {
		http.Error(w, "ticket not found", http.StatusNotFound)
		return
	}
	sort.SliceStable(history, func(i, j int) bool {
		ai, _ := history[i]["n"].(float64)
		bj, _ := history[j]["n"].(float64)
		return ai < bj
	})
	latest := history[len(history)-1]

	// Worklog rows for this ticket, newest first.
	var wl []ledger.Row
	wInvalidated := InvalidatedNs(proj.Worklog)
	for _, w := range proj.Worklog {
		if _, isCompanion := w["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := w["n"].(float64)
		if _, inv := wInvalidated[int(n)]; inv {
			continue
		}
		if id, _ := w["ticket"].(string); id == ticketID {
			wl = append(wl, w)
		}
	}
	sort.SliceStable(wl, func(i, j int) bool {
		a, _ := wl[i]["ts"].(string)
		b, _ := wl[j]["ts"].(string)
		return a > b
	})

	// If the latest row n is invalidated, surface the via_n.
	var invalidatedVia any = nil
	inv := InvalidatedNs(proj.Tickets)
	if n, _ := latest["n"].(float64); inv[int(n)] > 0 {
		invalidatedVia = inv[int(n)]
	}

	writeJSON(w, map[string]any{
		"ticket":          ticketID,
		"latest":          latest,
		"history":         history,
		"worklog":         wl,
		"invalidated_via": invalidatedVia,
	})
}

func activeTicketCount(counts map[string]int) int {
	total := 0
	for state, count := range counts {
		if isActiveState(state) {
			total += count
		}
	}
	return total
}

func closedTicketCount(counts map[string]int) int {
	total := 0
	for state, count := range counts {
		if isTerminalState(state) {
			total += count
		}
	}
	return total
}

func visibleWorklog(rows []ledger.Row) []ledger.Row {
	invalid := InvalidatedNs(rows)
	out := make([]ledger.Row, 0, len(rows))
	for _, r := range rows {
		n, _ := r["n"].(float64)
		if _, isInvalid := invalid[int(n)]; isInvalid {
			continue
		}
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		out = append(out, r)
	}
	return out
}

func recentWorklogTS(rows []ledger.Row) string {
	var best string
	for _, r := range rows {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		ts, _ := r["ts"].(string)
		if ts > best {
			best = ts
		}
	}
	return best
}

func recentProjectActivityTS(ticketRows, worklogRows []ledger.Row) string {
	best := recentWorklogTS(worklogRows)
	invalidated := InvalidatedNs(ticketRows)
	for _, r := range ticketRows {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := r["n"].(float64)
		if _, isInvalid := invalidated[int(n)]; isInvalid {
			continue
		}
		ts, _ := r["ts"].(string)
		if parseTS(ts).After(parseTS(best)) {
			best = ts
		}
	}
	return best
}

func usesStateTicketRows(rows []ledger.Row) bool {
	for _, row := range rows {
		if _, ok := row["id"]; ok {
			return true
		}
		if _, ok := row["state"]; ok {
			return true
		}
		if _, ok := row["event"]; ok {
			return true
		}
	}
	return false
}
