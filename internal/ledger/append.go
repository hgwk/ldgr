package ledger

import (
	"encoding/json"
	"os"
	"time"

	"github.com/hgwk/ldgr/internal/locks"
)

// Append acquires lockPath, reads jsonlPath to determine the next n,
// writes the normalized row, and releases the lock. The returned Row is
// the normalized form (with n assigned). Caller is responsible for
// supplying ts and other auto-fields before calling Append; Append owns
// only n and the lock.
func Append(jsonlPath, lockPath string, row Row) (Row, error) {
	release, err := locks.Acquire(lockPath, locks.Options{})
	if err != nil {
		return nil, err
	}
	defer release()

	rows, err := ReadRows(jsonlPath)
	if err != nil {
		return nil, err
	}
	next := len(rows) + 1
	row["n"] = next
	ensureMonotonicTS(row, rows)

	data, err := json.Marshal(row)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(jsonlPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return nil, err
	}
	return row, f.Close()
}

func ensureMonotonicTS(row Row, rows []Row) {
	if len(rows) == 0 {
		return
	}
	ts, ok := row["ts"].(string)
	if !ok || ts == "" {
		return
	}
	lastTS, _ := rows[len(rows)-1]["ts"].(string)
	if lastTS == "" {
		return
	}
	last, err := time.Parse(time.RFC3339Nano, lastTS)
	if err != nil {
		return
	}
	cur, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil || cur.After(last) {
		return
	}
	row["ts"] = last.Add(time.Second).UTC().Format("2006-01-02T15:04:05Z")
}
