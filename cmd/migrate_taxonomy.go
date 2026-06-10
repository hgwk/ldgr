package cmd

func mapV1State(status string) string {
	switch status {
	case "open":
		return "ready"
	case "in_progress":
		return "doing"
	case "blocked":
		return "blocked"
	case "audit_ready":
		return "review"
	case "changes_requested":
		return "rework"
	case "done":
		return "done"
	case "cancelled":
		return "dropped"
	default:
		return "backlog"
	}
}

func mapV1Area(category string) string {
	switch category {
	case "doc":
		return "docs"
	case "test":
		return "test"
	case "ops", "chore":
		return "ops"
	case "bug":
		return "backend"
	case "feature", "refactor":
		return "backend"
	default:
		return ""
	}
}

func mapV1EventRole(role string) string {
	switch role {
	case "impl":
		return "implementer"
	case "audit":
		return "auditor"
	case "review":
		return "reviewer"
	case "design":
		return "planner"
	case "ops":
		return "operator"
	default:
		return ""
	}
}
