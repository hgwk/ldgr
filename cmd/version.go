package cmd

import (
	"fmt"
	"io"
)

const Version = "0.3.5"

func RunVersionCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "usage: ldgr version")
		return 2
	}
	fmt.Fprintf(stdout, "ldgr %s\n", Version)
	return 0
}
