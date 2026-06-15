package cmd

import "strings"

const (
	instrMarkerStart = "<!-- LDGR_START -->"
	instrMarkerEnd   = "<!-- LDGR_END -->"
	legacyStart      = "<!-- LEDGER_KIT_START -->"
	legacyEnd        = "<!-- LEDGER_KIT_END -->"
)

func upsertPointer(current, bodyRel string) string {
	if hasPointerPrelude(current, bodyRel) &&
		!strings.Contains(current, instrMarkerStart) &&
		!strings.Contains(current, legacyStart) &&
		!hasAnyLegacyPointerPrelude(current) {
		return current
	}

	pointer := "@" + bodyRel
	cleaned := removeKnownPointerPreludes(current)
	cleaned = removeBlock(cleaned, instrMarkerStart, instrMarkerEnd)
	cleaned = removeBlock(cleaned, legacyStart, legacyEnd)
	body := strings.TrimSpace(stripLeadingPointerSeparator(cleaned))
	if current == "" || body == "" {
		return pointer + "\n"
	}
	return pointer + "\n\n---\n\n" + body + "\n"
}

func removeBlock(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return s
	}
	j := strings.Index(s[i:], end)
	if j < 0 {
		return s
	}
	cut := i + j + len(end)
	if cut < len(s) && s[cut] == '\n' {
		cut++
	}
	out := s[:i] + s[cut:]
	return strings.TrimPrefix(out, "\n")
}

func hasPointerPrelude(content, bodyRel string) bool {
	return pointerPreludePosition(content, bodyRel) >= 0
}

func removePointerPrelude(content, bodyRel string) (string, bool) {
	lines := strings.Split(content, "\n")
	pos := pointerPreludePosition(content, bodyRel)
	if pos < 0 {
		return content, false
	}
	out := make([]string, 0, len(lines)-1)
	for i, line := range lines {
		if i != pos {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(stripLeadingPointerSeparator(strings.Join(out, "\n"))), true
}

func removeKnownPointerPreludes(content string) string {
	out := content
	for _, bodyRel := range knownPointerRefs() {
		var removed bool
		out, removed = removePointerPrelude(out, bodyRel)
		if removed {
			out = stripLeadingPointerSeparator(out)
		}
	}
	return out
}

func hasAnyLegacyPointerPrelude(content string) bool {
	for _, bodyRel := range legacyBodyRels {
		if hasPointerPrelude(content, bodyRel) {
			return true
		}
	}
	return false
}

func knownPointerRefs() []string {
	refs := []string{}
	if bodyPath, err := instructionBodyPath(); err == nil {
		refs = append(refs, bodyPath)
	}
	return append(refs, legacyBodyRels...)
}

func pointerPreludePosition(content, bodyRel string) int {
	ref := "@" + bodyRel
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == ref {
			for _, before := range lines[:i] {
				if strings.TrimSpace(before) != "" {
					return -1
				}
			}
			return i
		}
	}
	return -1
}

func stripLeadingPointerSeparator(content string) string {
	trimmed := strings.TrimLeft(content, " \t\r\n")
	if trimmed == "---" {
		return ""
	}
	if rest, ok := strings.CutPrefix(trimmed, "---\n"); ok {
		return strings.TrimLeft(rest, " \t\r\n")
	}
	return trimmed
}
