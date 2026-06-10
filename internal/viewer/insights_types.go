package viewer

import "github.com/hgwk/ldgr/internal/ledger"

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
