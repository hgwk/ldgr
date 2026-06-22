package coordination

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

const FileName = "coordination.jsonl"

type Claim struct {
	N          int      `json:"n,omitempty"`
	ID         string   `json:"id"`
	Ticket     string   `json:"ticket,omitempty"`
	Lane       string   `json:"lane,omitempty"`
	Owner      string   `json:"owner,omitempty"`
	Team       string   `json:"team,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	Resources  []string `json:"resources"`
	Summary    string   `json:"summary,omitempty"`
	TS         string   `json:"ts,omitempty"`
	ClaimUntil string   `json:"claim_until,omitempty"`
	Expired    bool     `json:"expired"`
	ExpiresIn  string   `json:"expires_in,omitempty"`
}

type Note struct {
	N       int      `json:"n,omitempty"`
	ID      string   `json:"id,omitempty"`
	Kind    string   `json:"kind"`
	Scope   string   `json:"scope,omitempty"`
	Ticket  string   `json:"ticket,omitempty"`
	Tickets []string `json:"tickets,omitempty"`
	Lane    string   `json:"lane,omitempty"`
	Team    string   `json:"team,omitempty"`
	Summary string   `json:"summary"`
	Body    string   `json:"body,omitempty"`
	TS      string   `json:"ts,omitempty"`
}

type Conflict struct {
	Resource string `json:"resource"`
	First    Claim  `json:"first"`
	Second   Claim  `json:"second"`
}

type Summary struct {
	Claims    []Claim    `json:"claims"`
	Notes     []Note     `json:"notes"`
	Conflicts []Conflict `json:"conflicts"`
}

func Path(dir string) string {
	return filepath.Join(dir, "ledger", FileName)
}

func ReadRows(dir string) ([]ledger.Row, error) {
	return ledger.ReadRows(Path(dir))
}

func BuildSummary(rows []ledger.Row, now time.Time) Summary {
	claims := activeClaims(rows, now)
	notes := recentNotes(rows, 20)
	return Summary{Claims: claims, Notes: notes, Conflicts: conflicts(claims)}
}

func activeClaims(rows []ledger.Row, now time.Time) []Claim {
	claims := map[string]Claim{}
	order := []string{}
	for _, row := range rows {
		switch stringField(row, "type") {
		case "claim":
			c := claimFromRow(row, now)
			if c.ID == "" || len(c.Resources) == 0 {
				continue
			}
			if _, seen := claims[c.ID]; !seen {
				order = append(order, c.ID)
			}
			claims[c.ID] = c
		case "release":
			id := stringField(row, "claim_id")
			if id != "" {
				delete(claims, id)
				continue
			}
			ticket := stringField(row, "ticket")
			if ticket == "" {
				continue
			}
			for cid, claim := range claims {
				if claim.Ticket == ticket {
					delete(claims, cid)
				}
			}
		}
	}
	out := []Claim{}
	for _, id := range order {
		if claim, ok := claims[id]; ok {
			out = append(out, claim)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Expired != out[j].Expired {
			return out[i].Expired
		}
		return out[i].ClaimUntil < out[j].ClaimUntil
	})
	return out
}

func recentNotes(rows []ledger.Row, limit int) []Note {
	out := []Note{}
	for _, row := range rows {
		if stringField(row, "type") != "note" {
			continue
		}
		note := Note{
			N:       intField(row, "n"),
			ID:      stringField(row, "id"),
			Kind:    stringField(row, "kind"),
			Scope:   stringField(row, "scope"),
			Ticket:  stringField(row, "ticket"),
			Tickets: stringSliceField(row, "tickets"),
			Lane:    stringField(row, "lane"),
			Team:    stringField(row, "team"),
			Summary: stringField(row, "summary"),
			Body:    stringField(row, "body"),
			TS:      stringField(row, "ts"),
		}
		if note.Kind == "" || note.Summary == "" {
			continue
		}
		out = append(out, note)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func conflicts(claims []Claim) []Conflict {
	out := []Conflict{}
	for i := 0; i < len(claims); i++ {
		for j := i + 1; j < len(claims); j++ {
			if claims[i].Mode == "shared" || claims[j].Mode == "shared" {
				continue
			}
			if claims[i].Ticket != "" && claims[i].Ticket == claims[j].Ticket {
				continue
			}
			for _, a := range claims[i].Resources {
				for _, b := range claims[j].Resources {
					if resourcesOverlap(a, b) {
						out = append(out, Conflict{Resource: shorterPath(a, b), First: claims[i], Second: claims[j]})
					}
				}
			}
		}
	}
	return out
}

func claimFromRow(row ledger.Row, now time.Time) Claim {
	c := Claim{
		N:          intField(row, "n"),
		ID:         stringField(row, "id"),
		Ticket:     stringField(row, "ticket"),
		Lane:       stringField(row, "lane"),
		Owner:      stringField(row, "owner"),
		Team:       stringField(row, "team"),
		Mode:       stringField(row, "mode"),
		Resources:  normalizeResources(stringSliceField(row, "resources")),
		Summary:    stringField(row, "summary"),
		TS:         stringField(row, "ts"),
		ClaimUntil: stringField(row, "claim_until"),
	}
	if c.Mode == "" {
		c.Mode = "exclusive"
	}
	if until, err := time.Parse(time.RFC3339Nano, c.ClaimUntil); err == nil {
		c.Expired = !until.After(now)
		c.ExpiresIn = durationLabel(until.Sub(now))
	}
	return c
}

func normalizeResources(in []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, raw := range in {
		v := normalizeResource(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func normalizeResource(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." || path == "/" {
		return ""
	}
	return path
}

func resourcesOverlap(a, b string) bool {
	a, b = normalizeResource(a), normalizeResource(b)
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func shorterPath(a, b string) string {
	if len(a) <= len(b) {
		return a
	}
	return b
}

func durationLabel(d time.Duration) string {
	prefix := "in "
	if d < 0 {
		prefix = ""
		d = -d
	}
	mins := int(d.Round(time.Minute).Minutes())
	if mins < 1 {
		mins = 1
	}
	if mins < 60 {
		if prefix == "" {
			return "expired " + itoa(mins) + "m ago"
		}
		return prefix + itoa(mins) + "m"
	}
	hours := (mins + 30) / 60
	if prefix == "" {
		return "expired " + itoa(hours) + "h ago"
	}
	return prefix + itoa(hours) + "h"
}
