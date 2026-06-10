package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func addMigrationSamples(samples map[string][]string, kind string, source, mapped ledger.Row, counts migrateCounts) {
	label := migrationSampleLabel(kind, source, mapped)
	add := func(code string, n int) {
		if n <= 0 || len(samples[code]) >= 3 {
			return
		}
		samples[code] = append(samples[code], label)
	}
	add("WEAK_DONE_MAPPED_REVIEW", counts.weakDone)
	add("WEAK_REWORK_MAPPED_REVIEW", counts.weakRework)
	add("GHOST_TICKET_SYNTHESIZED", counts.ghostTickets)
	add("GHOST_WORKLOG_SYNTHESIZED", counts.ghostWorklogs)
	add("TYPE_DEFAULTED", counts.typeDefaulted)
	add("AREA_DEFAULTED", counts.areaDefaulted)
	add("ROLE_DEFAULTED", counts.roleDefaulted)
	add("SUMMARY_DEFAULTED", counts.summaryDefaulted)
	add("WORKLOG_TICKET_DEFAULTED", counts.worklogTicketDefault)
	add("UNMAPPED_FIELD", counts.unmappedField)
}

func migrationSampleLabel(kind string, source, mapped ledger.Row) string {
	n, _ := numberAsPositiveInt(source["n"])
	id := migrateStringField(mapped, "id")
	if id == "" {
		id = migrateStringField(mapped, "ticket")
	}
	if id == "" {
		id = "?"
	}
	return fmt.Sprintf("%s n=%d id=%s", kind, n, id)
}

func rewriteConfigSchemaVersion(path string, version int, baseline historicalBaseline) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	delete(raw, "version")
	raw["schema_version"] = version
	if baseline.Tickets > 0 || baseline.Worklog > 0 {
		raw["historical_baseline"] = baseline
	}
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func marshalJSONL(rows []ledger.Row) ([]byte, error) {
	var b strings.Builder
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}
