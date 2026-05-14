package cmd

import (
	"fmt"
	"io"

	"github.com/hgwk/ldgr/internal/verify"
)

func init() {
	Commands["verify"] = RunVerifyCLI
}

func RunVerifyCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("verify")
	target := fs.String("target", "", "")
	strict := fs.Bool("strict", false, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)
	rep, err := verify.RunStrict(dir, *strict)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, w := range rep.Warns {
		fmt.Fprintf(stdout, "warn  %s:%d %s\n", w.File, w.Line, w.Message)
	}
	for _, f := range rep.Fails {
		fmt.Fprintf(stderr, "fail  %s:%d %s\n", f.File, f.Line, f.Message)
	}
	if len(rep.Fails) > 0 {
		return 1
	}
	if len(rep.Warns) == 0 && len(rep.Fails) == 0 {
		fmt.Fprintln(stdout, "ok")
	}
	return 0
}
