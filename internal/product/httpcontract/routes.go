package httpcontract

import "regexp"

var projectReadRoute = regexp.MustCompile(`^/(identity|tests/catalogue|tests/records|tests/records/[0-9a-f]{32}|maintenance|maintenance/log|maintenance/commits/[0-9a-f]{40,64}|maintenance/tasks/[^/]+|tests/generations/[0-9a-f]{32}|tasks|tasks/[^/]+|graph|snapshot|candidates|presentation|environment-checks|environment-checks/(options|[0-9a-f]{32})|runs/[^/]+|runs/[^/]+/(activity|last-message|usage|conversation|final|body|recovery|decision-eligibility|tests|documents)|commands/[0-9a-f]{32})$`)

// AllowedProjectRead is the shared project query allowlist for the application
// handler and browser proxy. Task mutation/receipt routes remain backend-only.
func AllowedProjectRead(route string) bool { return projectReadRoute.MatchString(route) }
