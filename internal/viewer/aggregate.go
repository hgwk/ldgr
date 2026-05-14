// Package viewer provides the read-only HTTP dashboard for ldgr.
package viewer

import (
	"sort"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

// TreeBucket groups latest tickets by parent_ticket.
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

// InvalidEntry describes a row neutralized by invalidates_n.
type InvalidEntry struct {
	N    int    `json:"n"`
	ViaN int    `json:"via_n"`
	Kind string `json:"kind"`
}

// Insights mirrors the Node prototype's categories.
type Insights struct {
	ReadyQueue            []ledger.Row   `json:"readyQueue"`
	TopBlockers           []BlockerEntry `json:"topBlockers"`
	StaleInProgress       []StaleEntry   `json:"staleInProgress"`
	ClosedWithoutWorklog  []ledger.Row   `json:"closedWithoutWorklog"`
	WorklogsWithoutTicket []ledger.Row   `json:"worklogsWithoutTicket"`
	Invalidated           []InvalidEntry `json:"invalidated"`
	StaleHours            int            `json:"staleHours"`
}

// InvalidatedNs returns invalidated_n → via_n map across the rows.
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
		id, _ := r["ticket"].(string)
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
		ai, _ := out[i]["ticket"].(string)
		bj, _ := out[j]["ticket"].(string)
		return ai < bj
	})
	return out
}

// Tree groups latest tickets by parent_ticket, sorted alphabetically by parent.
func Tree(latest []ledger.Row) []TreeBucket {
	buckets := map[string][]ledger.Row{}
	for _, r := range latest {
		p, _ := r["parent_ticket"].(string)
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

// StatusCounts tallies status values from latest rows.
func StatusCounts(rows []ledger.Row) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		s, _ := r["status"].(string)
		if s == "" {
			continue
		}
		out[s]++
	}
	return out
}

