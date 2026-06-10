// Package viewer provides the read-only HTTP dashboard for ldgr.
package viewer

import (
	"sort"

	"github.com/hgwk/ldgr/internal/ledger"
)

type TreeBucket struct {
	Parent  string       `json:"parent"`
	Tickets []ledger.Row `json:"tickets"`
}

// BlockerEntry is the report shape for a single blocking ticket.
type BlockerEntry struct {
	Ticket     string   `json:"ticket"`
	Dependents []string `json:"dependents"`
	Status     string   `json:"status"`
}

// StaleEntry is the report shape for stale in_progress tickets.
type StaleEntry struct {
	Ticket         string `json:"ticket"`
	Status         string `json:"status"`
	Task           string `json:"task"`
	AgeMS          int64  `json:"age_ms"`
	LatestWorklogN int    `json:"latest_worklog_n"`
}

func InvalidatedNs(rows []ledger.Row) map[int]int {
	out := map[int]int{}
	for _, r := range rows {
		v, ok := r["invalidates_n"].(float64)
		if !ok {
			continue
		}
		n, _ := r["n"].(float64)
		out[int(v)] = int(n)
	}
	return out
}

// LatestTickets returns the row with greatest n per ticket id, excluding both
// invalidated rows and the companion invalidate-rows themselves.
func LatestTickets(rows []ledger.Row) []ledger.Row {
	invalidated := InvalidatedNs(rows)
	// invalidate-companion rows are recognizable by having `invalidates_n`.
	latest := map[string]ledger.Row{}
	for _, r := range rows {
		n, _ := r["n"].(float64)
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		if _, isGhost := invalidated[int(n)]; isGhost {
			continue
		}
		id := ticketID(r)
		if id == "" {
			continue
		}
		if cur, ok := latest[id]; ok {
			cn, _ := cur["n"].(float64)
			if n <= cn {
				continue
			}
		}
		latest[id] = r
	}
	out := make([]ledger.Row, 0, len(latest))
	for _, r := range latest {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := out[i]["ts"].(string)
		b, _ := out[j]["ts"].(string)
		if a != b {
			return a > b
		}
		ai := ticketID(out[i])
		bj := ticketID(out[j])
		return ai < bj
	})
	return out
}

// Tree groups latest tickets by parent_ticket, sorted alphabetically by parent.
func Tree(latest []ledger.Row) []TreeBucket {
	buckets := map[string][]ledger.Row{}
	for _, r := range latest {
		p := ticketParent(r)
		buckets[p] = append(buckets[p], r)
	}
	parents := make([]string, 0, len(buckets))
	for p := range buckets {
		parents = append(parents, p)
	}
	sort.Strings(parents)
	out := make([]TreeBucket, 0, len(parents))
	for _, p := range parents {
		rows := buckets[p]
		sort.Slice(rows, func(i, j int) bool {
			a, _ := rows[i]["ts"].(string)
			b, _ := rows[j]["ts"].(string)
			return a > b
		})
		out = append(out, TreeBucket{Parent: p, Tickets: rows})
	}
	return out
}

// StatusCounts tallies status/state values from latest rows.
func StatusCounts(rows []ledger.Row) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		s := ticketState(r)
		if s == "" {
			continue
		}
		out[s]++
	}
	return out
}
