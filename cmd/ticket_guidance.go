package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
)

// emitTicketGuidance reads the latest state for the row that was just appended
// and writes guidance text to the provided stderr writer. Best-effort: any
// error during read/compute is ignored because the ledger has already been
// successfully written.
func emitTicketGuidance(dir string, row map[string]any, stderr io.Writer) {
	id, _ := row["ticket"].(string)
	if id == "" {
		return
	}
	tickets, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return
	}
	latest, ok := findLatestTicket(tickets, id)
	if !ok {
		latest = ledger.Row(row)
	}
	worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))
	g := guidance.Compute(latest, worklog)
	g.WritingLanguage = loadWritingLanguage(dir)
	fmt.Fprint(stderr, guidance.RenderText(g))
}
