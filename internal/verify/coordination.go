package verify

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/hgwk/ldgr/internal/coordination"
	"github.com/hgwk/ldgr/internal/ledger"
)

func checkCoordinationRows(rep *Report, rows []ledger.Row) {
	sum := coordination.BuildSummary(rows, time.Now())
	for _, conflict := range sum.Conflicts {
		rep.Warns = append(rep.Warns, Issue{
			File: "ledger/coordination.jsonl",
			Line: conflict.Second.N,
			Code: "COORDINATION_CLAIM_CONFLICT",
			Message: fmt.Sprintf("[COORDINATION_CLAIM_CONFLICT] %s and %s both claim %s",
				conflict.First.ID, conflict.Second.ID, conflict.Resource),
		})
	}
	for _, claim := range sum.Claims {
		if !claim.Expired {
			continue
		}
		rep.Warns = append(rep.Warns, Issue{
			File:    "ledger/coordination.jsonl",
			Line:    claim.N,
			Code:    "COORDINATION_CLAIM_STALE",
			Message: fmt.Sprintf("[COORDINATION_CLAIM_STALE] %s expired for %s", claim.ID, claim.Ticket),
		})
	}
}

func readCoordinationRows(targetDir string, rep *Report) []ledger.Row {
	rows, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", coordination.FileName))
	if err != nil {
		rep.Fails = append(rep.Fails, Issue{
			File:    "ledger/coordination.jsonl",
			Code:    "PARSING_ERROR",
			Message: err.Error(),
		})
		return nil
	}
	return rows
}
