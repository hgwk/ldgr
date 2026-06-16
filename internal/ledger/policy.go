package ledger

// BoardColumn is the shared display policy for the control board.
type BoardColumn struct {
	ID    string
	Title string
}

var compatStatusTransitions = map[string][]string{
	"":                  {"open", "in_progress"},
	"open":              {"in_progress", "blocked", "cancelled"},
	"in_progress":       {"audit_ready", "blocked", "cancelled"},
	"blocked":           {"in_progress", "cancelled"},
	"audit_ready":       {"done", "changes_requested", "cancelled"},
	"changes_requested": {"in_progress", "open", "cancelled"},
	"done":              nil,
	"cancelled":         nil,
}

var stateTransitions = map[string][]string{
	"":        {"backlog", "ready"},
	"backlog": {"ready", "dropped"},
	"ready":   {"doing", "blocked", "dropped"},
	"doing":   {"review", "blocked", "dropped"},
	"blocked": {"ready", "doing", "dropped"},
	"review":  {"done", "rework", "dropped"},
	"rework":  {"doing", "ready", "dropped"},
	"done":    nil,
	"dropped": nil,
}

var boardColumns = []BoardColumn{
	{ID: "ready", Title: "Ready"},
	{ID: "doing", Title: "Doing"},
	{ID: "review", Title: "Review"},
	{ID: "rework", Title: "Rework"},
	{ID: "backlog", Title: "Backlog"},
	{ID: "blocked", Title: "Blocked"},
	{ID: "done", Title: "Done"},
	{ID: "dropped", Title: "Dropped"},
}

var boardGrid = [][]string{
	{"ready", "doing", "review", "rework"},
	{"backlog", "blocked", "done", "dropped"},
}

// NextCompatStatuses returns legal next values for older status-shaped rows.
func NextCompatStatuses(status string) []string {
	return copyStrings(compatStatusTransitions[status])
}

// AllowsCompatStatusTransition reports whether prev -> next is legal for older
// status-shaped rows.
func AllowsCompatStatusTransition(prev, next string) bool {
	return containsString(compatStatusTransitions[prev], next)
}

// NextStates returns the legal next state values.
func NextStates(state string) []string {
	return copyStrings(stateTransitions[state])
}

// AllowsStateTransition reports whether prev -> next is legal for state-shaped rows.
func AllowsStateTransition(prev, next string) bool {
	return containsString(stateTransitions[prev], next)
}

// BoardColumns returns the control-board columns in display order.
func BoardColumns() []BoardColumn {
	out := make([]BoardColumn, len(boardColumns))
	copy(out, boardColumns)
	return out
}

// BoardGrid returns the control-board grid layout.
func BoardGrid() [][]string {
	out := make([][]string, len(boardGrid))
	for i, row := range boardGrid {
		out[i] = copyStrings(row)
	}
	return out
}

// StatusToState maps older status values onto the board state model.
func StatusToState(status string) string {
	switch status {
	case "open":
		return "ready"
	case "in_progress":
		return "doing"
	case "blocked":
		return "blocked"
	case "audit_ready":
		return "review"
	case "changes_requested":
		return "rework"
	case "done":
		return "done"
	case "cancelled":
		return "dropped"
	default:
		return "backlog"
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func copyStrings(items []string) []string {
	if items == nil {
		return nil
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}