// BuildInsights mirrors templates/serve-ledger.cjs categories.
func BuildInsights(ticketRows, worklogRows []ledger.Row, now time.Time, staleHours int) Insights {
	if staleHours <= 0 {
		staleHours = 24
	}
	latest := LatestTickets(ticketRows)
	invalidated := InvalidatedNs(ticketRows)
	wInvalidated := InvalidatedNs(worklogRows)

	ticketByID := map[string]ledger.Row{}
	for _, t := range latest {
		id, _ := t["ticket"].(string)
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
		if s, _ := t["status"].(string); s != "open" {
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
		id, _ := t["ticket"].(string)
		s, _ := t["status"].(string)
		if s == "done" || s == "cancelled" {
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
					bs, _ := b["status"].(string)
					entry.Status = bs
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
		s, _ := t["status"].(string)
		if s != "in_progress" {
			continue
		}
		id, _ := t["ticket"].(string)
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
			Ticket: id, Status: s, Task: stringField(t, "task"),
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
		s, _ := t["status"].(string)
		if s != "done" && s != "cancelled" {
			continue
		}
		id, _ := t["ticket"].(string)
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

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func stringField(r ledger.Row, k string) string {
	v, _ := r[k].(string)
	return v
}

// --- Dashboard (control tower) -------------------------------------------------

// Dashboard mirrors the JSON shape of GET /api/projects/{id}/dashboard.
type Dashboard struct {
	Progress Progress         `json:"progress"`
	Parents  []ParentProgress `json:"parents"`
	Audit    AuditPipeline    `json:"audit"`
	Health   DeliveryHealth   `json:"health"`
	Recent   []RecentItem     `json:"recent"`
}

type Progress struct {
	Done      int `json:"done"`
	Active    int `json:"active"`
	Cancelled int `json:"cancelled"`
	Percent   int `json:"percent"`
}

type ParentProgress struct {
	Parent    string `json:"parent"`
	Done      int    `json:"done"`
	Active    int    `json:"active"`
	Blocked   int    `json:"blocked"`
	Cancelled int    `json:"cancelled"`
	Percent   int    `json:"percent"`
}

type AuditPipeline struct {
	AuditReady       int `json:"audit_ready"`
	ChangesRequested int `json:"changes_requested"`
	WeakDone         int `json:"weak_done"`
}

type DeliveryHealth struct {
	ClosedWithoutWorklog int `json:"closed_without_worklog"`
	OrphanWorklog        int `json:"orphan_worklog"`
	Invalidated          int `json:"invalidated"`
	MissingEvidence      int `json:"missing_evidence"`
}

type RecentItem struct {
	Kind   string `json:"kind"`
	Ticket string `json:"ticket"`
	TS     string `json:"ts"`
	Status string `json:"status,omitempty"`
	Task   string `json:"task,omitempty"`
	Result string `json:"result,omitempty"`
}

// activeStatuses lists ticket statuses counted as "active" for progress math.
// Cancelled is intentionally absent so it doesn't pull down completion %.
var activeStatuses = map[string]bool{
	"open":              true,
	"in_progress":       true,
	"blocked":           true,
	"audit_ready":       true,
	"changes_requested": true,
}

// BuildDashboard derives the control-tower view from latest ticket + worklog rows.
func BuildDashboard(ticketRows, worklogRows []ledger.Row, now time.Time) Dashboard {
	latest := LatestTickets(ticketRows)

	// Progress.
	var done, active, cancelled int
	for _, r := range latest {
		s, _ := r["status"].(string)
		switch {
		case s == "done":
			done++
		case s == "cancelled":
			cancelled++
		case activeStatuses[s]:
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
		p, _ := r["parent_ticket"].(string)
		s, _ := r["status"].(string)
		entry, ok := byParent[p]
		if !ok {
			entry = &pAgg{Parent: p}
			byParent[p] = entry
		}
		switch {
		case s == "done":
			entry.Done++
		case s == "cancelled":
			entry.Cancelled++
		case s == "blocked":
			entry.Active++
			entry.Blocked++
		case activeStatuses[s]:
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
		s, _ := r["status"].(string)
		switch s {
		case "audit_ready":
			auditReady++
		case "changes_requested":
			changesReq++
		case "done":
			if ar, _ := r["audit_result"].(string); ar != "pass" {
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
		id, _ := t["ticket"].(string)
		knownTicket[id] = struct{}{}
	}
	var closed, orphan, missingEv int
	for _, t := range latest {
		s, _ := t["status"].(string)
		id, _ := t["ticket"].(string)
		if s == "done" || s == "cancelled" {
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
			Kind: "ticket", Ticket: stringField(t, "ticket"), TS: ts,
			Status: stringField(t, "status"), Task: stringField(t, "task"),
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
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].ts > pool[j].ts })
	if len(pool) > 20 {
		pool = pool[:20]
	}
	recent := make([]RecentItem, 0, len(pool))
	for _, s := range pool {
		recent = append(recent, s.item)
	}

	return Dashboard{
		Progress: Progress{Done: done, Active: active, Cancelled: cancelled, Percent: percent},
		Parents:  parents,
		Audit:    AuditPipeline{AuditReady: auditReady, ChangesRequested: changesReq, WeakDone: weakDone},
		Health:   DeliveryHealth{ClosedWithoutWorklog: closed, OrphanWorklog: orphan, Invalidated: invalidated, MissingEvidence: missingEv},
		Recent:   recent,
	}
}

// --- Kanban (control tower) ---------------------------------------------------

// Kanban mirrors the JSON shape of GET /api/projects/{id}/kanban.
type Kanban struct {
	Columns []KanbanColumn `json:"columns"`
}

type KanbanColumn struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	Tickets []ledger.Row `json:"tickets"`
}

// BuildKanban groups latest ticket rows into Plan/Implement/Verify/Complete.
func BuildKanban(ticketRows []ledger.Row) Kanban {
	latest := LatestTickets(ticketRows)

	plan := KanbanColumn{ID: "plan", Title: "Plan"}
	impl := KanbanColumn{ID: "implement", Title: "Implement"}
	verify := KanbanColumn{ID: "verify", Title: "Verify"}
	complete := KanbanColumn{ID: "complete", Title: "Complete"}

	for _, r := range latest {
		s, _ := r["status"].(string)
		role, _ := r["role"].(string)
		switch s {
		case "done", "cancelled":
			complete.Tickets = append(complete.Tickets, r)
		case "audit_ready":
			verify.Tickets = append(verify.Tickets, r)
		case "in_progress", "blocked", "changes_requested":
			if role == "plan" {
				plan.Tickets = append(plan.Tickets, r)
			} else if role == "audit" {
				verify.Tickets = append(verify.Tickets, r)
			} else {
				impl.Tickets = append(impl.Tickets, r)
			}
		case "open":
			plan.Tickets = append(plan.Tickets, r)
		default:
			// Unknown status defaults to plan so nothing disappears.
			plan.Tickets = append(plan.Tickets, r)
		}
	}

	for _, col := range []*KanbanColumn{&plan, &impl, &verify, &complete} {
		sort.SliceStable(col.Tickets, func(i, j int) bool {
			a, _ := col.Tickets[i]["ts"].(string)
			b, _ := col.Tickets[j]["ts"].(string)
			return a > b
		})
	}
	return Kanban{Columns: []KanbanColumn{plan, impl, verify, complete}}
}
