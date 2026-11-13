package httpcontract

import "regexp"

var projectReadRoute = regexp.MustCompile(`^/(manual-verification|manual-verification/[0-9a-f]{32}/logs/[0-9a-f]{32}|refactoring|refactoring/([0-9a-f]{32}|directions|readiness)|main-agent|main-agent/[0-9a-f]{32}|activities|roadmap-planning|roadmap-planning/[0-9a-f]{32}(/history)?|roadmaps|roadmaps/[0-9a-f]{32}|roadmaps/sources/[0-9a-f]{32}|identity|dispatch|environment|environment/(options|materials|[0-9a-f]{32})|tests/catalogue|tests/records|tests/records/[0-9a-f]{32}|maintenance|maintenance/readiness|maintenance/log|maintenance/commits/[0-9a-f]{40,64}|maintenance/tasks/[^/]+|tasks|tasks/[^/]+|graph|snapshot|candidates|presentation|runs/[^/]+|runs/[^/]+/(activity|last-message|usage|conversation|final|body|recovery|decision-eligibility|tests|documents)|commands/[0-9a-f]{32})$`)

// AllowedProjectRead is the shared project query allowlist for the application
// handler and browser proxy. Task mutation/receipt routes remain backend-only.
var activityReadingRoute = regexp.MustCompile(`^/activities/[A-Za-z0-9_-]{1,1000}(/(interactions|changes|body|conversation))?$`)

func AllowedProjectRead(route string) bool {
	return projectReadRoute.MatchString(route) || activityReadingRoute.MatchString(route)
}
