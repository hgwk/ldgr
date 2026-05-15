package viewer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/ids"
	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/registry"
)

// Project bundles config + ledger data for one project.
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

// Handler returns the http.Handler for both /api/* and static assets.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/projects", s.handleProjects)
	mux.HandleFunc("/api/projects/", s.handleProjectSubroute)
	assets := http.FS(Assets())
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(assets)))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		f, err := Assets().Open("index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := io.Copy(w, f); err != nil {
			// best-effort
		}
	})
	return mux
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries, err := s.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		summary := map[string]any{
			"project_id":        e.ProjectID,
			"slug":              e.Slug,
			"name":              e.Name,
			"display":           ids.Display(e.Slug, e.ProjectID),
			"paths":             e.Paths,
			"goal_summary":      "",
			"open_tickets":      0,
			"recent_worklog_ts": "",
		}
		proj, err := s.LoadProject(e.ProjectID)
		if err != nil || proj.Missing {
			summary["missing"] = true
		} else {
			summary["goal_summary"] = proj.Goal.Summary
			counts := StatusCounts(LatestTickets(proj.Tickets))
			summary["open_tickets"] = activeTicketCount(counts)
			summary["recent_worklog_ts"] = recentWorklogTS(proj.Worklog)
		}
		out = append(out, summary)
	}
	writeJSON(w, out)
}

func (s *Server) handleProjectSubroute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	projectID := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	proj, err := s.LoadProject(projectID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if proj.Missing && sub != "" {
		http.NotFound(w, r)
		return
	}

	switch sub {
	case "":
		latest := LatestTickets(proj.Tickets)
		writeJSON(w, map[string]any{
			"project_id": proj.Config.ProjectID,
			"slug":       proj.Config.Slug,
			"name":       proj.Config.Name,
			"display":    proj.Display,
			"goal":       proj.Goal,
			"counts":     StatusCounts(latest),
		})
	case "goal":
		writeJSON(w, proj.Goal)
	case "tickets":
		latest := LatestTickets(proj.Tickets)
		writeJSON(w, map[string]any{"tree": Tree(latest)})
	case "worklog":
		rows := visibleWorklog(proj.Worklog)
		sort.SliceStable(rows, func(i, j int) bool {
			a, _ := rows[i]["ts"].(string)
			b, _ := rows[j]["ts"].(string)
			return a > b
		})
		writeJSON(w, map[string]any{"rows": rows})
	case "insights":
		now := s.Now()
		writeJSON(w, BuildInsights(proj.Tickets, proj.Worklog, now, s.StaleHours))
	case "dashboard":
		writeJSON(w, BuildDashboard(proj.Tickets, proj.Worklog, s.Now()))
	case "kanban":
		writeJSON(w, BuildKanban(proj.Tickets))
	case "audit-queue":
		latest := LatestTickets(proj.Tickets)
		writeJSON(w, map[string]any{"items": BuildAuditQueue(latest, s.Now())})
	default:
		// detect "tickets/<id>"
		if rest, ok := strings.CutPrefix(sub, "tickets/"); ok && rest != "" {
			s.handleTicketDetail(w, r, proj, rest)
			return
		}
		http.NotFound(w, r)
	}
}

func (s *Server) handleTicketDetail(w http.ResponseWriter, r *http.Request, proj Project, ticketID string) {
	// Collect all non-companion rows for this ticket, oldest first.
	history := make([]ledger.Row, 0)
	for _, row := range proj.Tickets {
		if id, _ := row["ticket"].(string); id != ticketID {
			continue
		}
		if _, isCompanion := row["invalidates_n"]; isCompanion {
			continue
		}
		history = append(history, row)
	}
	if len(history) == 0 {
		http.Error(w, "ticket not found", http.StatusNotFound)
		return
	}
	sort.SliceStable(history, func(i, j int) bool {
		ai, _ := history[i]["n"].(float64)
		bj, _ := history[j]["n"].(float64)
		return ai < bj
	})
	latest := history[len(history)-1]

	// Worklog rows for this ticket, newest first.
	var wl []ledger.Row
	wInvalidated := InvalidatedNs(proj.Worklog)
	for _, w := range proj.Worklog {
		if _, isCompanion := w["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := w["n"].(float64)
		if _, inv := wInvalidated[int(n)]; inv {
			continue
		}
		if id, _ := w["ticket"].(string); id == ticketID {
			wl = append(wl, w)
		}
	}
	sort.SliceStable(wl, func(i, j int) bool {
		a, _ := wl[i]["ts"].(string)
		b, _ := wl[j]["ts"].(string)
		return a > b
	})

	// If the latest row n is invalidated, surface the via_n.
	var invalidatedVia any = nil
	inv := InvalidatedNs(proj.Tickets)
	if n, _ := latest["n"].(float64); inv[int(n)] > 0 {
		invalidatedVia = inv[int(n)]
	}

	writeJSON(w, map[string]any{
		"ticket":          ticketID,
		"latest":          latest,
		"history":         history,
		"worklog":         wl,
		"invalidated_via": invalidatedVia,
	})
}

func activeTicketCount(counts map[string]int) int {
	return counts["open"] + counts["in_progress"] + counts["blocked"] + counts["audit_ready"] + counts["changes_requested"]
}

func visibleWorklog(rows []ledger.Row) []ledger.Row {
	invalid := InvalidatedNs(rows)
	out := make([]ledger.Row, 0, len(rows))
	for _, r := range rows {
		n, _ := r["n"].(float64)
		if _, isInvalid := invalid[int(n)]; isInvalid {
			continue
		}
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		out = append(out, r)
	}
	return out
}

func recentWorklogTS(rows []ledger.Row) string {
	var best string
	for _, r := range rows {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		ts, _ := r["ts"].(string)
		if ts > best {
			best = ts
		}
	}
	return best
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// silence unused imports in case the implementer trims one
var (
	_ = fmt.Sprintf
	_ = os.Stdout
	_ = sort.Slice
)
