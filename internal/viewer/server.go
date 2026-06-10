package viewer

import (
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/ids"
	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/registry"
	"github.com/hgwk/ldgr/internal/verify"
)

type Project struct {
	Config  config.Config
	Goal    ledger.Goal
	Tickets []ledger.Row
	Worklog []ledger.Row
	Display string
	Paths   []string
	Missing bool
}

// Server is the read-only HTTP front-end.
type Server struct {
	// ListProjects returns a (project_id → display + paths) snapshot.
	ListProjects func() ([]projectListEntry, error)
	// LoadProject resolves a project_id to its Project (or error/missing).
	LoadProject func(projectID string) (Project, error)
	// Now lets tests override the clock used by insights.
	Now func() time.Time
	// StaleHours overrides the default 24h staleness threshold.
	StaleHours int
	// RunVerify executes the verify package against a target dir. Overridable
	// for tests. Defaults to verify.RunStrict.
	RunVerify func(targetDir string, strict bool) (verify.Report, error)
	// VerifyTTL is how long verify results are cached per (project, strict)
	// key. Defaults to 30s.
	VerifyTTL time.Duration

	verifyCacheMu sync.Mutex
	verifyCache   map[string]verifyCacheEntry
}

type verifyCacheEntry struct {
	at     time.Time
	report verify.Report
}

type projectListEntry struct {
	ProjectID string
	Slug      string
	Name      string
	Paths     []string
}

// NewServer builds a Server backed by a global registry store.
func NewServer(store *registry.Store) *Server {
	return &Server{
		ListProjects: func() ([]projectListEntry, error) {
			r, err := store.Load()
			if err != nil {
				return nil, err
			}
			out := make([]projectListEntry, 0, len(r.Projects))
			for _, p := range r.Projects {
				out = append(out, projectListEntry{
					ProjectID: p.ProjectID, Slug: p.Slug, Name: p.Name, Paths: append([]string(nil), p.Paths...),
				})
			}
			return out, nil
		},
		LoadProject: func(projectID string) (Project, error) {
			r, err := store.Load()
			if err != nil {
				return Project{}, err
			}
			for _, p := range r.Projects {
				if p.ProjectID != projectID {
					continue
				}
				for _, path := range p.Paths {
					proj, err := loadProjectFromDir(path)
					if err == nil {
						proj.Display = ids.Display(p.Slug, p.ProjectID)
						proj.Paths = append([]string(nil), p.Paths...)
						return proj, nil
					}
				}
				return Project{Missing: true, Display: ids.Display(p.Slug, p.ProjectID), Paths: append([]string(nil), p.Paths...)}, nil
			}
			return Project{}, errNotFound
		},
		Now:        time.Now,
		StaleHours: 24,
		RunVerify:  verify.RunStrict,
		VerifyTTL:  30 * time.Second,
	}
}

// NewSingleProjectServer builds a Server that exposes only the project at targetDir.
func NewSingleProjectServer(targetDir string) (*Server, error) {
	proj, err := loadProjectFromDir(targetDir)
	if err != nil {
		return nil, err
	}
	proj.Display = ids.Display(proj.Config.Slug, proj.Config.ProjectID)
	proj.Paths = []string{targetDir}
	pid := proj.Config.ProjectID
	return &Server{
		ListProjects: func() ([]projectListEntry, error) {
			return []projectListEntry{{
				ProjectID: pid, Slug: proj.Config.Slug, Name: proj.Config.Name, Paths: []string{targetDir},
			}}, nil
		},
		LoadProject: func(id string) (Project, error) {
			if id != pid {
				return Project{}, errNotFound
			}
			// Reload from disk every time so tests + viewer pick up changes.
			fresh, err := loadProjectFromDir(targetDir)
			if err != nil {
				return Project{}, err
			}
			fresh.Display = proj.Display
			fresh.Paths = proj.Paths
			return fresh, nil
		},
		Now:        time.Now,
		StaleHours: 24,
	}, nil
}

var errNotFound = errors.New("project not found")

func loadProjectFromDir(dir string) (Project, error) {
	cfg, err := config.Load(filepath.Join(dir, "ledger", "config.json"))
	if err != nil {
		return Project{}, err
	}
	var goal ledger.Goal
	_ = jsonio.ReadJSON(filepath.Join(dir, "ledger", "goal.json"), &goal)
	tickets, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return Project{}, err
	}
	worklog, err := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))
	if err != nil {
		return Project{}, err
	}
	return Project{Config: cfg, Goal: goal, Tickets: tickets, Worklog: worklog}, nil
}
