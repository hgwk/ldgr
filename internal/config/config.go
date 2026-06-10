// Package config handles ledger/config.json: the per-repo operational
// metadata. Spec §3.2.
package config

import (
	"encoding/json"
	"os"

	"github.com/hgwk/ldgr/internal/jsonio"
)

type Config struct {
	SchemaVersion    int      `json:"schema_version"`
	ProjectID        string   `json:"project_id"`
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Parents          []string `json:"parents"`
	BranchConvention string   `json:"branch_convention"`
	LogGoalChanges   bool     `json:"log_goal_changes"`
	WritingLanguage  string   `json:"writing_language,omitempty"`
	Status           string   `json:"status,omitempty"`
}

type rawConfig struct {
	SchemaVersion    int             `json:"schema_version"`
	Version          int             `json:"version"`
	ProjectID        string          `json:"project_id"`
	Slug             string          `json:"slug"`
	Name             string          `json:"name"`
	Parents          json.RawMessage `json:"parents"`
	BranchConvention string          `json:"branch_convention"`
	LogGoalChanges   bool            `json:"log_goal_changes"`
	WritingLanguage  string          `json:"writing_language"`
	Status           string          `json:"status"`
}

type legacyParent struct {
	Ticket string `json:"ticket"`
}

// DefaultParents is the seed parent set per spec §3.2.
var DefaultParents = []string{"ROOT", "DOC", "FE", "BE", "OPS", "DEMO", "BUG", "LEGACY"}

// Default builds a fresh Config for a new project. name falls back to slug
// when empty.
func Default(slug, projectID, name string) Config {
	if name == "" {
		name = slug
	}
	return Config{
		SchemaVersion:    1,
		ProjectID:        projectID,
		Slug:             slug,
		Name:             name,
		Parents:          append([]string(nil), DefaultParents...),
		BranchConvention: "work/{ticket}",
		LogGoalChanges:   false,
		WritingLanguage:  "",
	}
}

func Load(path string) (Config, error) {
	var raw rawConfig
	if err := jsonio.ReadJSON(path, &raw); err != nil {
		return Config{}, err
	}
	c := Config{
		SchemaVersion:    raw.SchemaVersion,
		ProjectID:        raw.ProjectID,
		Slug:             raw.Slug,
		Name:             raw.Name,
		BranchConvention: raw.BranchConvention,
		LogGoalChanges:   raw.LogGoalChanges,
		WritingLanguage:  raw.WritingLanguage,
		Status:           raw.Status,
	}
	if c.SchemaVersion == 0 && raw.Version != 0 {
		c.SchemaVersion = raw.Version
	}
	if len(raw.Parents) > 0 && string(raw.Parents) != "null" {
		if err := json.Unmarshal(raw.Parents, &c.Parents); err != nil || hasEmptyParent(c.Parents) {
			var legacy []legacyParent
			if legacyErr := json.Unmarshal(raw.Parents, &legacy); legacyErr != nil {
				if err != nil {
					return Config{}, err
				}
				return Config{}, legacyErr
			}
			c.Parents = nil
			for _, p := range legacy {
				if p.Ticket != "" {
					c.Parents = append(c.Parents, p.Ticket)
				}
			}
		}
	}
	return c, nil
}

func hasEmptyParent(parents []string) bool {
	for _, p := range parents {
		if p == "" {
			return true
		}
	}
	return false
}

func Save(path string, c Config) error {
	return jsonio.WriteJSON(path, c)
}

// SchemaVersion returns the config schema version. Missing legacy versions are
// treated as schema v1 for compatibility.
func SchemaVersion(path string) (int, error) {
	cfg, err := Load(path)
	if err != nil {
		return 0, err
	}
	if cfg.SchemaVersion == 0 {
		return 1, nil
	}
	return cfg.SchemaVersion, nil
}

// PatchWritingLanguage updates only the writing_language key in an existing
// config.json, preserving unknown legacy/project-specific fields.
func PatchWritingLanguage(path, language string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	value, err := json.Marshal(language)
	if err != nil {
		return err
	}
	raw["writing_language"] = value
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}

func PatchStatus(path, status string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	value, err := json.Marshal(status)
	if err != nil {
		return err
	}
	raw["status"] = value
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}
