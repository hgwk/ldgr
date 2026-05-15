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

//go:embed instructions/*.ldgr.md
var instructionFS embed.FS

func init() {
	// Initialized before other init() funcs
}

var (
	agentsBody = readEmbedFile("instructions/AGENTS.ldgr.md")
	claudeBody = readEmbedFile("instructions/CLAUDE.ldgr.md")
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
	bodyRel     string
	body        string
}

func targets() []instructionTarget {
	return []instructionTarget{
		{"AGENTS.md", "ledger/instructions/AGENTS.ldgr.md", agentsBody},
		{"CLAUDE.md", "ledger/instructions/CLAUDE.ldgr.md", claudeBody},
	}
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
	keepBodies := fs.Bool("keep-bodies", false, "uninstall only: leave ledger/instructions/*.ldgr.md")
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
	for _, t := range targets() {
		bodyPath := filepath.Join(dir, t.bodyRel)
		if err := os.MkdirAll(filepath.Dir(bodyPath), 0o755); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := os.WriteFile(bodyPath, []byte(t.body), 0o644); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		pointerPath := filepath.Join(dir, t.pointerFile)
		current := ""
		if data, err := os.ReadFile(pointerPath); err == nil {
			current = string(data)
		} else if !os.IsNotExist(err) {
			fmt.Fprintln(stderr, err)
			return 1
		}
		updated := upsertPointer(current, t.bodyRel)
		if updated == current {
			continue
		}
		if err := os.WriteFile(pointerPath, []byte(updated), 0o644); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprintln(stdout, "instructions installed")
	return 0
}

func runInstructionsUninstall(dir string, keepBodies bool, stdout, stderr io.Writer) int {
	for _, t := range targets() {
		pointerPath := filepath.Join(dir, t.pointerFile)
		if data, err := os.ReadFile(pointerPath); err == nil {
			cleaned := removeBlock(string(data), instrMarkerStart, instrMarkerEnd)
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
			_ = os.Remove(filepath.Join(dir, t.bodyRel))
		}
	}
	if !keepBodies {
		_ = os.Remove(filepath.Join(dir, "ledger", "instructions"))
	}
	fmt.Fprintln(stdout, "instructions uninstalled")
	return 0
}

func upsertPointer(current, bodyRel string) string {
	pointer := instrMarkerStart + "\n" +
		"See [`" + bodyRel + "`](" + bodyRel + ") for the authoritative ldgr operating guide.\n" +
		"If local legacy ledger instructions below conflict, this ldgr guide wins.\n" +
		instrMarkerEnd + "\n"
	if i := strings.Index(current, legacyStart); i >= 0 {
		if j := strings.Index(current[i:], legacyEnd); j >= 0 {
			end := i + j + len(legacyEnd)
			if end < len(current) && current[end] == '\n' {
				end++
			}
			return current[:i] + pointer + current[end:]
		}
	}
	if i := strings.Index(current, instrMarkerStart); i >= 0 {
		if j := strings.Index(current[i:], instrMarkerEnd); j >= 0 {
			end := i + j + len(instrMarkerEnd)
			if end < len(current) && current[end] == '\n' {
				end++
			}
			return current[:i] + pointer + current[end:]
		}
	}
	if current == "" {
		return pointer
	}
	return pointer + "\n" + current
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
