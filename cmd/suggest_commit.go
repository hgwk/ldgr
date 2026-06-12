package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
)

func suggestCommit(latest ledger.Row, worklog []ledger.Row, allowUnaudited bool, writingLanguage string, stdout io.Writer) int {
	if _, isState := latest["state"]; isState {
		return suggestCommitState(latest, worklog, allowUnaudited, writingLanguage, stdout)
	}
	if !allowUnaudited && !ticketIsAuditPassDone(latest) {
		g := guidance.Compute(latest, worklog)
		g.WritingLanguage = writingLanguage
		fmt.Fprint(stdout, guidance.RenderText(g))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Run with --allow-unaudited to emit the commit scaffold anyway.")
		return 0
	}

	commitType := commitTypeFromCategory(stringField(latest, "category"))
	scope := strings.ToLower(stringField(latest, "parent_ticket"))
	if scope == "" || scope == "root" {
		scope = ""
	}
	subject := truncate(stringField(latest, "task"), 72)

	var line string
	if scope != "" {
		line = fmt.Sprintf("%s(%s): %s", commitType, scope, subject)
	} else {
		line = fmt.Sprintf("%s: %s", commitType, subject)
	}
	fmt.Fprintln(stdout, line)
	fmt.Fprintln(stdout)
	printWritingLanguageHint(stdout, writingLanguage)
	fmt.Fprintln(stdout, "## Summary")
	fmt.Fprintf(stdout, "- %s\n", stringField(latest, "task"))
	if notes := stringField(latest, "notes"); notes != "" {
		fmt.Fprintf(stdout, "- %s\n", notes)
	}
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
	return 0
}

func suggestCommitState(latest ledger.Row, worklog []ledger.Row, allowUnaudited bool, writingLanguage string, stdout io.Writer) int {
	if !allowUnaudited && !ticketIsAuditPassDone(latest) {
		g := guidance.ComputeState(latest, worklog)
		g.WritingLanguage = writingLanguage
		fmt.Fprint(stdout, guidance.RenderText(g))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Run with --allow-unaudited to emit the commit scaffold anyway.")
		return 0
	}
	commitType := commitTypeFromState(stringField(latest, "type"), stringField(latest, "area"))
	scope := strings.ToLower(stringField(latest, "parent"))
	if scope == "" || scope == "root" {
		scope = ""
	}
	subject := truncate(stringField(latest, "title"), 72)
	if scope != "" {
		fmt.Fprintf(stdout, "%s(%s): %s\n\n", commitType, scope, subject)
	} else {
		fmt.Fprintf(stdout, "%s: %s\n\n", commitType, subject)
	}
	printWritingLanguageHint(stdout, writingLanguage)
	fmt.Fprintln(stdout, "## Summary")
	fmt.Fprintf(stdout, "- %s\n\n", stringField(latest, "title"))
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
	return 0
}

func commitTypeFromState(kind, area string) string {
	switch kind {
	case "issue":
		return "fix"
	case "audit", "ops":
		return "chore"
	case "plan":
		return "docs"
	}
	switch area {
	case "docs":
		return "docs"
	case "test":
		return "test"
	case "infra", "ops", "release":
		return "chore"
	}
	return "feat"
}

func commitTypeFromCategory(cat string) string {
	switch cat {
	case "feature", "design", "demo":
		return "feat"
	case "bug":
		return "fix"
	case "docs", "research":
		return "docs"
	case "test":
		return "test"
	case "refactor", "cleanup":
		return "refactor"
	case "ops", "infra", "release":
		return "chore"
	}
	return "chore"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max])
}

func ifSliceField(r ledger.Row, k string) []any {
	v, _ := r[k].([]any)
	if v == nil {
		return []any{}
	}
	return v
}

func stringSliceFromRow(r ledger.Row, k string) []string {
	raw, _ := r[k].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func stringField(r ledger.Row, k string) string {
	v, _ := r[k].(string)
	return v
}
