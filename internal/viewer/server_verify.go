package viewer

import (
	"fmt"
	"net/http"
	"time"

	"github.com/hgwk/ldgr/internal/verify"
)

type verifySample struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
}

const verifySampleCap = 5

// handleVerify serves GET /api/projects/{id}/verify[?strict=1]. Results are
// cached per (project, strict) for VerifyTTL to keep the dashboard cheap.
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request, proj Project) {
	strict := r.URL.Query().Get("strict") == "1"
	if len(proj.Paths) == 0 {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("project has no source path"))
		return
	}
	// Pick the first resolvable path; same strategy as LoadProject.
	target := proj.Paths[0]

	pid := proj.Config.ProjectID
	if pid == "" {
		pid = target
	}
	key := pid + "|"
	if strict {
		key += "1"
	}
	ttl := s.VerifyTTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := s.Now()

	s.verifyCacheMu.Lock()
	if s.verifyCache == nil {
		s.verifyCache = map[string]verifyCacheEntry{}
	}
	ent, ok := s.verifyCache[key]
	s.verifyCacheMu.Unlock()

	var rep verify.Report
	var ranAt time.Time
	if ok && now.Sub(ent.at) < ttl {
		rep = ent.report
		ranAt = ent.at
	} else {
		runner := s.RunVerify
		if runner == nil {
			runner = verify.RunStrict
		}
		fresh, err := runner(target, strict)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		rep = fresh
		ranAt = now
		s.verifyCacheMu.Lock()
		s.verifyCache[key] = verifyCacheEntry{at: ranAt, report: rep}
		s.verifyCacheMu.Unlock()
	}

	byCode := map[string]int{}
	for _, is := range rep.Fails {
		byCode[is.Code]++
	}
	for _, is := range rep.Warns {
		byCode[is.Code]++
	}

	samples := make([]verifySample, 0, verifySampleCap)
	for _, is := range rep.Fails {
		if len(samples) >= verifySampleCap {
			break
		}
		samples = append(samples, verifySample{
			Code: is.Code, Severity: "fail", Message: is.Message, File: is.File, Line: is.Line,
		})
	}
	if len(samples) < verifySampleCap {
		for _, is := range rep.Warns {
			if len(samples) >= verifySampleCap {
				break
			}
			samples = append(samples, verifySample{
				Code: is.Code, Severity: "warning", Message: is.Message, File: is.File, Line: is.Line,
			})
		}
	}

	writeJSON(w, map[string]any{
		"strict":     strict,
		"fail_count": len(rep.Fails),
		"warn_count": len(rep.Warns),
		"by_code":    byCode,
		"samples":    samples,
		"ran_at":     ranAt.UTC().Format(time.RFC3339),
	})
}
