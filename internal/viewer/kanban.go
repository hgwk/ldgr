package viewer

import (
	"sort"

	"github.com/hgwk/ldgr/internal/ledger"
)

type Kanban struct {
	Columns []KanbanColumn `json:"columns"`
	Grid    [][]string     `json:"grid,omitempty"`
}

type KanbanColumn struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	Tickets []ledger.Row `json:"tickets"`
}

// BuildKanban projects ticket rows into the shared 4x2 control board. Older
// status-shaped rows get a transient `state` field in the API response so the
// frontend can render and filter consistently.
func BuildKanban(ticketRows []ledger.Row) Kanban {
	latest := LatestTickets(ticketRows)
	cols := kanbanColumnsFromPolicy()
	byID := map[string]*KanbanColumn{}
	for i := range cols {
		byID[cols[i].ID] = &cols[i]
	}

	for _, r := range latest {
		projected := cloneRow(r)
		state := boardState(projected)
		projected["state"] = state
		col := byID[state]
		if col == nil {
			col = byID["backlog"]
		}
		col.Tickets = append(col.Tickets, projected)
	}

	for i := range cols {
		sort.SliceStable(cols[i].Tickets, func(a, b int) bool {
			ats, _ := cols[i].Tickets[a]["ts"].(string)
			bts, _ := cols[i].Tickets[b]["ts"].(string)
			return ats > bts
		})
	}
	return Kanban{
		Columns: cols,
		Grid:    ledger.BoardGrid(),
	}
}

func kanbanColumnsFromPolicy() []KanbanColumn {
	specs := ledger.BoardColumns()
	cols := make([]KanbanColumn, len(specs))
	for i, spec := range specs {
		cols[i] = KanbanColumn{ID: spec.ID, Title: spec.Title}
	}
	return cols
}

func boardState(row ledger.Row) string {
	if state := stringField(row, "state"); state != "" {
		return state
	}
	return ledger.StatusToState(stringField(row, "status"))
}

func cloneRow(row ledger.Row) ledger.Row {
	out := make(ledger.Row, len(row)+1)
	for k, v := range row {
		out[k] = v
	}
	return out
}
