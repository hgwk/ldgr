package viewer

import (
	"net/http"
	"sort"

	"github.com/hgwk/ldgr/internal/ids"
)

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
			"project_id":         e.ProjectID,
			"slug":               e.Slug,
			"name":               e.Name,
			"display":            ids.Display(e.Slug, e.ProjectID),
			"paths":              e.Paths,
			"goal_summary":       "",
			"open_tickets":       0,
			"recent_worklog_ts":  "",
			"recent_activity_ts": "",
		}
		proj, err := s.LoadProject(e.ProjectID)
		if err != nil || proj.Missing {
			summary["missing"] = true
		} else {
			summary["goal_summary"] = proj.Goal.Summary
			counts := StatusCounts(LatestTickets(proj.Tickets))
			summary["open_tickets"] = activeTicketCount(counts)
			summary["recent_worklog_ts"] = recentWorklogTS(proj.Worklog)
			summary["recent_activity_ts"] = recentProjectActivityTS(proj.Tickets, proj.Worklog)
		}
		out = append(out, summary)
	}
	sort.SliceStable(out, func(i, j int) bool {
		im, _ := out[i]["missing"].(bool)
		jm, _ := out[j]["missing"].(bool)
		if im != jm {
			return !im
		}
		it, _ := out[i]["recent_activity_ts"].(string)
		jt, _ := out[j]["recent_activity_ts"].(string)
		if it != jt {
			return parseTS(it).After(parseTS(jt))
		}
		id, _ := out[i]["display"].(string)
		jd, _ := out[j]["display"].(string)
		return id < jd
	})
	writeJSON(w, out)
}
