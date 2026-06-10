package viewer

import (
	"sort"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func perTicketHistory(rows []ledger.Row) [][]ledger.Row {
	invalidated := InvalidatedNs(rows)
	byID := map[string][]ledger.Row{}
	order := []string{}
	for _, r := range rows {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := r["n"].(float64)
		if _, isGhost := invalidated[int(n)]; isGhost {
			continue
		}
		id := ticketID(r)
		if id == "" {
			continue
		}
		if _, seen := byID[id]; !seen {
			order = append(order, id)
		}
		byID[id] = append(byID[id], r)
	}
	out := make([][]ledger.Row, 0, len(order))
	for _, id := range order {
		hist := byID[id]
		sort.SliceStable(hist, func(i, j int) bool {
			a, _ := hist[i]["n"].(float64)
			b, _ := hist[j]["n"].(float64)
			return a < b
		})
		out = append(out, hist)
	}
	return out
}

// isAuditPassDone returns true for the strong-done predicate:
// legacy: role == "audit" AND status == "done" AND audit_result == "pass".
// state-model: state == "done" AND event.role == "auditor" AND event.result == "pass".
func isAuditPassDone(r ledger.Row) bool {
	role, _ := r["role"].(string)
	status := ticketState(r)
	result, _ := r["audit_result"].(string)
	if role == "audit" && status == "done" && result == "pass" {
		return true
	}
	return status == "done" && eventString(r, "role") == "auditor" && eventString(r, "result") == "pass"
}

// ComputeLifecycleLatency derives per-ticket cycle time (first active row to
// audit-pass done) and audit latency (latest audit_ready row to audit-pass
// done, or to now if still pending). Cancelled tickets are excluded from both
// completed and pending stats. Each input slice is one ticket's full history,
// sorted ascending by n/ts.
//
// Median uses the lower-of-two value for even sample counts. P90 uses
// nearest-rank: index = ceil(0.9 * n) - 1 after sort.
func ComputeLifecycleLatency(tickets [][]ledger.Row, now time.Time) LifecycleLatency {
	var cycleHours, auditHours []float64
	for _, hist := range tickets {
		if len(hist) == 0 {
			continue
		}
		// Determine final status to detect cancellation.
		final := hist[len(hist)-1]
		finalStatus := ticketState(final)
		if finalStatus == "cancelled" || finalStatus == "dropped" {
			continue
		}

		// First entry into open or in_progress (cycle start).
		var cycleStart time.Time
		for _, r := range hist {
			s := ticketState(r)
			if s != "open" && s != "in_progress" && s != "ready" && s != "doing" {
				continue
			}
			ts, _ := r["ts"].(string)
			cycleStart = parseTS(ts)
			break
		}

		// Find audit-pass done row (if any).
		var auditPassTS time.Time
		var auditPassFound bool
		for _, r := range hist {
			if isAuditPassDone(r) {
				ts, _ := r["ts"].(string)
				auditPassTS = parseTS(ts)
				auditPassFound = true
				break
			}
		}

		// Latest audit_ready row.
		var latestReadyTS time.Time
		var latestReadyFound bool
		for _, r := range hist {
			s := ticketState(r)
			if s != "audit_ready" && s != "review" {
				continue
			}
			ts, _ := r["ts"].(string)
			t := parseTS(ts)
			if !latestReadyFound || t.After(latestReadyTS) {
				latestReadyTS = t
				latestReadyFound = true
			}
		}

		if auditPassFound {
			if !cycleStart.IsZero() && !auditPassTS.Before(cycleStart) {
				cycleHours = append(cycleHours, auditPassTS.Sub(cycleStart).Hours())
			}
			if latestReadyFound && !auditPassTS.Before(latestReadyTS) {
				auditHours = append(auditHours, auditPassTS.Sub(latestReadyTS).Hours())
			}
			continue
		}

		// No audit-pass done yet. If currently pending audit (latest row is
		// audit_ready), contribute to pending audit latency.
		if (finalStatus == "audit_ready" || finalStatus == "review") && latestReadyFound {
			auditHours = append(auditHours, now.Sub(latestReadyTS).Hours())
		}
	}

	out := LifecycleLatency{
		CompletedCycleCount: len(cycleHours),
		PendingAuditCount:   len(auditHours),
	}
	if len(cycleHours) > 0 {
		out.MedianCycleHours = medianLower(cycleHours)
		out.P90CycleHours = p90NearestRank(cycleHours)
	}
	if len(auditHours) > 0 {
		out.MedianAuditLatencyHours = medianLower(auditHours)
		out.P90AuditLatencyHours = p90NearestRank(auditHours)
	}
	return out
}

// medianLower returns the median, picking the lower-of-two for even counts.
func medianLower(xs []float64) float64 {
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return sorted[n/2-1]
}

// p90NearestRank returns the 90th percentile using nearest-rank:
// index = ceil(0.9 * n) - 1 after sort.
func p90NearestRank(xs []float64) float64 {
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n == 0 {
		return 0
	}
	// ceil(0.9 * n) using integer math.
	idx := (9*n + 9) / 10
	if idx < 1 {
		idx = 1
	}
	if idx > n {
		idx = n
	}
	return sorted[idx-1]
}

// computeStaleClaims tallies expired and near-expiring claims across the
// latest ticket rows. Tickets with terminal status (done/cancelled) are
// excluded. Rows with missing/unparseable claim_until are ignored silently.
// Samples are the most overdue (then soonest-to-expire) up to 3 entries.
func computeStaleClaims(latest []ledger.Row, now time.Time) StaleClaims {
	type candidate struct {
		ticketID   string
		claimUntil string
		claimedBy  string
		until      time.Time
		expired    bool
	}
	var cands []candidate
	out := StaleClaims{}
	horizon := now.Add(nearExpiringClaimWindow)
	for _, r := range latest {
		s := ticketState(r)
		if isTerminalState(s) {
			continue
		}
		cu, _ := r["claim_until"].(string)
		if cu == "" {
			continue
		}
		until, err := time.Parse(time.RFC3339, cu)
		if err != nil {
			continue
		}
		id := ticketID(r)
		by := ticketOwner(r)
		switch {
		case until.Before(now):
			out.Expired++
			cands = append(cands, candidate{id, cu, by, until, true})
		case until.Before(horizon):
			out.NearExpiring++
			cands = append(cands, candidate{id, cu, by, until, false})
		}
	}
	// Sort: expired first, then by earliest until (most overdue → soonest).
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].expired != cands[j].expired {
			return cands[i].expired
		}
		if !cands[i].until.Equal(cands[j].until) {
			return cands[i].until.Before(cands[j].until)
		}
		return cands[i].ticketID < cands[j].ticketID
	})
	if len(cands) > 3 {
		cands = cands[:3]
	}
	out.Samples = make([]StaleClaimSample, 0, len(cands))
	for _, c := range cands {
		out.Samples = append(out.Samples, StaleClaimSample{
			TicketID: c.ticketID, ClaimUntil: c.claimUntil, ClaimedBy: c.claimedBy,
		})
	}
	return out
}

// --- Audit queue --------------------------------------------------------------
