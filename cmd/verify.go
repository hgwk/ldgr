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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)
	rep, err := verify.RunStrict(dir, *strict)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *newOnly {
		rep.Warns = filterOut(rep.Warns, "INVALIDATED_HISTORICAL")
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

func filterOut(in []verify.Issue, code string) []verify.Issue {
	out := in[:0]
	for _, x := range in {
		if x.Code != code {
			out = append(out, x)
		}
	}
	return out
}

func printSummary(w io.Writer, rep verify.Report) {
	fmt.Fprintf(w, "verify summary: %d fails, %d warns\n", len(rep.Fails), len(rep.Warns))
	grouped := map[string]int{}
	for _, x := range rep.Fails {
		grouped["fail "+x.Code]++
	}
	for _, x := range rep.Warns {
		grouped["warn "+x.Code]++
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
}
