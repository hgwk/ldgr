package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/agent"
	"github.com/hgwk/ldgr/internal/coordination"
	"github.com/hgwk/ldgr/internal/ledger"
)

func init() {
	Commands["claim"] = RunClaimCLI
}

func RunClaimCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprintln(stderr, "usage: ldgr claim <add|release> [flags]")
		return 2
	}
	switch args[0] {
	case "add":
		return runClaimAdd(args[1:], stdout, stderr)
	case "release":
		return runClaimRelease(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown claim subcommand: %s\n", args[0])
		return 2
	}
}

func runClaimAdd(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("claim add")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "ticket id")
	lane := fs.String("lane", "", "coordination lane")
	owner := fs.String("owner", "", "claim owner")
	team := fs.String("team", "", "team name")
	mode := fs.String("mode", "exclusive", "exclusive|shared")
	ttl := fs.Duration("ttl", 2*time.Hour, "claim lifetime")
	until := fs.String("until", "", "RFC3339 claim expiry")
	summary := fs.String("summary", "", "short claim summary")
	var resources stringListFlag
	fs.Var(&resources, "resource", "claimed resource/path; repeat or comma-separate")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *ticket == "" {
		fmt.Fprintln(stderr, "claim add: --ticket is required")
		return 2
	}
	if len(resources) == 0 {
		fmt.Fprintln(stderr, "claim add: at least one --resource is required")
		return 2
	}
	if *mode != "exclusive" && *mode != "shared" {
		fmt.Fprintln(stderr, "claim add: --mode must be exclusive or shared")
		return 2
	}
	dir := resolveTarget(*target)
	claimUntil, err := resolveClaimUntil(*until, *ttl)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if strings.TrimSpace(*owner) == "" {
		resolved, warn, err := agent.Resolve("", envAsMap())
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		*owner = resolved
		if warn != "" {
			fmt.Fprintln(stderr, "warning:", warn)
		}
	}
	row := ledger.Row{
		"type":        "claim",
		"id":          "claim-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		"ts":          time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"ticket":      strings.TrimSpace(*ticket),
		"lane":        strings.TrimSpace(*lane),
		"owner":       strings.TrimSpace(*owner),
		"team":        strings.TrimSpace(*team),
		"mode":        *mode,
		"resources":   []string(resources),
		"summary":     strings.TrimSpace(*summary),
		"claim_until": claimUntil,
	}
	return appendCoordinationRow(dir, row, stdout, stderr)
}

func runClaimRelease(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("claim release")
	target := fs.String("target", "", "")
	id := fs.String("id", "", "claim id")
	ticket := fs.String("ticket", "", "release all claims for ticket")
	summary := fs.String("summary", "", "release summary")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" && *ticket == "" {
		fmt.Fprintln(stderr, "claim release: --id or --ticket is required")
		return 2
	}
	row := ledger.Row{
		"type":     "release",
		"ts":       time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"claim_id": strings.TrimSpace(*id),
		"ticket":   strings.TrimSpace(*ticket),
		"summary":  strings.TrimSpace(*summary),
	}
	return appendCoordinationRow(resolveTarget(*target), row, stdout, stderr)
}

func resolveClaimUntil(until string, ttl time.Duration) (string, error) {
	if strings.TrimSpace(until) != "" {
		t, err := time.Parse(time.RFC3339Nano, until)
		if err != nil {
			return "", fmt.Errorf("claim add: invalid --until: %w", err)
		}
		return t.UTC().Format(time.RFC3339Nano), nil
	}
	if ttl <= 0 {
		return "", fmt.Errorf("claim add: --ttl must be positive")
	}
	return time.Now().UTC().Add(ttl).Format(time.RFC3339Nano), nil
}

func appendCoordinationRow(dir string, row ledger.Row, stdout, stderr io.Writer) int {
	out, err := ledger.Append(coordination.Path(dir), ldgrLockPath(dir), row)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}
