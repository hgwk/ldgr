package cmd

import (
	"fmt"
	"io"
)

func suggestWorklogCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest worklog")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, worklog, dir, code := loadTicketContext(*target, *ticket, false, stderr)
	if code != 0 {
		return code
	}
	return suggestWorklog(latest, worklog, loadWritingLanguage(dir), stdout)
}

func suggestCommitCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest commit")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	allowUnaudited := fs.Bool("allow-unaudited", false, "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, worklog, dir, code := loadTicketContext(*target, *ticket, false, stderr)
	if code != 0 {
		return code
	}
	return suggestCommit(latest, worklog, *allowUnaudited, loadWritingLanguage(dir), stdout)
}

func suggestAuditCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest audit")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, worklog, dir, code := loadTicketContext(*target, *ticket, false, stderr)
	if code != 0 {
		return code
	}
	return suggestAudit(latest, worklog, loadWritingLanguage(dir), stdout)
}

func suggestCorrectionCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest correction")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	invalidatesN := fs.Int("invalidates-n", 0, "")
	notes := fs.String("notes", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return 2
	}
	if *invalidatesN <= 0 {
		fmt.Fprintln(stderr, "--invalidates-n is required (positive integer)")
		return 2
	}
	if isStateTarget(resolveTarget(*target)) {
		return suggestCorrectionState(*ticket, *invalidatesN, *notes, loadWritingLanguage(resolveTarget(*target)), stdout)
	}
	return suggestCorrection(*ticket, *invalidatesN, *notes, loadWritingLanguage(resolveTarget(*target)), stdout)
}

func suggestPlanCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest plan")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, _, dir, code := loadTicketContext(*target, *ticket, true, stderr)
	if code != 0 {
		return code
	}
	if isStateTarget(dir) {
		return suggestPlanState(latest, *ticket, loadWritingLanguage(dir), stdout)
	}
	return suggestPlan(latest, *ticket, loadWritingLanguage(dir), stdout)
}

func suggestPRCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest pr")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	allowUnaudited := fs.Bool("allow-unaudited", false, "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, worklog, dir, code := loadTicketContext(*target, *ticket, false, stderr)
	if code != 0 {
		return code
	}
	return suggestPR(latest, worklog, *allowUnaudited, loadWritingLanguage(dir), stdout)
}
