package ledger

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ReadRows reads a JSONL file and returns one Row per non-empty line.
// A missing file is treated as zero rows (not an error). Parse errors
// include the 1-based line number in the message.
func ReadRows(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Tickets/worklog lines can be larger than the default 64KiB buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var rows []Row
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var r Row
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("parse error at line %d: %w", line, err)
		}
		rows = append(rows, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}
