package cmd

import (
	"fmt"
	"io"

	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
)

func suggestPR(latest ledger.Row, worklog []ledger.Row, allowUnaudited bool, writingLanguage string, stdout io.Writer) int {
	if _, isState := latest["state"]; isState {
		return suggestPRState(latest, worklog, allowUnaudited, writingLanguage, stdout)
	}
	if !allowUnaudited && !ticketIsAuditPassDone(latest) {
		g := guidance.Compute(latest, worklog)
		g.WritingLanguage = writingLanguage
		fmt.Fprint(stdout, guidance.RenderText(g))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Run with --allow-unaudited to emit the PR scaffold anyway.")
		return 0
	}

	ticketID := stringField(latest, "ticket")
	task := stringField(latest, "task")
	truncatedTask := truncate(task, 60)

	fmt.Fprintf(stdout, "# PR: %s %s\n", ticketID, truncatedTask)
	fmt.Fprintln(stdout)
	printWritingLanguageHint(stdout, writingLanguage)
	fmt.Fprintln(stdout, "## Summary")
	fmt.Fprintf(stdout, "- %s\n", task)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "## Verification")
	evidence := stringSliceFromRow(latest, "evidence")
	if len(evidence) == 0 {
		fmt.Fprintln(stdout, "- No verification evidence recorded on the ticket.")
		fmt.Fprintln(stdout, "- Add ticket evidence before using this scaffold for merge or release notes.")
	} else {
		for _, e := range evidence {
			fmt.Fprintf(stdout, "- %s\n", e)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "## Related ticket")
	auditResult := stringField(latest, "audit_result")
	if auditResult == "" {
		auditResult = "pending"
	}
	fmt.Fprintf(stdout, "- %s (audit_result=%s)\n", ticketID, auditResult)
	return 0
}

func suggestPRState(latest ledger.Row, worklog []ledger.Row, allowUnaudited bool, writingLanguage string, stdout io.Writer) int {
	if !allowUnaudited && !ticketIsAuditPassDone(latest) {
		g := guidance.ComputeState(latest, worklog)
		g.WritingLanguage = writingLanguage
		fmt.Fprint(stdout, guidance.RenderText(g))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Run with --allow-unaudited to emit the PR scaffold anyway.")
		return 0
	}
	ticketID := stringField(latest, "id")
	title := stringField(latest, "title")
	fmt.Fprintf(stdout, "# PR: %s %s\n\n", ticketID, truncate(title, 60))
	printWritingLanguageHint(stdout, writingLanguage)
	fmt.Fprintln(stdout, "## Summary")
	fmt.Fprintf(stdout, "- %s\n\n", title)
	fmt.Fprintln(stdout, "## Verification")
	evidence := stringSliceFromRow(latest, "evidence")
	if len(evidence) == 0 {
		fmt.Fprintln(stdout, "- No verification evidence recorded on the ticket.")
		fmt.Fprintln(stdout, "- Add ticket evidence before using this scaffold for merge or release notes.")
	} else {
		for _, e := range evidence {
			fmt.Fprintf(stdout, "- %s\n", e)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "## Related ticket")
	result := "pending"
	if event, _ := latest["event"].(map[string]any); event != nil {
		if v, _ := event["result"].(string); v != "" {
			result = v
		}
	}
	fmt.Fprintf(stdout, "- %s (event.result=%s)\n", ticketID, result)
	return 0
}
