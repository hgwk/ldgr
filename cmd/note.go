package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/coordination"
	"github.com/hgwk/ldgr/internal/ledger"
)

func init() {
	Commands["note"] = RunNoteCLI
}

func RunNoteCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprintln(stderr, "usage: ldgr note add [flags]")
		return 2
	}
	if args[0] != "add" {
		fmt.Fprintf(stderr, "unknown note subcommand: %s\n", args[0])
		return 2
	}
	return runNoteAdd(args[1:], stdout, stderr)
}

func runNoteAdd(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("note add")
	target := fs.String("target", "", "")
	kind := fs.String("kind", "decision", "decision|risk|handoff|broadcast")
	scope := fs.String("scope", "", "resource/path/scope")
	lane := fs.String("lane", "", "coordination lane")
	team := fs.String("team", "", "team name")
	summary := fs.String("summary", "", "short note summary")
	body := fs.String("body", "", "longer note body")
	var tickets stringListFlag
	fs.Var(&tickets, "ticket", "linked ticket; repeat or comma-separate")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !validNoteKind(*kind) {
		fmt.Fprintln(stderr, "note add: --kind must be decision, risk, handoff, or broadcast")
		return 2
	}
	if strings.TrimSpace(*summary) == "" {
		fmt.Fprintln(stderr, "note add: --summary is required")
		return 2
	}
	row := ledger.Row{
		"type":    "note",
		"id":      "note-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		"ts":      time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"kind":    strings.TrimSpace(*kind),
		"scope":   strings.TrimSpace(*scope),
		"lane":    strings.TrimSpace(*lane),
		"team":    strings.TrimSpace(*team),
		"summary": strings.TrimSpace(*summary),
		"body":    strings.TrimSpace(*body),
		"tickets": []string(tickets),
	}
	out, err := ledger.Append(coordination.Path(resolveTarget(*target)), ldgrLockPath(resolveTarget(*target)), row)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}

func validNoteKind(kind string) bool {
	switch kind {
	case "decision", "risk", "handoff", "broadcast":
		return true
	default:
		return false
	}
}
