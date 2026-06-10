// Package guidance derives state-aware next-action guidance from ledger rows.
package guidance

type Warning struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // critical | warning | hint
	Message  string `json:"message"`
}

// Guidance is the wire shape for both stderr text rendering and the `next` JSON output.
type Guidance struct {
	Ticket            string    `json:"ticket,omitempty"`
	Status            string    `json:"status,omitempty"`
	ID                string    `json:"id,omitempty"`
	State             string    `json:"state,omitempty"`
	WritingLanguage   string    `json:"writing_language,omitempty"`
	Summary           string    `json:"summary"`
	Actions           []string  `json:"actions"`
	Warnings          []Warning `json:"warnings"`
	SuggestedCommands []string  `json:"suggested_commands"`
	SuggestedJSON     []any     `json:"suggested_json"`
	NextTransitions   []string  `json:"next_transitions"`
}
