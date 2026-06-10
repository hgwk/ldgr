package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/ledger"
)

func init() {
	Commands["closeout"] = RunCloseoutCLI
}

func RunCloseoutCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("closeout")
	target := fs.String("target", "", "")
	status := fs.String("status", "archived", "archived or closed")
	apply := fs.Bool("apply", false, "write ledger/config.json status")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)
	cfgPath := filepath.Join(dir, "ledger", "config.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	active, err := activeTicketLines(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *apply {
		if *status != "archived" && *status != "closed" {
			fmt.Fprintln(stderr, "--status must be archived or closed")
			return 2
		}
		if err := config.PatchStatus(cfgPath, *status); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		cfg.Status = *status
	}
	printCloseout(stdout, cfg, active, *apply)
	return 0
}

type activeTicket struct {
	ID     string
	State  string
	Line   int
	Title  string
	Append string
}

func activeTicketLines(dir string) ([]activeTicket, error) {
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	stateMode := false
	for _, row := range rows {
		if _, ok := row["state"]; ok {
			stateMode = true
			break
		}
	}
	latest := map[string]ledger.Row{}
	for _, row := range rows {
		id := closeoutStringField(row, "ticket")
		if stateMode {
			id = closeoutStringField(row, "id")
		}
		if id == "" {
			continue
		}
		line, _ := closeoutNumberAsInt(row["n"])
		if cur, ok := latest[id]; ok {
			curLine, _ := closeoutNumberAsInt(cur["n"])
			if line <= curLine {
				continue
			}
		}
		latest[id] = row
	}
	var out []activeTicket
	for id, row := range latest {
		state := closeoutStringField(row, "status")
		if stateMode {
			state = closeoutStringField(row, "state")
		}
		if !isActiveState(state, stateMode) {
			continue
		}
		line, _ := closeoutNumberAsInt(row["n"])
		appendState := "cancelled"
		if stateMode {
			appendState = "dropped"
		}
		out = append(out, activeTicket{ID: id, State: state, Line: line, Title: closeoutStringField(row, "title"), Append: appendState})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func printCloseout(w io.Writer, cfg config.Config, active []activeTicket, applied bool) {
	status := strings.TrimSpace(cfg.Status)
	if status == "" {
		status = "(unset)"
	}
	fmt.Fprintf(w, "project: %s status=%s\n", cfg.Slug, status)
	if applied {
		fmt.Fprintln(w, "config status updated")
	}
	if len(active) == 0 {
		fmt.Fprintln(w, "active tickets: none")
		return
	}
	fmt.Fprintf(w, "active tickets: %d\n", len(active))
	for _, item := range active {
		title := item.Title
		if title != "" {
			title = " " + title
		}
		fmt.Fprintf(w, "  %s line=%d state=%s%s\n", item.ID, item.Line, item.State, title)
		fmt.Fprintf(w, "    suggested terminal state: %s\n", item.Append)
	}
}

func closeoutStringField(row ledger.Row, key string) string {
	value, _ := row[key].(string)
	return value
}

func closeoutNumberAsInt(value any) (int, bool) {
	switch value := value.(type) {
	case float64:
		if value > 0 && value == float64(int(value)) {
			return int(value), true
		}
	case int:
		if value > 0 {
			return value, true
		}
	}
	return 0, false
}

func isActiveState(state string, stateMode bool) bool {
	if stateMode {
		switch state {
		case "ready", "doing", "blocked", "review", "rework":
			return true
		default:
			return false
		}
	}
	switch state {
	case "open", "planned", "claimed", "in_progress", "blocked", "audit_ready", "changes_requested", "review_ready":
		return true
	default:
		return false
	}
}
