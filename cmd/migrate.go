package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/legacy"
	"github.com/hgwk/ldgr/internal/verify"
)

func init() {
	Commands["migrate"] = RunMigrateCLI
}

func RunMigrateCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "legacy-to-v1" {
		fmt.Fprintln(stderr, "usage: ldgr migrate legacy-to-v1 --target PATH (--plan | --apply)")
		return 2
	}
	mode := args[0]
	fs := newFlagSet("migrate " + mode)
	target := fs.String("target", "", "")
	planFlag := fs.Bool("plan", false, "")
	applyFlag := fs.Bool("apply", false, "")
	backupFlag := fs.Bool("backup", true, "create ledger/.backup before rewriting")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *planFlag == *applyFlag {
		fmt.Fprintln(stderr, "specify exactly one of --plan or --apply")
		return 2
	}
	if !*backupFlag {
		fmt.Fprintf(stderr, "migrate %s requires --backup=true\n", mode)
		return 2
	}
	dir := resolveTarget(*target)
	plan, err := composeLegacyToCanonicalPlan(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *planFlag {
		renderMigratePlan(stdout, plan)
		return 0
	}
	if err := legacy.Apply(plan.legacyPlan(), legacy.ApplyOpts{BackupPrefix: "legacy-to-v1-"}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	report, err := verify.Run(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(report.Fails) > 0 {
		fmt.Fprintln(stderr, "migration wrote schema v1 files but verification failed; restore from ledger/.backup/")
		for _, fail := range report.Fails {
			fmt.Fprintf(stderr, "%s:%d %s %s\n", fail.File, fail.Line, fail.Code, fail.Message)
		}
		return 1
	}
	renderMigrateApply(stdout, plan, len(report.Warns))
	return 0
}

type migratePlan struct {
	targetDir      string
	changes        []legacy.Change
	warnings       []string
	warningSamples map[string][]string
	counts         migrateCounts
}

type migrateCounts struct {
	tickets              int
	worklogs             int
	weakDone             int
	weakRework           int
	ghostTickets         int
	ghostWorklogs        int
	typeDefaulted        int
	areaDefaulted        int
	roleDefaulted        int
	summaryDefaulted     int
	worklogTicketDefault int
	unmappedField        int
}

func (p migratePlan) legacyPlan() legacy.Plan {
	return legacy.Plan{TargetDir: p.targetDir, Changes: p.changes, Warnings: p.warnings}
}

func composeLegacyToCanonicalPlan(target string) (migratePlan, error) {
	cfgPath := filepath.Join(target, "ledger", "config.json")
	ticketsPath := filepath.Join(target, "ledger", "tickets.jsonl")
	worklogPath := filepath.Join(target, "ledger", "worklog.jsonl")
	if _, err := os.Stat(cfgPath); err != nil {
		return migratePlan{}, fmt.Errorf("migrate legacy-to-v1: missing ledger/config.json: %w", err)
	}
	tickets, err := ledger.ReadRows(ticketsPath)
	if err != nil {
		return migratePlan{}, err
	}
	worklogs, err := ledger.ReadRows(worklogPath)
	if err != nil {
		return migratePlan{}, err
	}
	if rowsNeedNoLegacyMigration(tickets, worklogs) {
		return migratePlan{targetDir: target}, nil
	}

	var counts migrateCounts
	samples := map[string][]string{}
	canonicalTickets := make([]ledger.Row, 0, len(tickets))
	for _, row := range tickets {
		if ticketRowIsCanonicalV1(row) {
			canonicalTickets = append(canonicalTickets, row)
			continue
		}
		canonical, rowCounts := mapLegacyTicketToCanonical(row)
		counts.add(rowCounts)
		addMigrationSamples(samples, "ticket", row, canonical, rowCounts)
		canonicalTickets = append(canonicalTickets, canonical)
	}
	canonicalWorklogs := make([]ledger.Row, 0, len(worklogs))
	for _, row := range worklogs {
		if worklogRowIsCanonicalV1(row) {
			canonicalWorklogs = append(canonicalWorklogs, row)
			continue
		}
		canonical, rowCounts := mapLegacyWorklogToCanonical(row)
		counts.add(rowCounts)
		addMigrationSamples(samples, "worklog", row, canonical, rowCounts)
		canonicalWorklogs = append(canonicalWorklogs, canonical)
	}
	cfgBytes, err := rewriteConfigSchemaVersion(cfgPath, 1)
	if err != nil {
		return migratePlan{}, err
	}
	ticketBytes, err := marshalJSONL(canonicalTickets)
	if err != nil {
		return migratePlan{}, err
	}
	worklogBytes, err := marshalJSONL(canonicalWorklogs)
	if err != nil {
		return migratePlan{}, err
	}
	plan := migratePlan{
		targetDir: target,
		changes: []legacy.Change{
			{OutputPath: "ledger/config.json", Action: legacy.ActionReplace, NewBytes: cfgBytes},
			{OutputPath: "ledger/tickets.jsonl", Action: legacy.ActionReplace, NewBytes: ticketBytes},
			{OutputPath: "ledger/worklog.jsonl", Action: legacy.ActionReplace, NewBytes: worklogBytes},
		},
		counts:         counts,
		warningSamples: samples,
	}
	plan.warnings = migrationWarnings(counts)
	return plan, nil
}

func (c *migrateCounts) add(o migrateCounts) {
	c.tickets += o.tickets
	c.worklogs += o.worklogs
	c.weakDone += o.weakDone
	c.weakRework += o.weakRework
	c.ghostTickets += o.ghostTickets
	c.ghostWorklogs += o.ghostWorklogs
	c.typeDefaulted += o.typeDefaulted
	c.areaDefaulted += o.areaDefaulted
	c.roleDefaulted += o.roleDefaulted
	c.summaryDefaulted += o.summaryDefaulted
	c.worklogTicketDefault += o.worklogTicketDefault
	c.unmappedField += o.unmappedField
}

func rowsNeedNoLegacyMigration(tickets, worklogs []ledger.Row) bool {
	for _, row := range tickets {
		if !ticketRowIsCanonicalV1(row) {
			return false
		}
	}
	for _, row := range worklogs {
		if !worklogRowIsCanonicalV1(row) {
			return false
		}
	}
	return true
}

func ticketRowIsCanonicalV1(row ledger.Row) bool {
	_, hasID := row["id"]
	_, hasState := row["state"]
	_, hasEvent := row["event"]
	return hasID && hasState && hasEvent
}

func worklogRowIsCanonicalV1(row ledger.Row) bool {
	_, hasActor := row["actor"]
	_, hasTitle := row["title"]
	_, hasSummary := row["summary"]
	return hasActor && hasTitle && hasSummary
}

func mapLegacyTicketToCanonical(row ledger.Row) (ledger.Row, migrateCounts) {
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
	if _, ok := ledger.CanonicalTypeEnum[typ]; !ok {
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
	if extra := unknownFields(row, v1TicketKnownFields()); len(extra) > 0 {
		event["extra"] = extra
		counts.unmappedField = len(extra)
	}
	return out, counts
}

func mapLegacyWorklogToCanonical(row ledger.Row) (ledger.Row, migrateCounts) {
	var counts migrateCounts
	counts.worklogs = 1
	title := migrateStringField(row, "task")
	n, hasN := numberAsPositiveInt(row["n"])
	if title == "" {
		title = migrateStringField(row, "ticket")
		if title == "" && hasN {
			title = fmt.Sprintf("Invalid historical worklog row %d", n)
			counts.ghostWorklogs = 1
		}
		counts.summaryDefaulted = 1
	}
	summary := migrateStringField(row, "result")
	if summary == "" {
		summary = title
		counts.summaryDefaulted = 1
	}
	ticket := migrateStringField(row, "ticket")
	if strings.TrimSpace(ticket) == "" {
		if hasN {
			ticket = fmt.Sprintf("invalid-worklog-row-%d", n)
		} else {
			ticket = "invalid-worklog-row"
		}
		counts.worklogTicketDefault = 1
	}
	out := ledger.Row{
		"n":        row["n"],
		"ts":       row["ts"],
		"ticket":   ticket,
		"actor":    stringDefault(migrateStringField(row, "agent"), "unknown"),
		"title":    title,
		"summary":  summary,
		"paths":    arrayField(row, "paths"),
		"commands": arrayField(row, "commands"),
		"notes":    stringField(row, "notes"),
	}
	if extra := unknownFields(row, v1WorklogKnownFields()); len(extra) > 0 {
		out["extra"] = extra
		counts.unmappedField = len(extra)
	}
	return out, counts
}

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

func rewriteConfigSchemaVersion(path string, version int) ([]byte, error) {
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

func migrationWarnings(c migrateCounts) []string {
	var warnings []string
	add := func(code string, n int) {
		if n > 0 {
			warnings = append(warnings, fmt.Sprintf("%s %d rows", code, n))
		}
	}
	add("WEAK_DONE_MAPPED_REVIEW", c.weakDone)
	add("WEAK_REWORK_MAPPED_REVIEW", c.weakRework)
	add("GHOST_TICKET_SYNTHESIZED", c.ghostTickets)
	add("GHOST_WORKLOG_SYNTHESIZED", c.ghostWorklogs)
	add("TYPE_DEFAULTED", c.typeDefaulted)
	add("AREA_DEFAULTED", c.areaDefaulted)
	add("ROLE_DEFAULTED", c.roleDefaulted)
	add("SUMMARY_DEFAULTED", c.summaryDefaulted)
	add("WORKLOG_TICKET_DEFAULTED", c.worklogTicketDefault)
	add("UNMAPPED_FIELD", c.unmappedField)
	sort.Strings(warnings)
	return warnings
}

func renderMigratePlan(w io.Writer, plan migratePlan) {
	fmt.Fprintln(w, "Schema v1 migration plan")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Target:")
	for _, c := range plan.changes {
		fmt.Fprintf(w, "  %s\t%s\n", c.OutputPath, actionName(c.Action))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Rows:")
	fmt.Fprintln(w, "  config    1")
	fmt.Fprintln(w, "  goal      unchanged")
	fmt.Fprintf(w, "  tickets   %d\n", plan.counts.tickets)
	fmt.Fprintf(w, "  worklog   %d\n", plan.counts.worklogs)
	if len(plan.warnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Warnings:")
		for _, warning := range plan.warnings {
			fmt.Fprintf(w, "  %s\n", warning)
			code := strings.Fields(warning)[0]
			for _, sample := range plan.warningSamples[code] {
				fmt.Fprintf(w, "    sample: %s\n", sample)
			}
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Apply:")
	fmt.Fprintln(w, "  ldgr migrate legacy-to-v1 --apply")
}

func renderMigrateApply(w io.Writer, plan migratePlan, verifyWarnings int) {
	for _, c := range plan.changes {
		fmt.Fprintf(w, "%s %s\n", actionName(c.Action), c.OutputPath)
	}
	fmt.Fprintf(w, "verify warnings %d\n", verifyWarnings)
}

func mapV1State(status string) string {
	switch status {
	case "open":
		return "ready"
	case "in_progress":
		return "doing"
	case "blocked":
		return "blocked"
	case "audit_ready":
		return "review"
	case "changes_requested":
		return "rework"
	case "done":
		return "done"
	case "cancelled":
		return "dropped"
	default:
		return "backlog"
	}
}

func mapV1Area(category string) string {
	switch category {
	case "doc":
		return "docs"
	case "test":
		return "test"
	case "ops", "chore":
		return "ops"
	case "bug":
		return "backend"
	case "feature", "refactor":
		return "backend"
	default:
		return ""
	}
}

func mapV1EventRole(role string) string {
	switch role {
	case "impl":
		return "implementer"
	case "audit":
		return "auditor"
	case "review":
		return "reviewer"
	case "design":
		return "planner"
	case "ops":
		return "operator"
	default:
		return ""
	}
}

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
