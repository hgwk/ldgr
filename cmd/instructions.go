package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Commands["instructions"] = RunInstructionsCLI
}

type instructionTarget struct {
	pointerFile string
}

func targets() []instructionTarget {
	return []instructionTarget{
		{"AGENTS.md"},
		{"CLAUDE.md"},
	}
}

const instructionBodyRel = ".ldgr/instructions.md"

var legacyBodyRels = []string{
	instructionBodyRel,
	".ldgr/operating-guide.md",
	"ledger/instructions/ldgr.md",
	"ledger/instructions/AGENTS.ldgr.md",
	"ledger/instructions/CLAUDE.ldgr.md",
}

// RunInstructionsCLI implements `ldgr instructions install|uninstall`.
func RunInstructionsCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr instructions <install|uninstall> [--target PATH] [--home PATH] [--keep-bodies]")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("instructions " + sub)
	target := fs.String("target", "", "")
	home := fs.String("home", "", "ldgr home directory for operating guide (overrides LDGR_HOME)")
	keepBodies := fs.Bool("keep-bodies", false, "uninstall only: leave local legacy instruction bodies")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	restore, err := setLDGRHomeOverride(*home)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer restore()
	dir := resolveTarget(*target)
	switch sub {
	case "install":
		return runInstructionsInstall(dir, stdout, stderr)
	case "uninstall":
		return runInstructionsUninstall(dir, *keepBodies, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown instructions subcommand: %s\n", sub)
		return 2
	}
}

func runInstructionsInstall(dir string, stdout, stderr io.Writer) int {
	if err := installInstructions(dir); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, "instructions installed")
	return 0
}

func installInstructions(dir string) error {
	bodyPath, err := instructionBodyPath()
	if err != nil {
		return err
	}
	body, err := loadInstructionBody()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(bodyPath), 0o755); err != nil {
		return fmt.Errorf("create ldgr home for operating guide %s: %w; set LDGR_HOME or pass --home to a writable directory", filepath.Dir(bodyPath), err)
	}
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write ldgr operating guide %s: %w; set LDGR_HOME or pass --home to a writable directory", bodyPath, err)
	}
	for _, oldRel := range legacyBodyRels {
		_ = os.Remove(filepath.Join(dir, oldRel))
	}

	for _, t := range targets() {
		pointerPath := filepath.Join(dir, t.pointerFile)
		current := ""
		if data, err := os.ReadFile(pointerPath); err == nil {
			current = string(data)
		} else if !os.IsNotExist(err) {
			return err
		}
		updated := upsertPointer(current, bodyPath)
		if updated == current {
			continue
		}
		if err := os.WriteFile(pointerPath, []byte(updated), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func runInstructionsUninstall(dir string, keepBodies bool, stdout, stderr io.Writer) int {
	for _, t := range targets() {
		pointerPath := filepath.Join(dir, t.pointerFile)
		if data, err := os.ReadFile(pointerPath); err == nil {
			cleaned := removeKnownPointerPreludes(string(data))
			cleaned = removeBlock(cleaned, instrMarkerStart, instrMarkerEnd)
			cleaned = removeBlock(cleaned, legacyStart, legacyEnd)
			if strings.TrimSpace(cleaned) == "" {
				_ = os.Remove(pointerPath)
			} else {
				if err := os.WriteFile(pointerPath, []byte(cleaned), 0o644); err != nil {
					fmt.Fprintln(stderr, err)
					return 1
				}
			}
		}
		if !keepBodies {
			for _, oldRel := range legacyBodyRels {
				_ = os.Remove(filepath.Join(dir, oldRel))
			}
		}
	}
	if !keepBodies {
		_ = os.Remove(filepath.Join(dir, ".ldgr"))
		_ = os.Remove(filepath.Join(dir, "ledger", "instructions"))
	}
	fmt.Fprintln(stdout, "instructions uninstalled")
	return 0
}

func instructionBodyPath() (string, error) {
	home := os.Getenv("LDGR_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(h, ".ldgr")
	}
	return filepath.Join(home, "operating-guide.md"), nil
}
