package ports

// ActivityBinding records only the selected execution and attempt ownership
// facts needed before projecting their bounded event page.
type ActivityBinding struct {
	ProjectIdentity        string
	ExecutionID            string
	ExecutionWorkflowRunID string
	AttemptID              string
	AttemptExecutionID     string
}
