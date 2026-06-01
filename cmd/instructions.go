package cmd

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Commands["instructions"] = RunInstructionsCLI
}

//go:embed instructions/*.md
var instructionFS embed.FS

func init() {
	// Initialized before other init() funcs
}

var (
	instructionBody = readEmbedFile("instructions/ldgr.md")
)

func readEmbedFile(name string) string {
	data, err := instructionFS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return string(data)
}

const (
	instrMarkerStart = "<!-- LDGR_START -->"
	instrMarkerEnd   = "<!-- LDGR_END -->"
	legacyStart      = "<!-- LEDGER_KIT_START -->"
	legacyEnd        = "<!-- LEDGER_KIT_END -->"
)

type instructionTarget struct {
	pointerFile string
}

func targets() []instructionTarget {
	return []instructionTarget{
		{"AGENTS.md"},
		{"CLAUDE.md"},
	}
}

const instructionBodyRel = "ledger/instructions/ldgr.md"

var legacyBodyRels = []string{
	"ledger/instructions/AGENTS.ldgr.md",
	"ledger/instructions/CLAUDE.ldgr.md",
}

// RunInstructionsCLI implements `ldgr instructions install|uninstall`.
func RunInstructionsCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr instructions <install|uninstall> [--target PATH] [--keep-bodies]")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("instructions " + sub)
	target := fs.String("target", "", "")
	keepBodies := fs.Bool("keep-bodies", false, "uninstall only: leave ledger/instructions/ldgr.md")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
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
	bodyPath := filepath.Join(dir, instructionBodyRel)
	if err := os.MkdirAll(filepath.Dir(bodyPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(bodyPath, []byte(instructionBody), 0o644); err != nil {
		return err
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
		updated := upsertPointer(current, instructionBodyRel)
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
			_ = os.Remove(filepath.Join(dir, instructionBodyRel))
			for _, oldRel := range legacyBodyRels {
				_ = os.Remove(filepath.Join(dir, oldRel))
			}
		}
	}
	if !keepBodies {
		_ = os.Remove(filepath.Join(dir, "ledger", "instructions"))
	}
	fmt.Fprintln(stdout, "instructions uninstalled")
	return 0
}

func upsertPointer(current, bodyRel string) string {
	if hasPointerPrelude(current, bodyRel) &&
		!strings.Contains(current, instrMarkerStart) &&
		!strings.Contains(current, legacyStart) &&
		!hasAnyLegacyPointerPrelude(current) {
		return current
	}

	pointer := "@" + bodyRel
	cleaned := removeKnownPointerPreludes(current)
	cleaned = removeBlock(cleaned, instrMarkerStart, instrMarkerEnd)
	cleaned = removeBlock(cleaned, legacyStart, legacyEnd)
	body := strings.TrimSpace(stripLeadingPointerSeparator(cleaned))
	if current == "" {
		return pointer + "\n"
	}
	if body == "" {
		return pointer + "\n"
	}
	return pointer + "\n\n---\n\n" + body + "\n"
}

func removeBlock(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return s
	}
	j := strings.Index(s[i:], end)
	if j < 0 {
		return s
	}
	cut := i + j + len(end)
	if cut < len(s) && s[cut] == '\n' {
		cut++
	}
	out := s[:i] + s[cut:]
	out = strings.TrimPrefix(out, "\n")
	return out
}

func hasPointerPrelude(content, bodyRel string) bool {
	return pointerPreludePosition(content, bodyRel) >= 0
}

func removePointerPrelude(content, bodyRel string) (string, bool) {
	lines := strings.Split(content, "\n")
	pos := pointerPreludePosition(content, bodyRel)
	if pos < 0 {
		return content, false
	}
	out := make([]string, 0, len(lines)-1)
	for i, line := range lines {
		if i != pos {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(stripLeadingPointerSeparator(strings.Join(out, "\n"))), true
}

func removeKnownPointerPreludes(content string) string {
	out := content
	for _, bodyRel := range append([]string{instructionBodyRel}, legacyBodyRels...) {
		var removed bool
		out, removed = removePointerPrelude(out, bodyRel)
		if removed {
			out = stripLeadingPointerSeparator(out)
		}
	}
	return out
}

func hasAnyLegacyPointerPrelude(content string) bool {
	for _, bodyRel := range legacyBodyRels {
		if hasPointerPrelude(content, bodyRel) {
			return true
		}
	}
	return false
}

func pointerPreludePosition(content, bodyRel string) int {
	ref := "@" + bodyRel
	for i, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == ref {
			for _, before := range strings.Split(content, "\n")[:i] {
				if strings.TrimSpace(before) != "" {
					return -1
				}
			}
			return i
		}
	}
	return -1
}

func stripLeadingPointerSeparator(content string) string {
	trimmed := strings.TrimLeft(content, " \t\r\n")
	if trimmed == "---" {
		return ""
	}
	if rest, ok := strings.CutPrefix(trimmed, "---\n"); ok {
		return strings.TrimLeft(rest, " \t\r\n")
	}
	return trimmed
}
