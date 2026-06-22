package cmd

import (
	"fmt"
	"strings"

	"github.com/hgwk/ldgr/internal/coordination"
)

func renderCoordinationText(sum coordination.Summary) string {
	if len(sum.Claims) == 0 && len(sum.Notes) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nCoordination\n")
	if len(sum.Conflicts) > 0 {
		fmt.Fprintf(&b, "  conflicts=%d\n", len(sum.Conflicts))
		for _, c := range sum.Conflicts {
			fmt.Fprintf(&b, "  ! %s: %s ↔ %s (%s)\n", c.Resource, claimLabel(c.First), claimLabel(c.Second), c.First.Mode)
		}
	}
	if len(sum.Claims) > 0 {
		fmt.Fprintf(&b, "  active claims=%d\n", len(sum.Claims))
		for _, claim := range firstClaims(sum.Claims, 5) {
			status := claim.ExpiresIn
			if claim.Expired {
				status = "stale " + status
			}
			fmt.Fprintf(&b, "  - %s %s [%s] %s\n", claimLabel(claim), strings.Join(claim.Resources, ","), claim.Mode, status)
		}
	}
	if len(sum.Notes) > 0 {
		fmt.Fprintf(&b, "  recent notes=%d\n", len(sum.Notes))
		for _, note := range firstNotes(sum.Notes, 5) {
			fmt.Fprintf(&b, "  - %s %s: %s\n", note.Kind, note.Scope, note.Summary)
		}
	}
	return b.String()
}

func claimLabel(c coordination.Claim) string {
	parts := []string{}
	if c.Ticket != "" {
		parts = append(parts, c.Ticket)
	}
	if c.Lane != "" {
		parts = append(parts, c.Lane)
	}
	if c.Owner != "" {
		parts = append(parts, c.Owner)
	}
	if len(parts) == 0 {
		return c.ID
	}
	return strings.Join(parts, "/")
}

func firstClaims(in []coordination.Claim, n int) []coordination.Claim {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

func firstNotes(in []coordination.Note, n int) []coordination.Note {
	if len(in) <= n {
		return in
	}
	return in[:n]
}
