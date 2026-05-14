// Package ids generates and formats project identifiers.
package ids

import (
	"crypto/rand"
	"encoding/hex"
)

// NewProjectID returns a 32-character lowercase hex string backed by
// 128 random bits from crypto/rand. The ledger spec forbids ULID due to
// stdlib-only constraints; sortability is not required here.
func NewProjectID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure on a healthy system is fatal: we have no
		// reasonable fallback. Panic so callers don't proceed with a
		// predictable id.
		panic("ids: crypto/rand failure: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// Display formats the human-visible project handle as "<slug>-<first 6 of id>".
func Display(slug, projectID string) string {
	if len(projectID) < 6 {
		return slug
	}
	return slug + "-" + projectID[:6]
}
