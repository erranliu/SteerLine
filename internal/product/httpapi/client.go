package httpapi

func diagnosticOrStatus(diagnostic, status string) string {
	if diagnostic != "" {
		return diagnostic
	}
	return status
}
