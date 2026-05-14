package legacy

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/ledger"
)

// Scan returns the six well-known sources for targetDir. Sources that do
// not exist are returned with Exists=false so callers can render a stable
// plan report regardless of which subset is present.
func Scan(targetDir string) ([]Source, error) {
	specs := []struct {
		kind SourceKind
		rel  string
		goal bool
	}{
		{SourceLegacyTickets, "agent-tickets.jsonl", false},
		{SourceLegacyWorklog, "agent-worklog.jsonl", false},
		{SourceLegacyGoal, "goal.json", true},
		{SourceCurrentTickets, "ledger/tickets.jsonl", false},
		{SourceCurrentWorklog, "ledger/worklog.jsonl", false},
		{SourceCurrentGoal, "ledger/goal.json", true},
	}
	out := make([]Source, 0, len(specs))
	for _, sp := range specs {
		full := filepath.Join(targetDir, sp.rel)
		src := Source{Path: full, Kind: sp.kind}
		if _, err := os.Stat(full); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				out = append(out, src)
				continue
			}
			return nil, err
		}
		src.Exists = true
		if sp.goal {
			var g ledger.Goal
			if err := jsonio.ReadJSON(full, &g); err != nil {
				src.ParseErrs = append(src.ParseErrs, ParseError{Line: 0, Raw: full, Err: err.Error()})
			} else {
				src.Goal = &g
			}
		} else {
			rows, errs, err := readJSONL(full)
			if err != nil {
				return nil, err
			}
			src.Rows = rows
			src.ParseErrs = errs
		}
		out = append(out, src)
	}
	return out, nil
}

func readJSONL(path string) ([]ledger.Row, []ParseError, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var rows []ledger.Row
	var errs []ParseError
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var r ledger.Row
		if err := json.Unmarshal(raw, &r); err != nil {
			errs = append(errs, ParseError{Line: line, Raw: string(raw), Err: err.Error()})
			continue
		}
		rows = append(rows, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return rows, errs, nil
}
