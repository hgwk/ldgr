// Package ledger holds the typed data model and JSONL persistence for
// tickets.jsonl, worklog.jsonl, and goal.json.
package ledger

// Row is the state-shaped wire form: an ordered, unknown-field-preserving
// JSON object. We marshal/unmarshal as map[string]any so that round-trip
// preserves anything the writer included that the current binary doesn't
// know about (forward compatibility).
type Row map[string]any

// Required field sets per spec §3.4 / §3.5.
var (
	TicketRequired = []string{
		"n", "ts", "parent_ticket", "ticket", "agent", "role", "status",
		"task", "scope", "paths", "blocked_by", "branch",
	}
	// "ticket" is intentionally absent — it is optional in worklog rows (§3.5).
	WorklogRequired = []string{
		"n", "ts", "agent", "task", "scope", "result", "paths", "commands",
		"notes", "branch", "commit",
	}
)

// Non-empty semantic string fields. branch/commit must exist where required
// but may be empty when git state is unavailable.
var (
	TicketNonEmpty = []string{
		"parent_ticket", "ticket", "agent", "role", "status", "task", "scope",
	}
	WorklogNonEmpty = []string{
		"agent", "task", "scope", "result",
	}
)

// StatusEnum lists the legal values for ticket.status (§3.4).
var StatusEnum = map[string]struct{}{
	"open":              {},
	"in_progress":       {},
	"blocked":           {},
	"audit_ready":       {},
	"changes_requested": {},
	"done":              {},
	"cancelled":         {},
}

// CategoryEnum lists known ticket categories. Missing or unknown categories
// are warnings (not fails) per spec §6.2.
var CategoryEnum = map[string]struct{}{
	"feature":  {},
	"bug":      {},
	"doc":      {},
	"refactor": {},
	"chore":    {},
	"test":     {},
	"ops":      {},
}

// KindEnum lists the legal values for ticket.kind (optional, defaulted to "task").
var KindEnum = map[string]struct{}{
	"plan":  {},
	"issue": {},
	"task":  {},
	"audit": {},
	"ops":   {},
}

// PriorityEnum lists the legal values for ticket.priority (optional, defaulted to "P2").
var PriorityEnum = map[string]struct{}{
	"P0": {},
	"P1": {},
	"P2": {},
	"P3": {},
}

// State-model field sets and enums: id/state/type/title/event for tickets and
// actor/title/summary for worklogs.
var (
	StateTicketRequired = []string{
		"n", "ts", "id", "parent", "type", "state", "area", "priority",
		"title", "owner", "blocked_by", "acceptance", "evidence", "event",
	}
	StateTicketNonEmpty = []string{
		"id", "parent", "type", "state", "area", "priority", "title", "owner",
	}
	StateWorklogRequired = []string{
		"n", "ts", "ticket", "actor", "title", "summary", "paths", "commands", "notes",
	}
	StateWorklogNonEmpty = []string{
		"ticket", "actor", "title", "summary",
	}
	EventRequired = []string{
		"actor", "role", "summary", "notes",
	}
	EventNonEmpty = []string{
		"actor", "role", "summary",
	}
)

var TicketTypeEnum = map[string]struct{}{
	"epic":  {},
	"plan":  {},
	"issue": {},
	"task":  {},
	"audit": {},
	"ops":   {},
}

var StateEnum = map[string]struct{}{
	"backlog": {},
	"ready":   {},
	"doing":   {},
	"blocked": {},
	"review":  {},
	"rework":  {},
	"done":    {},
	"dropped": {},
}

var AreaEnum = map[string]struct{}{
	"frontend": {},
	"backend":  {},
	"runtime":  {},
	"docs":     {},
	"infra":    {},
	"test":     {},
	"security": {},
	"release":  {},
	"ops":      {},
}

var EventRoleEnum = map[string]struct{}{
	"planner":     {},
	"implementer": {},
	"auditor":     {},
	"operator":    {},
	"reviewer":    {},
}

var EventResultEnum = map[string]struct{}{
	"pass":              {},
	"changes_requested": {},
	"cancelled":         {},
	"corrected":         {},
}

// Goal mirrors ledger/goal.json. unknown fields are preserved through Row,
// but Goal exposes the documented shape for command convenience.
type Goal struct {
	SchemaVersion   int      `json:"schema_version"`
	Track           string   `json:"track"`
	Version         string   `json:"version"`
	Updated         string   `json:"updated"`
	SourceOfTruth   string   `json:"source_of_truth"`
	Summary         string   `json:"summary"`
	SuccessCriteria []string `json:"success_criteria"`
}
