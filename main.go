package main

import (
	"os"

	"github.com/hgwk/ldgr/cmd"
)

func main() {
	os.Exit(cmd.Dispatch(os.Args[1:], os.Stdout, os.Stderr))
}
