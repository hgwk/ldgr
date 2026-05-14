// Package agent resolves the writer's identity using the priority defined
// in the spec §5.1: explicit JSON > LEDGER_AGENT > known env detection >
// $USER (with warning) > error.
package agent

import (
	"errors"
	"strings"
)

// ErrUnresolved is returned when no source produced an agent value.
var ErrUnresolved = errors.New("agent: could not resolve (set LEDGER_AGENT or include \"agent\" in input)")

// Resolve returns (agent, warning, error). warning is non-empty when a
// less-preferred source was used and the caller should surface it on stderr.
func Resolve(fromJSON string, env map[string]string) (string, string, error) {
	if fromJSON != "" {
		return fromJSON, "", nil
	}
	if v := env["LEDGER_AGENT"]; v != "" {
		return v, "", nil
	}
	if detected := detect(env); detected != "" {
		return detected, "", nil
	}
	if u := env["USER"]; u != "" {
		return u, "agent resolved from $USER; set LEDGER_AGENT to silence this warning", nil
	}
	return "", "", ErrUnresolved
}

func detect(env map[string]string) string {
	if hasPrefix(env, "CODEX_") {
		return "codex"
	}
	if _, ok := env["CLAUDECODE"]; ok {
		return "claude"
	}
	if hasPrefix(env, "CLAUDE_CODE_") {
		return "claude"
	}
	if hasPrefix(env, "CURSOR_") {
		return "cursor"
	}
	return ""
}

func hasPrefix(env map[string]string, prefix string) bool {
	for k := range env {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}
