package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

type Options struct {
	Strict     bool
	ActiveOnly bool
	CodeSize   bool
}

func RunWithOptions(targetDir string, opts Options) (Report, error) {
	rep, err := runWith(targetDir, opts.Strict)
	if err != nil {
		return rep, err
	}
	if opts.ActiveOnly {
		rep = filterActiveOnly(targetDir, rep)
	}
	if opts.CodeSize {
		checkCodeSizeGuardrails(&rep, targetDir)
	}
	return rep, nil
}

func filterActiveOnly(targetDir string, rep Report) Report {
	tickets, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", "tickets.jsonl"))
	if err != nil {
		return rep
	}
	worklog, _ := ledger.ReadRows(filepath.Join(targetDir, "ledger", "worklog.jsonl"))
	stateMode := usesStateRows(tickets, worklog)
	activeTickets := map[int]struct{}{}
	activeIDs := map[string]struct{}{}
	for id, row := range latestTicketRows(tickets, stateMode) {
		if !isActiveClaimState(rowStatus(row, stateMode), stateMode) {
			continue
		}
		line, ok := numberAsInt(row["n"])
		if ok {
			activeTickets[line] = struct{}{}
		}
		activeIDs[id] = struct{}{}
	}
	activeWorklogs := latestWorklogLines(worklog, activeIDs)
	return Report{
		Fails: filterActiveIssues(rep.Fails, activeTickets, activeWorklogs),
		Warns: filterActiveIssues(rep.Warns, activeTickets, activeWorklogs),
	}
}

func latestWorklogLines(rows []ledger.Row, activeIDs map[string]struct{}) map[int]struct{} {
	latest := map[string]int{}
	for _, row := range rows {
		ticket := stringField(row, "ticket")
		if _, ok := activeIDs[ticket]; !ok {
			continue
		}
		line, ok := numberAsInt(row["n"])
		if !ok || line <= latest[ticket] {
			continue
		}
		latest[ticket] = line
	}
	out := map[int]struct{}{}
	for _, line := range latest {
		out[line] = struct{}{}
	}
	return out
}

func filterActiveIssues(in []Issue, ticketLines, worklogLines map[int]struct{}) []Issue {
	out := in[:0]
	for _, issue := range in {
		switch issue.File {
		case "ledger/tickets.jsonl":
			if _, ok := ticketLines[issue.Line]; ok {
				out = append(out, issue)
			}
		case "ledger/worklog.jsonl":
			if _, ok := worklogLines[issue.Line]; ok {
				out = append(out, issue)
			}
		default:
			out = append(out, issue)
		}
	}
	return out
}

type MigrationSummary struct {
	TicketRows        int
	WorklogRows       int
	StateTicketRows   int
	CompatTicketRows  int
	StateWorklogRows  int
	CompatWorklogRows int
	LegacyStates      map[string]int
	Compatibility     map[string]int
}

func BuildMigrationSummary(targetDir string, rep Report) (MigrationSummary, error) {
	tickets, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", "tickets.jsonl"))
	if err != nil {
		return MigrationSummary{}, err
	}
	worklog, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", "worklog.jsonl"))
	if err != nil {
		return MigrationSummary{}, err
	}
	sum := MigrationSummary{
		TicketRows:    len(tickets),
		WorklogRows:   len(worklog),
		LegacyStates:  map[string]int{},
		Compatibility: map[string]int{},
	}
	for _, row := range tickets {
		if isStateTicketRow(row) {
			sum.StateTicketRows++
		} else {
			sum.CompatTicketRows++
		}
		if state := stringField(row, "state"); isLegacyStateValue(state) {
			sum.LegacyStates[state]++
		}
	}
	for _, row := range worklog {
		if isStateWorklogRow(row) {
			sum.StateWorklogRows++
		} else {
			sum.CompatWorklogRows++
		}
	}
	for _, issue := range append(append([]Issue{}, rep.Warns...), rep.Fails...) {
		if isCompatibilityWarning(issue.Code) {
			sum.Compatibility[issue.Code]++
		}
	}
	return sum, nil
}

func FormatMigrationSummary(sum MigrationSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "migration report\n")
	fmt.Fprintf(&b, "tickets: %d state, %d compat, %d total\n", sum.StateTicketRows, sum.CompatTicketRows, sum.TicketRows)
	fmt.Fprintf(&b, "worklog: %d state, %d compat, %d total\n", sum.StateWorklogRows, sum.CompatWorklogRows, sum.WorklogRows)
	writeCountMap(&b, "legacy state values", sum.LegacyStates)
	writeCountMap(&b, "compatibility issues", sum.Compatibility)
	return strings.TrimRight(b.String(), "\n")
}

func writeCountMap(b *strings.Builder, title string, counts map[string]int) {
	if len(counts) == 0 {
		fmt.Fprintf(b, "%s: none\n", title)
		return
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Fprintf(b, "%s:\n", title)
	for _, key := range keys {
		fmt.Fprintf(b, "  %s %d\n", key, counts[key])
	}
}

func LocalVerifierScripts(targetDir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(targetDir, "package.json"))
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	var out []string
	for name, script := range pkg.Scripts {
		lowerName := strings.ToLower(name)
		lowerScript := strings.ToLower(script)
		if (!strings.Contains(lowerName, "ledger") && !strings.Contains(lowerName, "verify")) || !strings.Contains(lowerScript, "ledger") {
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s", name, script))
	}
	sort.Strings(out)
	return out, nil
}

func RunHrns(targetDir string) (string, int, error) {
	path, err := exec.LookPath("hrns")
	if err != nil {
		return "", 127, err
	}
	cmd := exec.Command(path, "audit", "--all")
	cmd.Dir = targetDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0, nil
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return string(out), exit.ExitCode(), nil
	}
	return string(out), 1, err
}
