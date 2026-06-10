package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/lifecycle"
)

func ticketIsAuditPassDone(latest ledger.Row) bool {
	if _, isState := latest["state"]; isState {
		return isStateWorklogAllowed(latest)
	}
	return lifecycle.IsAuditPassDone(latest)
}

func suggestWorklog(latest ledger.Row, worklog []ledger.Row, writingLanguage string, stdout io.Writer) int {
	if !ticketIsAuditPassDone(latest) {
		g := guidance.Compute(latest, worklog)
		if _, isState := latest["state"]; isState {
			g = guidance.ComputeState(latest, worklog)
		}
		g.WritingLanguage = writingLanguage
		fmt.Fprint(stdout, guidance.RenderText(g))
		return 0
	}
	if _, isState := latest["state"]; isState {
		skeleton := map[string]any{
			"ticket":   latest["id"],
			"actor":    latest["owner"],
			"title":    latest["title"],
			"summary":  localizedShippedResult(writingLanguage, stringField(latest, "title")),
			"paths":    []any{},
			"commands": ifSliceField(latest, "evidence"),
			"notes":    "",
		}
		addWritingLanguage(skeleton, writingLanguage)
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(skeleton); err != nil {
			return 1
		}
		return 0
	}
	skeleton := map[string]any{
		"ticket":   latest["ticket"],
		"task":     latest["task"],
		"scope":    latest["scope"],
		"result":   localizedShippedResult(writingLanguage, stringField(latest, "task")),
		"paths":    latest["paths"],
		"commands": ifSliceField(latest, "evidence"),
		"notes":    "",
	}
	addWritingLanguage(skeleton, writingLanguage)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}
