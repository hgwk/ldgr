package viewer

import (
	"net/http"
	"testing"

	"github.com/hgwk/ldgr/internal/verify"
)

func TestServer_VerifyEndpointSuccess(t *testing.T) {
	srv, pid := newTestServer(t)
	// Inject a stub verifier that returns warns only (default mode).
	srv.RunVerify = func(target string, strict bool) (verify.Report, error) {
		return verify.Report{
			Warns: []verify.Issue{
				{Code: "WEAK_DONE", Message: "weak A", File: "ledger/tickets.jsonl", Line: 1},
				{Code: "WEAK_DONE", Message: "weak B"},
				{Code: "ORPHAN_WORKLOG", Message: "orphan"},
			},
		}, nil
	}
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/verify", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	if resp["strict"] != false {
		t.Fatalf("strict=%v", resp["strict"])
	}
	if resp["fail_count"] != float64(0) || resp["warn_count"] != float64(3) {
		t.Fatalf("counts wrong: %+v", resp)
	}
	bc, _ := resp["by_code"].(map[string]any)
	if bc["WEAK_DONE"] != float64(2) || bc["ORPHAN_WORKLOG"] != float64(1) {
		t.Fatalf("by_code: %+v", bc)
	}
	samples, _ := resp["samples"].([]any)
	if len(samples) == 0 || len(samples) > 5 {
		t.Fatalf("samples len=%d", len(samples))
	}
	s0, _ := samples[0].(map[string]any)
	if s0["severity"] != "warning" {
		t.Fatalf("sample severity=%v", s0["severity"])
	}
}

func TestServer_VerifyEndpointStrictPromotesWarnings(t *testing.T) {
	srv, pid := newTestServer(t)
	// Honor strict: when strict=true, warns are promoted to fails.
	srv.RunVerify = func(target string, strict bool) (verify.Report, error) {
		rep := verify.Report{
			Warns: []verify.Issue{{Code: "WEAK_DONE", Message: "weak"}},
		}
		if strict {
			rep.Fails = append(rep.Fails, rep.Warns...)
			rep.Warns = nil
		}
		return rep, nil
	}
	var loose map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/verify", &loose); c != 200 {
		t.Fatalf("status %d", c)
	}
	if loose["fail_count"] != float64(0) || loose["warn_count"] != float64(1) {
		t.Fatalf("default mode wrong: %+v", loose)
	}

	var strict map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/verify?strict=1", &strict); c != 200 {
		t.Fatalf("status %d", c)
	}
	if strict["fail_count"] != float64(1) || strict["warn_count"] != float64(0) {
		t.Fatalf("strict mode wrong: %+v", strict)
	}
	samples, _ := strict["samples"].([]any)
	if len(samples) != 1 {
		t.Fatalf("want 1 sample, got %d", len(samples))
	}
	s0, _ := samples[0].(map[string]any)
	if s0["severity"] != "fail" {
		t.Fatalf("sample severity=%v want fail", s0["severity"])
	}
}

func TestServer_VerifyEndpointMissingProject(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.RunVerify = func(target string, strict bool) (verify.Report, error) {
		return verify.Report{}, nil
	}
	c := getJSON(t, srv.Handler(), "/api/projects/does-not-exist/verify", nil)
	if c != http.StatusNotFound {
		t.Fatalf("want 404, got %d", c)
	}
}

func TestServer_VerifyEndpointCachesResults(t *testing.T) {
	srv, pid := newTestServer(t)
	calls := 0
	srv.RunVerify = func(target string, strict bool) (verify.Report, error) {
		calls++
		return verify.Report{}, nil
	}
	// Two requests within TTL should hit the cache; only one underlying run.
	for i := 0; i < 3; i++ {
		var resp map[string]any
		if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/verify", &resp); c != 200 {
			t.Fatalf("status %d", c)
		}
	}
	if calls != 1 {
		t.Fatalf("expected cache hit, calls=%d", calls)
	}
}
