package httpcontract

import "regexp"

var projectReadRoute = regexp.MustCompile(`^/(identity|tasks|tasks/[^/]+|graph|snapshot|candidates|presentation|environment-checks|environment-checks/(options|[0-9a-f]{32})|runs/[^/]+|runs/[^/]+/(activity|last-message|conversation|final|body|recovery|decision-eligibility|tests)|commands/[0-9a-f]{32})$`)

// AllowedProjectRead is the shared project query allowlist for the application
// handler and browser proxy. Task mutation/receipt routes remain backend-only.
func AllowedProjectRead(route string) bool { return projectReadRoute.MatchString(route) }
