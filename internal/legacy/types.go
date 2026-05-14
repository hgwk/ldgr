// Package legacy implements the data migration from earlier ledger layouts
// to the spec §3 standard. Spec §11.
package legacy

import "github.com/hgwk/ldgr/internal/ledger"

// Source enumerates the files we scan in the target directory.
type Source struct {
	Path      string       // absolute path on disk
	Kind      SourceKind   // tickets, worklog, goal, etc.
	Rows      []ledger.Row // parsed rows (empty for Goal)
	Goal      *ledger.Goal // populated only for SourceGoal
	ParseErrs []ParseError // rows that failed JSON parsing, keyed by 1-based line
	Exists    bool
}

type SourceKind int

const (
	SourceUnknown        SourceKind = iota
	SourceLegacyTickets             // <target>/agent-tickets.jsonl
	SourceLegacyWorklog             // <target>/agent-worklog.jsonl
	SourceLegacyGoal                // <target>/goal.json
	SourceCurrentTickets            // <target>/ledger/tickets.jsonl
	SourceCurrentWorklog            // <target>/ledger/worklog.jsonl
	SourceCurrentGoal               // <target>/ledger/goal.json
)

type ParseError struct {
	Line int    // 1-based
	Raw  string // raw line text
	Err  string // parse error message
}

// Plan describes what `--apply` would do. Build once from sources, then
// either render (--plan) or execute (--apply).
type Plan struct {
	TargetDir   string
	Sources     []Source     // exists==true subset only
	Changes     []Change     // per output file
	Warnings    []string     // human-readable, surfaced in plan + apply
	ParseErrors []ParseError // aggregated across sources; routed to import-errors.jsonl
	Counts      Counts
}

// HasChanges returns true if at least one Change is non-noop.
func (p Plan) HasChanges() bool {
	for _, c := range p.Changes {
		if c.Action != ActionNoop {
			return true
		}
	}
	return false
}

// Change represents a write the apply phase will perform.
type Change struct {
	OutputPath string // relative to TargetDir
	Action     ChangeAction
	NewBytes   []byte // for create/replace — full file contents
}

type ChangeAction int

const (
	ActionNoop ChangeAction = iota
	ActionCreate
	ActionReplace
)

// Counts feeds the plan report.
type Counts struct {
	TicketsImported int
	WorklogImported int
	GoalCreated     bool
	ParentInferred  int
	BranchInferred  int
	NReassigned     int
	TSReplaced      int
	AgentDefaulted  int
	GhostTickets    int // rows with empty semantic ticket fields
	GhostWorklog    int // rows with empty semantic worklog fields
	ParseErrors     int
	OrphanWorklog   int
}
