package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/verify"
)

func init() {
	Commands["verify"] = RunVerifyCLI
}

func RunVerifyCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("verify")
	target := fs.String("target", "", "")
	strict := fs.Bool("strict", false, "")
	summary := fs.Bool("summary", false, "")
	verbose := fs.Bool("verbose", false, "")
	newOnly := fs.Bool("new-only", false, "")
	activeOnly := fs.Bool("active-only", false, "")
	migrationReport := fs.Bool("migration-report", false, "")
	compareLocal := fs.Bool("compare-local", false, "")
	withHrns := fs.Bool("with-hrns", false, "")
	codeSize := fs.Bool("code-size", false, "warn on source files over 300 lines")
	sinceN := fs.Int("since-n", 0, "")
	sinceTicketN := fs.Int("since-ticket-n", 0, "")
	sinceWorklogN := fs.Int("since-worklog-n", 0, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *sinceN > 0 {
		if *sinceTicketN == 0 {
			*sinceTicketN = *sinceN
		}
		if *sinceWorklogN == 0 {
			*sinceWorklogN = *sinceN
		}
	}
	if *newOnly && *sinceTicketN <= 0 && *sinceWorklogN <= 0 {
		fmt.Fprintln(stderr, "--new-only requires --since-ticket-n <n> and/or --since-worklog-n <n>")
		return 2
	}
	dir := resolveTarget(*target)
	rep, err := verify.RunWithOptions(dir, verify.Options{Strict: *strict, ActiveOnly: *activeOnly, CodeSize: *codeSize})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *newOnly {
		rep.Warns = filterRowsAtOrBefore(rep.Warns, *sinceTicketN, *sinceWorklogN)
		rep.Fails = filterRowsAtOrBefore(rep.Fails, *sinceTicketN, *sinceWorklogN)
	}

	// Default + verbose: per-issue lines.
	if !*summary || *verbose {
		for _, w := range rep.Warns {
			fmt.Fprintf(stdout, "warn  %s:%d %s\n", w.File, w.Line, w.Message)
		}
		for _, f := range rep.Fails {
			fmt.Fprintf(stderr, "fail  %s:%d %s\n", f.File, f.Line, f.Message)
		}
	}

	// Summary + verbose: grouped table.
	if *summary || *verbose {
		printSummary(stdout, rep)
	}
	if *migrationReport {
		sum, err := verify.BuildMigrationSummary(dir, rep)
		if err != nil {
			fmt.Fprintf(stderr, "migration report: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, verify.FormatMigrationSummary(sum))
	}
	if *compareLocal {
		printLocalVerifierScripts(stdout, dir)
	}
	if *withHrns {
		if code := runHrns(stdout, stderr, dir); code != 0 && len(rep.Fails) == 0 {
			return code
		}
	}

	if len(rep.Fails) > 0 {
		return 1
	}
	if !*summary && !*verbose && len(rep.Warns) == 0 && len(rep.Fails) == 0 {
		fmt.Fprintln(stdout, "ok")
	}
	return 0
}

func printLocalVerifierScripts(stdout io.Writer, dir string) {
	scripts, err := verify.LocalVerifierScripts(dir)
	if err != nil || len(scripts) == 0 {
		fmt.Fprintln(stdout, "local verifier scripts: none")
		return
	}
	fmt.Fprintln(stdout, "local verifier scripts:")
	for _, script := range scripts {
		marker := ""
		if !strings.Contains(strings.ToLower(script), "ldgr verify") {
			marker = " (project-local)"
		}
		fmt.Fprintf(stdout, "  %s%s\n", script, marker)
	}
}

func runHrns(stdout, stderr io.Writer, dir string) int {
	out, code, err := verify.RunHrns(dir)
	if err != nil {
		fmt.Fprintf(stderr, "hrns: %v\n", err)
		return code
	}
	if strings.TrimSpace(out) != "" {
		fmt.Fprint(stdout, out)
		if !strings.HasSuffix(out, "\n") {
			fmt.Fprintln(stdout)
		}
	}
	return code
}

func filterRowsAtOrBefore(in []verify.Issue, sinceTicketN, sinceWorklogN int) []verify.Issue {
	out := in[:0]
	for _, x := range in {
		if keepIssueAfterBaseline(x, sinceTicketN, sinceWorklogN) {
			out = append(out, x)
		}
	}
	return out
}

func keepIssueAfterBaseline(x verify.Issue, sinceTicketN, sinceWorklogN int) bool {
	if x.Line == 0 {
		return true
	}
	switch x.File {
	case "ledger/tickets.jsonl":
		return sinceTicketN <= 0 || x.Line > sinceTicketN
	case "ledger/worklog.jsonl":
		return sinceWorklogN <= 0 || x.Line > sinceWorklogN
	default:
		return true
	}
}

func printSummary(w io.Writer, rep verify.Report) {
	fmt.Fprintf(w, "verify summary: %d fails, %d warns\n", len(rep.Fails), len(rep.Warns))
	grouped := map[string]int{}
	compatWarns := 0
	for _, x := range rep.Fails {
		grouped["fail "+x.Code]++
	}
	for _, x := range rep.Warns {
		grouped["warn "+x.Code]++
		if isLegacyCompatibilityWarning(x.Code) {
			compatWarns++
		}
	}
	type kv struct {
		k string
		v int
	}
	arr := make([]kv, 0, len(grouped))
	for k, v := range grouped {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].v != arr[j].v {
			return arr[i].v > arr[j].v
		}
		return arr[i].k < arr[j].k
	})
	for _, p := range arr {
		fmt.Fprintf(w, "%-30s %4d\n", p.k, p.v)
	}
	if compatWarns > 0 {
		fmt.Fprintf(w, "\nhistorical compatibility warnings %d\n", compatWarns)
		fmt.Fprintln(w, "  These are historical rows checked against current lifecycle/taxonomy gates.")
		fmt.Fprintln(w, "  For active append gates, use --new-only with --since-ticket-n/--since-worklog-n baselines.")
	}
}

func isLegacyCompatibilityWarning(code string) bool {
	switch code {
	case "MISSING_CATEGORY",
		"MISSING_REQUIRED",
		"NON_EMPTY_VIOLATION",
		"TS_NOT_INCREASING",
		"UNKNOWN_TYPE",
		"UNKNOWN_AREA",
		"UNKNOWN_PRIORITY",
		"LEGACY_STATE_VALUE",
		"UNKNOWN_STATUS",
		"UNKNOWN_EVENT_ROLE",
		"UNKNOWN_EVENT_RESULT",
		"MISSING_EVENT_FIELD",
		"EMPTY_EVENT_FIELD",
		"ORPHAN_WORKLOG",
		"PREMATURE_WORKLOG",
		"WEAK_DONE",
		"REWORK_WEAK",
		"INVALID_TRANSITION",
		"AUDIT_REVIEWED_N_MISMATCH",
		"INVALIDATED_HISTORICAL":
		return true
	default:
		return false
	}
}
