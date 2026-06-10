package cmd

import (
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func migrateStringField(row map[string]any, key string) string {
	v, _ := row[key].(string)
	return v
}

func stringDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func stringDefaultPriority(v string) string {
	if _, ok := ledger.PriorityEnum[v]; ok {
		return v
	}
	return "P2"
}

func arrayField(row map[string]any, key string) []any {
	if v, ok := row[key].([]any); ok {
		return v
	}
	return []any{}
}

func numberAsPositiveInt(v any) (int, bool) {
	n, ok := v.(float64)
	if !ok || n <= 0 || n != float64(int(n)) {
		return 0, false
	}
	return int(n), true
}

func unknownFields(row ledger.Row, known map[string]struct{}) map[string]any {
	extra := map[string]any{}
	for k, v := range row {
		if _, ok := known[k]; ok {
			continue
		}
		extra[k] = v
	}
	return extra
}

func v1TicketKnownFields() map[string]struct{} {
	return map[string]struct{}{
		"n": {}, "ts": {}, "parent_ticket": {}, "ticket": {}, "agent": {}, "role": {},
		"status": {}, "task": {}, "scope": {}, "paths": {}, "blocked_by": {}, "branch": {},
		"decision": {}, "notes": {}, "category": {}, "kind": {}, "priority": {},
		"acceptance": {}, "evidence": {}, "audit_result": {}, "audit_notes": {}, "reviewed_n": {},
	}
}

func v1WorklogKnownFields() map[string]struct{} {
	return map[string]struct{}{
		"n": {}, "ts": {}, "ticket": {}, "agent": {}, "task": {}, "scope": {}, "result": {},
		"paths": {}, "commands": {}, "notes": {}, "branch": {}, "commit": {},
	}
}
