package cmd

import (
	"fmt"
	"io"
	"sort"

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
	rep, err := verify.RunStrict(dir, *strict)
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

	if len(rep.Fails) > 0 {
		return 1
	}
	if !*summary && !*verbose && len(rep.Warns) == 0 && len(rep.Fails) == 0 {
		fmt.Fprintln(stdout, "ok")
	}
	return 0
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
		"ORPHAN_WORKLOG",
		"PREMATURE_WORKLOG",
		"WEAK_DONE",
		"INVALID_TRANSITION",
		"AUDIT_REVIEWED_N_MISMATCH",
		"INVALIDATED_HISTORICAL":
		return true
	default:
		return false
	}
}
