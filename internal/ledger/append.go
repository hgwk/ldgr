package ledger

import (
	"encoding/json"
	"os"

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
