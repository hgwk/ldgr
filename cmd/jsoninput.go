package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadJSONInput resolves --json values: '<inline>', '@-' (stdin), '@<path>'.
func ReadJSONInput(spec string, stdin io.Reader) (map[string]any, error) {
	if spec == "" {
		return nil, errors.New("--json is required")
	}
	var data []byte
	switch {
	case spec == "@-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, err
		}
		data = b
	case strings.HasPrefix(spec, "@"):
		b, err := os.ReadFile(spec[1:])
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", spec[1:], err)
		}
		data = b
	default:
		data = []byte(spec)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse --json: %w", err)
	}
	return out, nil
}
