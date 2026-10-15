package native

// ProcessFaultTarget is test-owned external process evidence. Runner's public
// service identity is the parent; this fixture owns exactly one local Run.
type ProcessFaultTarget struct {
	ParentPID                                     int
	ParentCreated, Executable, Workspace, Session string
}
