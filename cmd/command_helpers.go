package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func resolveTarget(target string) string {
	if target == "" {
		wd, _ := os.Getwd()
		return wd
	}
	abs, _ := filepath.Abs(target)
	return abs
}

func encErr(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
