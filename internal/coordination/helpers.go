package coordination

import (
	"strconv"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func stringField(r ledger.Row, key string) string {
	v, _ := r[key].(string)
	return strings.TrimSpace(v)
}

func intField(r ledger.Row, key string) int {
	switch v := r[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}

func stringSliceField(r ledger.Row, key string) []string {
	arr, _ := r[key].([]any)
	out := []string{}
	for _, raw := range arr {
		s, _ := raw.(string)
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
