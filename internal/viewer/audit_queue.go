package viewer

import (
	"sort"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

type AuditQueueItem struct {
	TicketID     string `json:"ticket_id"`
	Task         string `json:"task"`
	Priority     string `json:"priority"`
	WaitingSince string `json:"waiting_since"`
	ClaimedBy    string `json:"claimed_by"`
	Agent        string `json:"agent"`
	HasEvidence  bool   `json:"has_evidence"`
}

// priorityRank maps the priority enum to a sort rank. Lower = higher priority.
// Missing or unknown priorities are treated as P2.
func priorityRank(p string) int {
	switch p {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P3":
		return 3
	default:
		return 2 // P2 or missing/unknown
	}
}

// normalizePriority returns the priority string with missing/unknown values
// coerced to "P2" so it round-trips through the API consistently.
func normalizePriority(p string) string {
	switch p {
	case "P0", "P1", "P2", "P3":
		return p
	default:
		return "P2"
	}
}

// BuildAuditQueue returns audit_ready tickets sorted by priority (P0..P3) then
// age (older first). `latest` is the map of ticket id → latest row.
func BuildAuditQueue(latest []ledger.Row, now time.Time) []AuditQueueItem {
	out := make([]AuditQueueItem, 0)
	for _, r := range latest {
		s := ticketState(r)
		if s != "audit_ready" && s != "review" {
			continue
		}
		id := ticketID(r)
		if id == "" {
			continue
		}
		prio, _ := r["priority"].(string)
		ts, _ := r["ts"].(string)
		claimedBy, _ := r["claimed_by"].(string)
		agent := ticketOwner(r)
		ev, _ := r["evidence"].([]any)
		hasEv := false
		for _, e := range ev {
			if s, _ := e.(string); s != "" {
				hasEv = true
				break
			}
		}
		out = append(out, AuditQueueItem{
			TicketID:     id,
			Task:         ticketTitle(r),
			Priority:     normalizePriority(prio),
			WaitingSince: ts,
			ClaimedBy:    claimedBy,
			Agent:        agent,
			HasEvidence:  hasEv,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := priorityRank(out[i].Priority), priorityRank(out[j].Priority)
		if ri != rj {
			return ri < rj
		}
		// Older waiting_since first within the same priority band.
		if out[i].WaitingSince != out[j].WaitingSince {
			return out[i].WaitingSince < out[j].WaitingSince
		}
		return out[i].TicketID < out[j].TicketID
	})
	_ = now // accepted for parity with other Build* functions; age is computed client-side from WaitingSince
	return out
}

// --- Kanban (control tower) ---------------------------------------------------
