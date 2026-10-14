package workflow

// FactAdmission concerns business visibility of a normalized notification.
// It has no session, process, request or backend knowledge.
func FactAdmission(status, event string) bool {
	switch event {
	case "started":
		return status == "queued" || status == "waiting_input" || status == "waiting_external" || status == "reconciling"
	case "question", "ended":
		return status == "running" || status == "waiting_input"
	case "answered":
		return status == "waiting_input"
	}
	return false
}
func FactStatus(event string, pendingQuestion bool) string {
	switch event {
	case "started", "answered":
		return "running"
	case "question":
		return "waiting_input"
	case "ended":
		if pendingQuestion {
			return "waiting_input"
		}
		return "blocked"
	}
	return ""
}
