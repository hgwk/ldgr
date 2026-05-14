package legacy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/ledger"
)

// Compose builds an executable Plan from sources discovered by Scan.
// now is injected to keep tests deterministic.
func Compose(targetDir string, sources []Source, cfg config.Config, now string) Plan {
	plan := Plan{TargetDir: targetDir}

	var ticketRows, worklogRows []ledger.Row
	var goal *ledger.Goal

	for _, s := range sources {
		if !s.Exists {
			continue
		}
		plan.Sources = append(plan.Sources, s)
		switch s.Kind {
		case SourceLegacyTickets:
			rows, c, w := NormalizeTickets(s.Rows, cfg.Parents, now)
			rows, c = insertInvalidates(rows, c, now, "ticket")
			ticketRows = rows
			mergeCounts(&plan.Counts, c)
			plan.Counts.TicketsImported = len(ticketRows)
			plan.Warnings = append(plan.Warnings, w...)
			plan.ParseErrors = append(plan.ParseErrors, s.ParseErrs...)
		case SourceLegacyWorklog:
			rows, c, w := NormalizeWorklog(s.Rows, now)
			rows, c = insertInvalidates(rows, c, now, "worklog")
			worklogRows = rows
			mergeCounts(&plan.Counts, c)
			plan.Counts.WorklogImported = len(worklogRows)
			plan.Warnings = append(plan.Warnings, w...)
			plan.ParseErrors = append(plan.ParseErrors, s.ParseErrs...)
		case SourceLegacyGoal:
			if s.Goal != nil {
				goal = s.Goal
				plan.Counts.GoalCreated = true
			}
		}
	}

	if ticketRows != nil {
		plan.Changes = append(plan.Changes, diffJSONL(targetDir, "ledger/tickets.jsonl", ticketRows))
	}
	if worklogRows != nil {
		plan.Changes = append(plan.Changes, diffJSONL(targetDir, "ledger/worklog.jsonl", worklogRows))
	}
	if goal != nil {
		plan.Changes = append(plan.Changes, diffJSON(targetDir, "ledger/goal.json", goal))
	}
	if len(plan.ParseErrors) > 0 {
		plan.Counts.ParseErrors = len(plan.ParseErrors)
		plan.Changes = append(plan.Changes, parseErrorsChange(targetDir, plan.ParseErrors))
	}
	return plan
}

// insertInvalidates inserts a companion `invalidates_n` row after each
// ghost row. kind selects the row shape ("ticket" or "worklog").
func insertInvalidates(rows []ledger.Row, counts Counts, now string, kind string) ([]ledger.Row, Counts) {
	out := make([]ledger.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, r)
		ghost := false
		if kind == "ticket" {
			ghost = isGhostTicket(r)
		} else {
			ghost = isGhostWorklog(r)
		}
		if ghost {
			n, _ := r["n"].(int)
			// Use the ghost row's own ts for the companion so the output is
			// deterministic across runs (otherwise wall-clock `now` makes
			// --apply non-idempotent). Equal ts is fine; verify only enforces
			// non-decreasing.
			ts, _ := r["ts"].(string)
			if ts == "" {
				ts = now
			}
			out = append(out, companionRow(kind, n, ts))
		}
	}
	// Re-number consecutively. Each companion sits directly after its ghost,
	// so the ghost's new n equals the companion's 0-based index `i`.
	for i := range out {
		out[i]["n"] = i + 1
		if _, ok := out[i]["invalidates_n"]; ok {
			out[i]["invalidates_n"] = i // ghost's new n
		}
	}
	return out, counts
}

func companionRow(kind string, ghostN int, now string) ledger.Row {
	if kind == "ticket" {
		return ledger.Row{
			"ts":            now,
			"parent_ticket": "LEGACY",
			"ticket":        fmt.Sprintf("legacy-invalid-%d", ghostN),
			"agent":         "legacy",
			"role":          "ops",
			"status":        "cancelled",
			"task":          fmt.Sprintf("invalidate ghost row %d", ghostN),
			"scope":         "ledger",
			"paths":         []any{},
			"blocked_by":    []any{},
			"branch":        "",
			"invalidates_n": ghostN,
		}
	}
	return ledger.Row{
		"ts":            now,
		"agent":         "legacy",
		"task":          fmt.Sprintf("invalidate ghost worklog row %d", ghostN),
		"scope":         "ledger",
		"result":        fmt.Sprintf("invalidate ghost worklog row %d", ghostN),
		"paths":         []any{},
		"commands":      []any{},
		"notes":         "",
		"branch":        "",
		"commit":        "",
		"invalidates_n": ghostN,
	}
}

func diffJSONL(targetDir, rel string, rows []ledger.Row) Change {
	var buf bytes.Buffer
	for _, r := range rows {
		b, _ := json.Marshal(r)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	full := filepath.Join(targetDir, rel)
	return diffBytes(full, rel, buf.Bytes())
}

func diffJSON(targetDir, rel string, v any) Change {
	b, _ := json.MarshalIndent(v, "", "  ")
	b = append(b, '\n')
	full := filepath.Join(targetDir, rel)
	return diffBytes(full, rel, b)
}

func parseErrorsChange(targetDir string, errs []ParseError) Change {
	var buf bytes.Buffer
	for _, e := range errs {
		row := map[string]any{"line": e.Line, "raw": e.Raw, "error": e.Err}
		b, _ := json.Marshal(row)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	rel := "ledger/import-errors.jsonl"
	full := filepath.Join(targetDir, rel)
	return diffBytes(full, rel, buf.Bytes())
}

func diffBytes(full, rel string, want []byte) Change {
	cur, err := os.ReadFile(full)
	if err != nil && !os.IsNotExist(err) {
		return Change{OutputPath: rel, Action: ActionReplace, NewBytes: want}
	}
	if err != nil { // not exist
		return Change{OutputPath: rel, Action: ActionCreate, NewBytes: want}
	}
	if bytes.Equal(cur, want) {
		return Change{OutputPath: rel, Action: ActionNoop, NewBytes: want}
	}
	return Change{OutputPath: rel, Action: ActionReplace, NewBytes: want}
}

func mergeCounts(dst *Counts, src Counts) {
	dst.NReassigned += src.NReassigned
	dst.TSReplaced += src.TSReplaced
	dst.AgentDefaulted += src.AgentDefaulted
	dst.ParentInferred += src.ParentInferred
	dst.BranchInferred += src.BranchInferred
	dst.GhostTickets += src.GhostTickets
	dst.GhostWorklog += src.GhostWorklog
}
