package verify

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const sourceLineBudget = 300

func checkCodeSizeGuardrails(rep *Report, targetDir string) {
	_ = filepath.WalkDir(targetDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if shouldSkipSizeDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isBudgetedSource(path) {
			return nil
		}
		lines, err := countLines(path)
		if err != nil || lines <= sourceLineBudget {
			return nil
		}
		rel := relFile(targetDir, path)
		rep.Warns = append(rep.Warns, Issue{
			File:    rel,
			Line:    sourceLineBudget + 1,
			Code:    "SOURCE_FILE_TOO_LONG",
			Message: "[SOURCE_FILE_TOO_LONG] source file exceeds 300 lines",
		})
		return nil
	})
}

func shouldSkipSizeDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "build", "coverage", ".next", "out", "target", "vendor":
		return true
	default:
		return false
	}
}

func isBudgetedSource(path string) bool {
	p := filepath.ToSlash(path)
	if strings.Contains(p, "/fixtures/") || strings.Contains(p, "/vendor/") {
		return false
	}
	for _, suffix := range []string{".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"} {
		if strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return false
}

func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	lines := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines++
	}
	return lines, scanner.Err()
}
