package viewer

import "time"

type Dashboard struct {
	Progress     Progress         `json:"progress"`
	Parents      []ParentProgress `json:"parents"`
	Audit        AuditPipeline    `json:"audit"`
	Health       DeliveryHealth   `json:"health"`
	Recent       []RecentItem     `json:"recent"`
	Priority     PriorityCounts   `json:"priority"`
	Kind         []KindCount      `json:"kind"`
	StaleClaims  StaleClaims      `json:"stale_claims"`
	Lifecycle    LifecycleLatency `json:"lifecycle"`
	ActiveAgents ActiveAgents     `json:"active_agents"`
}

// ActiveAgent is one entry in the active-agents widget — a single actor's
// recent activity across ticket and worklog rows.
type ActiveAgent struct {
	Agent  string `json:"agent"`
	Role   string `json:"role,omitempty"`
	Rows   int    `json:"rows"`
	Latest string `json:"latest"`
}

// ActiveAgents aggregates recent agent activity within a 24h window.
type ActiveAgents struct {
	Agents       []ActiveAgent `json:"agents"`
	UnknownCount int           `json:"unknown_count"`
	WindowHours  int           `json:"window_hours"`
}

// activeAgentsWindow is the lookback window for the active-agents widget.
const activeAgentsWindow = 24 * time.Hour

// activeAgentsMax caps the displayed list to avoid clutter.
const activeAgentsMax = 8

// LifecycleLatency summarizes per-ticket cycle time and audit latency.
// Hours are emitted raw; the frontend rounds for display.
type LifecycleLatency struct {
	CompletedCycleCount     int     `json:"completed_cycle_count"`
	MedianCycleHours        float64 `json:"median_cycle_hours"`
	P90CycleHours           float64 `json:"p90_cycle_hours"`
	PendingAuditCount       int     `json:"pending_audit_count"`
	MedianAuditLatencyHours float64 `json:"median_audit_latency_hours"`
	P90AuditLatencyHours    float64 `json:"p90_audit_latency_hours"`
}

// StaleClaims summarizes expired and near-expiring agent claims on
// non-terminal tickets. Computed from latest ticket rows only.
type StaleClaims struct {
	Expired      int                `json:"expired"`
	NearExpiring int                `json:"near_expiring"`
	Samples      []StaleClaimSample `json:"samples"`
}

// StaleClaimSample is a small projection of a stale-claimed ticket for the
// dashboard tile (used to render up to 3 most overdue ids).
type StaleClaimSample struct {
	TicketID   string `json:"ticket_id"`
	ClaimUntil string `json:"claim_until"`
	ClaimedBy  string `json:"claimed_by"`
}

// nearExpiringClaimWindow is the lookahead window for "near-expiring" claims.
// Intentionally a constant (not configurable) per Task A3.
const nearExpiringClaimWindow = 2 * time.Hour

type Progress struct {
	Done      int `json:"done"`
	Active    int `json:"active"`
	Cancelled int `json:"cancelled"`
	Percent   int `json:"percent"`
}

type ParentProgress struct {
	Parent    string `json:"parent"`
	Done      int    `json:"done"`
	Active    int    `json:"active"`
	Blocked   int    `json:"blocked"`
	Cancelled int    `json:"cancelled"`
	Percent   int    `json:"percent"`
}

type AuditPipeline struct {
	AuditReady       int `json:"audit_ready"`
	ChangesRequested int `json:"changes_requested"`
	WeakDone         int `json:"weak_done"`
}

type DeliveryHealth struct {
	ClosedWithoutWorklog int `json:"closed_without_worklog"`
	OrphanWorklog        int `json:"orphan_worklog"`
	Invalidated          int `json:"invalidated"`
	MissingEvidence      int `json:"missing_evidence"`
}

type RecentItem struct {
	Kind   string `json:"kind"`
	Ticket string `json:"ticket"`
	TS     string `json:"ts"`
	Status string `json:"status,omitempty"`
	Task   string `json:"task,omitempty"`
	Result string `json:"result,omitempty"`
}

// PriorityCounts tallies active priority levels.
type PriorityCounts struct {
	P0 int `json:"p0"`
	P1 int `json:"p1"`
	P2 int `json:"p2"`
	P3 int `json:"p3"`
}

// KindCount represents the count of tickets of a given kind.
type KindCount struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// activeStatuses lists ticket statuses counted as "active" for progress math.
// Cancelled is intentionally absent so it doesn't pull down completion %.
var activeStatuses = map[string]bool{
	"open":              true,
	"in_progress":       true,
	"blocked":           true,
	"audit_ready":       true,
	"changes_requested": true,
}
