package viewer

import (
	"sort"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func BuildDashboard(ticketRows, worklogRows []ledger.Row, now time.Time) Dashboard {
	latest := LatestTickets(ticketRows)

	// Progress.
	var done, active, cancelled int
	for _, r := range latest {
		s := ticketState(r)
		switch {
		case s == "done":
			done++
		case s == "cancelled" || s == "dropped":
			cancelled++
		case activeStatuses[s] || isActiveState(s):
			active++
		}
	}
	denom := done + active
	percent := 0
	if denom > 0 {
		percent = (done * 100) / denom
	}

	// Parents.
	type pAgg struct {
		Parent                           string
		Done, Active, Blocked, Cancelled int
	}
	byParent := map[string]*pAgg{}
	for _, r := range latest {
		p := ticketParent(r)
		s := ticketState(r)
		entry, ok := byParent[p]
		if !ok {
			entry = &pAgg{Parent: p}
			byParent[p] = entry
		}
		switch {
		case s == "done":
			entry.Done++
		case s == "cancelled" || s == "dropped":
			entry.Cancelled++
		case s == "blocked":
			entry.Active++
			entry.Blocked++
		case activeStatuses[s] || isActiveState(s):
			entry.Active++
		}
	}
	parents := make([]ParentProgress, 0, len(byParent))
	for _, e := range byParent {
		den := e.Done + e.Active
		pct := 0
		if den > 0 {
			pct = (e.Done * 100) / den
		}
		parents = append(parents, ParentProgress{
			Parent: e.Parent, Done: e.Done, Active: e.Active,
			Blocked: e.Blocked, Cancelled: e.Cancelled, Percent: pct,
		})
	}
	sort.Slice(parents, func(i, j int) bool { return parents[i].Parent < parents[j].Parent })

	// Audit pipeline.
	var auditReady, changesReq, weakDone int
	for _, r := range latest {
		s := ticketState(r)
		switch s {
		case "audit_ready", "review":
			auditReady++
		case "changes_requested", "rework":
			changesReq++
		case "done":
			if ar, _ := r["audit_result"].(string); ar == "pass" {
				continue
			}
			if eventString(r, "result") != "pass" {
				weakDone++
			}
		}
	}

	// Delivery health.
	worklogByTicket := map[string][]ledger.Row{}
	wInvalidated := InvalidatedNs(worklogRows)
	for _, w := range worklogRows {
		if _, isCompanion := w["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := w["n"].(float64)
		if _, isInvalid := wInvalidated[int(n)]; isInvalid {
			continue
		}
		id, _ := w["ticket"].(string)
		if id == "" {
			continue
		}
		worklogByTicket[id] = append(worklogByTicket[id], w)
	}
	knownTicket := map[string]struct{}{}
	for _, t := range latest {
		id := ticketID(t)
		knownTicket[id] = struct{}{}
	}
	var closed, orphan, missingEv int
	for _, t := range latest {
		s := ticketState(t)
		id := ticketID(t)
		if s == "done" {
			if _, has := worklogByTicket[id]; !has {
				closed++
			}
		}
		if s == "done" {
			ev, _ := t["evidence"].([]any)
			if len(ev) == 0 && len(worklogByTicket[id]) == 0 {
				missingEv++
			}
		}
	}
	for _, w := range worklogRows {
		if _, isCompanion := w["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := w["n"].(float64)
		if _, isInvalid := wInvalidated[int(n)]; isInvalid {
			continue
		}
		id, _ := w["ticket"].(string)
		if id == "" {
			continue
		}
		if _, ok := knownTicket[id]; !ok {
			orphan++
		}
	}
	tInvalidated := InvalidatedNs(ticketRows)
	invalidated := len(tInvalidated) + len(wInvalidated)

	// Recent activity: merge ticket rows + worklog rows, newest TS first, cap 20.
	type stamped struct {
		ts   string
		item RecentItem
	}
	var pool []stamped
	for _, t := range ticketRows {
		if _, isCompanion := t["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := t["n"].(float64)
		if _, isInvalid := tInvalidated[int(n)]; isInvalid {
			continue
		}
		ts, _ := t["ts"].(string)
		pool = append(pool, stamped{ts: ts, item: RecentItem{
			Kind: "ticket", Ticket: ticketID(t), TS: ts,
			Status: ticketState(t), Task: ticketTitle(t),
		}})
	}
	for _, w := range worklogRows {
		if _, isCompanion := w["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := w["n"].(float64)
		if _, isInvalid := wInvalidated[int(n)]; isInvalid {
			continue
		}
		ts, _ := w["ts"].(string)
		pool = append(pool, stamped{ts: ts, item: RecentItem{
			Kind: "worklog", Ticket: stringField(w, "ticket"), TS: ts,
			Task: stringField(w, "task"), Result: stringField(w, "result"),
		}})
		if pool[len(pool)-1].item.Task == "" {
			pool[len(pool)-1].item.Task = stringField(w, "title")
		}
		if pool[len(pool)-1].item.Result == "" {
			pool[len(pool)-1].item.Result = stringField(w, "summary")
		}
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].ts > pool[j].ts })
	if len(pool) > 20 {
		pool = pool[:20]
	}
	recent := make([]RecentItem, 0, len(pool))
	for _, s := range pool {
		recent = append(recent, s.item)
	}

	// Priority: only count active (open/in_progress/blocked/audit_ready/changes_requested).
	var pc PriorityCounts
	for _, t := range latest {
		s := ticketState(t)
		if !activeStatuses[s] && !isActiveState(s) {
			continue
		}
		p, _ := t["priority"].(string)
		switch p {
		case "P0":
			pc.P0++
		case "P1":
			pc.P1++
		case "P2":
			pc.P2++
		case "P3":
			pc.P3++
		}
	}

	// Kind distribution: count ALL latest tickets by kind, sort by count desc, kind asc.
	byKind := map[string]int{}
	for _, t := range latest {
		k := ticketType(t)
		if k == "" {
			k = "—"
		}
		byKind[k]++
	}
	type kvk struct {
		k string
		v int
	}
	var arr []kvk
	for k, v := range byKind {
		arr = append(arr, kvk{k, v})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].v != arr[j].v {
			return arr[i].v > arr[j].v
		}
		return arr[i].k < arr[j].k
	})
	var kinds []KindCount
	for _, p := range arr {
		kinds = append(kinds, KindCount{Kind: p.k, Count: p.v})
	}

	stale := computeStaleClaims(latest, now)
	lifecycle := ComputeLifecycleLatency(perTicketHistory(ticketRows), now)
	agents := computeActiveAgents(ticketRows, worklogRows, now)

	return Dashboard{
		Progress:     Progress{Done: done, Active: active, Cancelled: cancelled, Percent: percent},
		Parents:      parents,
		Audit:        AuditPipeline{AuditReady: auditReady, ChangesRequested: changesReq, WeakDone: weakDone},
		Health:       DeliveryHealth{ClosedWithoutWorklog: closed, OrphanWorklog: orphan, Invalidated: invalidated, MissingEvidence: missingEv},
		Recent:       recent,
		Priority:     pc,
		Kind:         kinds,
		StaleClaims:  stale,
		Lifecycle:    lifecycle,
		ActiveAgents: agents,
	}
}

// computeActiveAgents aggregates ticket and worklog rows from the trailing
// 24h window, grouped by actor identity. Precedence per row:
// claimed_by → owner → agent → actor → event.actor. Empty/missing values land
// in the unknown bucket.
