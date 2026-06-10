package viewer

import (
	"sort"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func BuildInsights(ticketRows, worklogRows []ledger.Row, now time.Time, staleHours int) Insights {
	if staleHours <= 0 {
		staleHours = 24
	}
	latest := LatestTickets(ticketRows)
	invalidated := InvalidatedNs(ticketRows)
	wInvalidated := InvalidatedNs(worklogRows)

	ticketByID := map[string]ledger.Row{}
	for _, t := range latest {
		id := ticketID(t)
		ticketByID[id] = t
	}

	worklogByTicket := map[string][]ledger.Row{}
	for _, w := range worklogRows {
		n, _ := w["n"].(float64)
		if _, isInvalid := wInvalidated[int(n)]; isInvalid {
			continue
		}
		if _, isCompanion := w["invalidates_n"]; isCompanion {
			continue
		}
		id, _ := w["ticket"].(string)
		if id == "" {
			continue
		}
		worklogByTicket[id] = append(worklogByTicket[id], w)
	}

	ins := Insights{StaleHours: staleHours}

	// readyQueue: open, blocked_by empty.
	for _, t := range latest {
		if s := ticketState(t); s != "open" && s != "ready" {
			continue
		}
		bb, _ := t["blocked_by"].([]any)
		if len(bb) > 0 {
			continue
		}
		ins.ReadyQueue = append(ins.ReadyQueue, t)
	}
	sort.Slice(ins.ReadyQueue, func(i, j int) bool {
		a, _ := ins.ReadyQueue[i]["ts"].(string)
		b, _ := ins.ReadyQueue[j]["ts"].(string)
		return a < b
	})
	if len(ins.ReadyQueue) > 8 {
		ins.ReadyQueue = ins.ReadyQueue[:8]
	}

	// topBlockers: aggregate dependents.
	blockerMap := map[string]*BlockerEntry{}
	for _, t := range latest {
		id := ticketID(t)
		s := ticketState(t)
		if s == "done" || s == "cancelled" || s == "dropped" {
			continue
		}
		bb, _ := t["blocked_by"].([]any)
		for _, raw := range bb {
			ref, _ := raw.(string)
			if ref == "" {
				continue
			}
			entry, ok := blockerMap[ref]
			if !ok {
				entry = &BlockerEntry{Ticket: ref, Status: "missing"}
				if b := ticketByID[ref]; b != nil {
					entry.Status = ticketState(b)
				}
				blockerMap[ref] = entry
			}
			entry.Dependents = append(entry.Dependents, id)
		}
	}
	for _, e := range blockerMap {
		ins.TopBlockers = append(ins.TopBlockers, *e)
	}
	sort.Slice(ins.TopBlockers, func(i, j int) bool {
		if len(ins.TopBlockers[i].Dependents) != len(ins.TopBlockers[j].Dependents) {
			return len(ins.TopBlockers[i].Dependents) > len(ins.TopBlockers[j].Dependents)
		}
		return ins.TopBlockers[i].Ticket < ins.TopBlockers[j].Ticket
	})
	if len(ins.TopBlockers) > 8 {
		ins.TopBlockers = ins.TopBlockers[:8]
	}

	// staleInProgress: in_progress with age >= staleHours.
	staleMS := int64(staleHours) * int64(time.Hour/time.Millisecond)
	for _, t := range latest {
		s := ticketState(t)
		if s != "in_progress" && s != "doing" {
			continue
		}
		id := ticketID(t)
		tsStr, _ := t["ts"].(string)
		lastTouch := parseTS(tsStr)
		var latestN int
		for _, w := range worklogByTicket[id] {
			ws, _ := w["ts"].(string)
			wt := parseTS(ws)
			if wt.After(lastTouch) {
				lastTouch = wt
			}
			if n, ok := w["n"].(float64); ok && int(n) > latestN {
				latestN = int(n)
			}
		}
		age := now.Sub(lastTouch).Milliseconds()
		if age < staleMS && latestN > 0 {
			continue
		}
		ins.StaleInProgress = append(ins.StaleInProgress, StaleEntry{
			Ticket: id, Status: s, Task: ticketTitle(t),
			AgeMS: age, LatestWorklogN: latestN,
		})
	}
	sort.Slice(ins.StaleInProgress, func(i, j int) bool {
		return ins.StaleInProgress[i].AgeMS > ins.StaleInProgress[j].AgeMS
	})
	if len(ins.StaleInProgress) > 8 {
		ins.StaleInProgress = ins.StaleInProgress[:8]
	}

	// closedWithoutWorklog: done/cancelled with no worklog row by ticket id.
	for _, t := range latest {
		s := ticketState(t)
		if s != "done" {
			continue
		}
		id := ticketID(t)
		if _, has := worklogByTicket[id]; has {
			continue
		}
		ins.ClosedWithoutWorklog = append(ins.ClosedWithoutWorklog, t)
	}
	sort.Slice(ins.ClosedWithoutWorklog, func(i, j int) bool {
		a, _ := ins.ClosedWithoutWorklog[i]["ts"].(string)
		b, _ := ins.ClosedWithoutWorklog[j]["ts"].(string)
		return a > b
	})
	if len(ins.ClosedWithoutWorklog) > 8 {
		ins.ClosedWithoutWorklog = ins.ClosedWithoutWorklog[:8]
	}

	// worklogsWithoutTicket: worklog has ticket field that isn't a known ticket id.
	for _, w := range worklogRows {
		n, _ := w["n"].(float64)
		if _, isInvalid := wInvalidated[int(n)]; isInvalid {
			continue
		}
		if _, isCompanion := w["invalidates_n"]; isCompanion {
			continue
		}
		id, _ := w["ticket"].(string)
		if id == "" {
			continue
		}
		if _, ok := ticketByID[id]; ok {
			continue
		}
		ins.WorklogsWithoutTicket = append(ins.WorklogsWithoutTicket, w)
	}
	if len(ins.WorklogsWithoutTicket) > 8 {
		ins.WorklogsWithoutTicket = ins.WorklogsWithoutTicket[len(ins.WorklogsWithoutTicket)-8:]
	}

	// invalidated: ghost rows by kind.
	for n, via := range invalidated {
		ins.Invalidated = append(ins.Invalidated, InvalidEntry{N: n, ViaN: via, Kind: "ticket"})
	}
	for n, via := range wInvalidated {
		ins.Invalidated = append(ins.Invalidated, InvalidEntry{N: n, ViaN: via, Kind: "worklog"})
	}
	sort.Slice(ins.Invalidated, func(i, j int) bool {
		if ins.Invalidated[i].Kind != ins.Invalidated[j].Kind {
			return ins.Invalidated[i].Kind < ins.Invalidated[j].Kind
		}
		return ins.Invalidated[i].N < ins.Invalidated[j].N
	})

	return ins
}
