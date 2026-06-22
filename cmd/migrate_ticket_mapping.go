package cmd

import (
	"fmt"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func mapLegacyTicketToState(row ledger.Row) (ledger.Row, migrateCounts) {
	var counts migrateCounts
	counts.tickets = 1
	id := migrateStringField(row, "ticket")
	if strings.TrimSpace(id) == "" {
		if n, ok := numberAsPositiveInt(row["n"]); ok {
			id = fmt.Sprintf("invalid-ticket-row-%d", n)
		} else {
			id = "invalid-ticket-row"
		}
		counts.ghostTickets = 1
	}
	title := migrateStringField(row, "task")
	if strings.TrimSpace(title) == "" {
		if counts.ghostTickets > 0 {
			title = "Invalid historical ticket row"
		} else {
			title = id
		}
		counts.summaryDefaulted = 1
	}
	typ := migrateStringField(row, "kind")
	if _, ok := ledger.TicketTypeEnum[typ]; !ok {
		typ = "task"
		counts.typeDefaulted = 1
	}
	area := mapV1Area(migrateStringField(row, "category"))
	if area == "" {
		area = "ops"
		counts.areaDefaulted = 1
	}
	role := mapV1EventRole(migrateStringField(row, "role"))
	if role == "" {
		role = "implementer"
		counts.roleDefaulted = 1
	}
	state := mapV1State(migrateStringField(row, "status"))
	if counts.ghostTickets > 0 {
		state = "dropped"
	}
	evidence := arrayField(row, "evidence")
	event := ledger.Row{
		"actor":   stringDefault(migrateStringField(row, "agent"), "unknown"),
		"role":    role,
		"summary": title,
		"notes":   migrateStringField(row, "notes"),
	}
	if decision := migrateStringField(row, "decision"); decision != "" {
		event["summary"] = decision
	}
	if state == "done" {
		if migrateStringField(row, "role") == "audit" && migrateStringField(row, "audit_result") == "pass" && len(evidence) > 0 {
			event["role"] = "auditor"
			event["result"] = "pass"
			if reviewed, ok := numberAsPositiveInt(row["reviewed_n"]); ok {
				event["reviewed_n"] = reviewed
			} else if n, ok := numberAsPositiveInt(row["n"]); ok && n > 1 {
				event["reviewed_n"] = n - 1
			} else {
				state = "review"
				counts.weakDone = 1
			}
		} else {
			state = "review"
			counts.weakDone = 1
		}
	}
	if state == "rework" {
		if reviewed, ok := numberAsPositiveInt(row["reviewed_n"]); ok {
			event["role"] = "auditor"
			event["result"] = "changes_requested"
			event["reviewed_n"] = reviewed
			if event["notes"] == "" {
				event["notes"] = migrateStringField(row, "audit_notes")
			}
		} else {
			state = "review"
			counts.weakRework = 1
		}
	}
	out := ledger.Row{
		"n":          row["n"],
		"ts":         row["ts"],
		"id":         id,
		"parent":     stringDefault(migrateStringField(row, "parent_ticket"), "ROOT"),
		"type":       typ,
		"state":      state,
		"area":       area,
		"priority":   stringDefaultPriority(migrateStringField(row, "priority")),
		"title":      title,
		"owner":      stringDefault(migrateStringField(row, "agent"), "unknown"),
		"blocked_by": arrayField(row, "blocked_by"),
		"acceptance": arrayField(row, "acceptance"),
		"evidence":   evidence,
		"event":      event,
	}
	if team := migrateStringField(row, "team"); team != "" {
		out["team"] = team
	}
	if extra := unknownFields(row, v1TicketKnownFields()); len(extra) > 0 {
		event["extra"] = extra
		counts.unmappedField = len(extra)
	}
	return out, counts
}
