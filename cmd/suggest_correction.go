package cmd

import (
	"encoding/json"
	"fmt"
	"io"
)

func suggestCorrection(ticket string, invalidatesN int, notes string, writingLanguage string, stdout io.Writer) int {
	skeleton := map[string]any{
		"ticket":        ticket,
		"role":          "ops",
		"status":        "cancelled",
		"invalidates_n": invalidatesN,
		"notes":         notes,
		"task":          fmt.Sprintf("invalidate n=%d", invalidatesN),
	}
	addWritingLanguage(skeleton, writingLanguage)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}

func suggestCorrectionState(ticket string, invalidatesN int, notes string, writingLanguage string, stdout io.Writer) int {
	skeleton := map[string]any{
		"id":            ticket,
		"state":         "dropped",
		"invalidates_n": invalidatesN,
		"event": map[string]any{
			"role":    "operator",
			"result":  "corrected",
			"summary": fmt.Sprintf("invalidate n=%d", invalidatesN),
			"notes":   notes,
		},
	}
	addWritingLanguage(skeleton, writingLanguage)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}
