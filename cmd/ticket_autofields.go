package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/agent"
	"github.com/hgwk/ldgr/internal/gitutil"
)

func autoFields(dir string, in map[string]any, stderr io.Writer) (map[string]any, error) {
	if _, ok := in["ts"]; !ok || in["ts"] == "" {
		in["ts"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}
	if _, ok := in["branch"]; !ok {
		in["branch"] = gitutil.CurrentBranch(dir)
	}
	envMap := envAsMap()
	fromJSON, _ := in["agent"].(string)
	resolved, warn, err := agent.Resolve(fromJSON, envMap)
	if err != nil {
		return nil, err
	}
	in["agent"] = resolved
	if warn != "" {
		fmt.Fprintln(stderr, "warning:", warn)
	}
	return in, nil
}

func envAsMap() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				out[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return out
}

func requireFields(row map[string]any, required []string, kind string) error {
	var errs validationErrors
	for _, f := range required {
		v, ok := row[f]
		if !ok || v == nil {
			errs.Add(fmt.Sprintf("%s: missing required field %q", kind, f))
		}
	}
	return errs.Err()
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok && s == "" {
		return true
	}
	return false
}

func requireNonEmpty(row map[string]any, fields []string, kind string) error {
	var errs validationErrors
	for _, f := range fields {
		v, ok := row[f]
		if !ok {
			errs.Add(fmt.Sprintf("%s: missing required field %q", kind, f))
			continue
		}
		if v == nil {
			errs.Add(fmt.Sprintf("%s: field %q must be non-empty", kind, f))
			continue
		}
		s, isStr := v.(string)
		if !isStr || s == "" {
			errs.Add(fmt.Sprintf("%s: field %q must be non-empty", kind, f))
		}
	}
	return errs.Err()
}

type validationErrors []string

func (e *validationErrors) Add(message string) {
	if message != "" {
		*e = append(*e, message)
	}
}

func (e *validationErrors) AddErr(err error) {
	if err != nil {
		e.Add(err.Error())
	}
}

func (e validationErrors) Err() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

func (e validationErrors) Error() string {
	return strings.Join(e, "\n")
}
