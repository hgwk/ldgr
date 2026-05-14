package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/ledger"
)

func init() {
	Commands["goal"] = func(args []string, stdout, stderr io.Writer) int {
		return RunGoalCLI(args, os.Stdin, stdout, stderr)
	}
}

func RunGoalCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr goal <set|show>")
		return 2
	}
	switch args[0] {
	case "set":
		return runGoalSet(args[1:], stdin, stdout, stderr)
	case "show":
		return runGoalShow(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown goal subcommand: %s\n", args[0])
		return 2
	}
}

func runGoalSet(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("goal set")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	logFlag := fs.Bool("log", false, "also append a worklog row recording the change")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	goalPath := filepath.Join(dir, "ledger", "goal.json")
	var existing ledger.Goal
	_ = jsonio.ReadJSON(goalPath, &existing)
	merged := mergeGoal(existing, input)
	merged.Updated = time.Now().UTC().Format(time.RFC3339)
	if merged.SchemaVersion == 0 {
		merged.SchemaVersion = 1
	}
	if err := jsonio.WriteJSON(goalPath, merged); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	cfg, _ := config.Load(filepath.Join(dir, "ledger", "config.json"))
	shouldLog := *logFlag || cfg.LogGoalChanges
	if shouldLog {
		row := map[string]any{
			"task":     "goal set",
			"scope":    "ledger",
			"result":   "Updated project goal snapshot.",
			"paths":    []any{"ledger/goal.json"},
			"commands": []any{"ldgr goal set"},
			"notes":    "",
		}
		row, err = autoFields(dir, row, stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		row["commit"] = ""
		// Check all required fields except 'n' which is assigned by Append
		required := make([]string, 0, len(ledger.WorklogRequired))
		for _, f := range ledger.WorklogRequired {
			if f != "n" {
				required = append(required, f)
			}
		}
		if err := requireFields(row, required, "worklog"); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := requireNonEmpty(row, ledger.WorklogNonEmpty, "worklog"); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if _, err := ledger.Append(filepath.Join(dir, "ledger", "worklog.jsonl"), filepath.Join(dir, "ledger", ".lock"), ledger.Row(row)); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(merged), stderr)
}

func runGoalShow(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("goal show")
	target := fs.String("target", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)
	data, err := os.ReadFile(filepath.Join(dir, "ledger", "goal.json"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	stdout.Write(data)
	return 0
}

func mergeGoal(existing ledger.Goal, input map[string]any) ledger.Goal {
	out := existing
	if v, ok := input["schema_version"].(float64); ok {
		out.SchemaVersion = int(v)
	}
	if v, ok := input["track"].(string); ok {
		out.Track = v
	}
	if v, ok := input["version"].(string); ok {
		out.Version = v
	}
	if v, ok := input["source_of_truth"].(string); ok {
		out.SourceOfTruth = v
	}
	if v, ok := input["summary"].(string); ok {
		out.Summary = v
	}
	if v, ok := input["success_criteria"].([]any); ok {
		out.SuccessCriteria = out.SuccessCriteria[:0]
		for _, s := range v {
			if str, ok := s.(string); ok {
				out.SuccessCriteria = append(out.SuccessCriteria, str)
			}
		}
	}
	return out
}
