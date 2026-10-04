package steerline

func validServiceCommandKind(kind string) bool {
	switch kind {
	case "retry_run", "mark_stopped", "resolve_command":
		return true
	default:
		return false
	}
}
