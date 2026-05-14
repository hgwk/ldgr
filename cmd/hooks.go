package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Commands["hooks"] = RunHooksCLI
}

const (
	hookMarkerStart = "# >>> LDGR_HOOK_START >>>"
	hookMarkerEnd   = "# <<< LDGR_HOOK_END <<<"
	hookBackupSfx   = ".ldgr.bak"
)

func hookBlock() string {
	return hookMarkerStart + "\n" +
		"ldgr verify || exit 1\n" +
		hookMarkerEnd + "\n"
}

// RunHooksCLI implements `ldgr hooks install|uninstall`.
func RunHooksCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr hooks <install|uninstall> [--target PATH]")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("hooks " + sub)
	target := fs.String("target", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	dir := resolveTarget(*target)
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	switch sub {
	case "install":
		return runHooksInstall(hookPath, stdout, stderr)
	case "uninstall":
		return runHooksUninstall(hookPath, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown hooks subcommand: %s\n", sub)
		return 2
	}
}

func runHooksInstall(hookPath string, stdout, stderr io.Writer) int {
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	existing, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if bytes.Contains(existing, []byte(hookMarkerStart)) {
		fmt.Fprintln(stdout, "hooks already installed")
		return 0
	}
	if len(existing) > 0 {
		bak := hookPath + hookBackupSfx
		if _, err := os.Stat(bak); os.IsNotExist(err) {
			if err := os.WriteFile(bak, existing, 0o755); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
	}
	var content bytes.Buffer
	if len(existing) > 0 && bytes.HasPrefix(existing, []byte("#!")) {
		nl := bytes.IndexByte(existing, '\n')
		if nl < 0 {
			nl = len(existing)
		}
		content.Write(existing[:nl+1])
		content.WriteString(hookBlock())
		content.Write(existing[nl+1:])
	} else {
		content.WriteString("#!/usr/bin/env bash\n")
		content.WriteString(hookBlock())
		content.Write(existing)
	}
	if err := os.WriteFile(hookPath, content.Bytes(), 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "installed pre-commit hook at %s\n", hookPath)
	return 0
}

func runHooksUninstall(hookPath string, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(stdout, "no hook to uninstall")
			return 0
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	cleaned := removeHookBlock(string(data))
	trimmed := strings.TrimSpace(cleaned)
	if trimmed == "" || trimmed == "#!/usr/bin/env bash" {
		if err := os.Remove(hookPath); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		_ = os.Remove(hookPath + hookBackupSfx)
		fmt.Fprintln(stdout, "removed pre-commit hook")
		return 0
	}
	if err := os.WriteFile(hookPath, []byte(cleaned), 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	_ = os.Remove(hookPath + hookBackupSfx)
	fmt.Fprintln(stdout, "removed ldgr hook block; user content preserved")
	return 0
}

func removeHookBlock(s string) string {
	start := strings.Index(s, hookMarkerStart)
	if start < 0 {
		return s
	}
	end := strings.Index(s[start:], hookMarkerEnd)
	if end < 0 {
		return s
	}
	end = start + end + len(hookMarkerEnd)
	if end < len(s) && s[end] == '\n' {
		end++
	}
	return s[:start] + s[end:]
}
