// Package registry manages ~/.ldgr/registry.json. Spec §7.1.
package registry

import (
	"errors"
	"os"
	"time"

	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/locks"
)

type Project struct {
	ProjectID    string   `json:"project_id"`
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Paths        []string `json:"paths"`
	RegisteredAt string   `json:"registered_at"`
	LastSeen     string   `json:"last_seen"`
}

type Registry struct {
	Version  int       `json:"version"`
	Projects []Project `json:"projects"`
}

type Store struct {
	path string
	lock string
}

func New(path, lockPath string) *Store {
	return &Store{path: path, lock: lockPath}
}

// LockPath returns the path of the registry lock file.
func (s *Store) LockPath() string { return s.lock }

func (s *Store) Load() (Registry, error) {
	var r Registry
	err := jsonio.ReadJSON(s.path, &r)
	if errors.Is(err, os.ErrNotExist) {
		return Registry{Version: 1}, nil
	}
	if err != nil {
		return r, err
	}
	if r.Version == 0 {
		r.Version = 1
	}
	return r, nil
}

func (s *Store) save(r Registry) error {
	return jsonio.WriteJSON(s.path, r)
}

func (s *Store) Register(p Project) error {
	release, err := locks.Acquire(s.lock, locks.Options{})
	if err != nil {
		return err
	}
	defer release()

	r, err := s.Load()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	idx := indexByID(r.Projects, p.ProjectID)
	if idx == -1 {
		if p.RegisteredAt == "" {
			p.RegisteredAt = now
		}
		p.LastSeen = now
		p.Paths = dedupe(p.Paths)
		r.Projects = append(r.Projects, p)
	} else {
		existing := &r.Projects[idx]
		existing.Slug = nonEmpty(p.Slug, existing.Slug)
		existing.Name = nonEmpty(p.Name, existing.Name)
		existing.Paths = dedupe(append(existing.Paths, p.Paths...))
		existing.LastSeen = now
	}
	r.Version = 1
	return s.save(r)
}

func (s *Store) UnregisterPath(path string) error {
	release, err := locks.Acquire(s.lock, locks.Options{})
	if err != nil {
		return err
	}
	defer release()

	r, err := s.Load()
	if err != nil {
		return err
	}
	out := r.Projects[:0]
	for _, p := range r.Projects {
		kept := kept(p.Paths, path)
		if len(kept) == 0 {
			continue
		}
		p.Paths = kept
		out = append(out, p)
	}
	r.Projects = out
	return s.save(r)
}

func (s *Store) UnregisterID(id string) error {
	release, err := locks.Acquire(s.lock, locks.Options{})
	if err != nil {
		return err
	}
	defer release()

	r, err := s.Load()
	if err != nil {
		return err
	}
	out := r.Projects[:0]
	for _, p := range r.Projects {
		if p.ProjectID == id {
			continue
		}
		out = append(out, p)
	}
	r.Projects = out
	return s.save(r)
}

func indexByID(ps []Project, id string) int {
	for i, p := range ps {
		if p.ProjectID == id {
			return i
		}
	}
	return -1
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func kept(in []string, drop string) []string {
	out := in[:0]
	for _, v := range in {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
