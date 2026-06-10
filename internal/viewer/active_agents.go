package viewer

import (
	"sort"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func computeActiveAgents(ticketRows, worklogRows []ledger.Row, now time.Time) ActiveAgents {
	out := ActiveAgents{WindowHours: int(activeAgentsWindow / time.Hour)}
	cutoff := now.Add(-activeAgentsWindow)

	type agg struct {
		rows   int
		latest time.Time
		roles  map[string]int
	}
	byAgent := map[string]*agg{}

	visit := func(rows []ledger.Row) {
		invalid := InvalidatedNs(rows)
		for _, r := range rows {
			if _, isCompanion := r["invalidates_n"]; isCompanion {
				continue
			}
			n, _ := r["n"].(float64)
			if _, isInvalid := invalid[int(n)]; isInvalid {
				continue
			}
			ts, _ := r["ts"].(string)
			t := parseTS(ts)
			if t.IsZero() || t.Before(cutoff) {
				continue
			}
			agent := stringField(r, "claimed_by")
			if agent == "" {
				agent = stringField(r, "owner")
			}
			if agent == "" {
				agent = stringField(r, "agent")
			}
			if agent == "" {
				agent = stringField(r, "actor")
			}
			if agent == "" {
				agent = eventString(r, "actor")
			}
			if agent == "" {
				out.UnknownCount++
				continue
			}
			a, ok := byAgent[agent]
			if !ok {
				a = &agg{roles: map[string]int{}}
				byAgent[agent] = a
			}
			a.rows++
			if t.After(a.latest) {
				a.latest = t
			}
			role := stringField(r, "role")
			if role == "" {
				role = eventString(r, "role")
			}
			if role != "" {
				a.roles[role]++
			}
		}
	}
	visit(ticketRows)
	visit(worklogRows)

	list := make([]ActiveAgent, 0, len(byAgent))
	for name, a := range byAgent {
		// Pick most common role; on tie, lexicographically smallest for determinism.
		var role string
		var best int
		for r, c := range a.roles {
			if c > best || (c == best && (role == "" || r < role)) {
				role = r
				best = c
			}
		}
		list = append(list, ActiveAgent{
			Agent:  name,
			Role:   role,
			Rows:   a.rows,
			Latest: a.latest.UTC().Format(time.RFC3339),
		})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Rows != list[j].Rows {
			return list[i].Rows > list[j].Rows
		}
		if list[i].Latest != list[j].Latest {
			return list[i].Latest > list[j].Latest
		}
		return list[i].Agent < list[j].Agent
	})
	if len(list) > activeAgentsMax {
		list = list[:activeAgentsMax]
	}
	out.Agents = list
	return out
}

// perTicketHistory groups ticket rows by ticket id with invalidated rows and
// invalidate-companion rows removed, sorted by n ascending. This matches the
