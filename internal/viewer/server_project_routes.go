package viewer

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/coordination"
)

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
	case "coordination":
		writeJSON(w, coordination.BuildSummary(proj.Coord, s.Now()))
	case "audit-queue":
		latest := LatestTickets(proj.Tickets)
		writeJSON(w, map[string]any{"items": BuildAuditQueue(latest, s.Now())})
	case "verify":
		s.handleVerify(w, r, proj)
	default:
		// detect "tickets/<id>"
		if rest, ok := strings.CutPrefix(sub, "tickets/"); ok && rest != "" {
			s.handleTicketDetail(w, r, proj, rest)
			return
		}
		http.NotFound(w, r)
	}
}
