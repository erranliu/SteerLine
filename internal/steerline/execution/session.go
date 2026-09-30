package execution

type DecisionOption struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Detail   string `json:"detail,omitempty"`
	Receiver string `json:"receiver,omitempty"`
	Action   string `json:"action,omitempty"`
}
type Question struct {
	ID            string           `json:"id"`
	Header        string           `json:"header"`
	Question      string           `json:"question"`
	Options       []DecisionOption `json:"options"`
	AllowFreeText bool             `json:"allow_free_text"`
}

// SessionContext freezes center identity separately from verified native session
// and turn IDs. It contains values only, with no business mutation authority.
type SessionContext struct {
	ProjectID        string
	TaskID           string
	WorkflowRunID    string
	ExecutionID      string
	AttemptID        string
	Agent            AgentSpec
	Target           ExecutionTarget
	SessionID        string
	Generation       string
	CurrentTurnID    string
	PendingTurnID    string
	CurrentCommandID string
	PendingCommandID string
}

// TurnInput identifies a fixed input operation; accepting it does not prove
// that its native turn has started. The native session route is bound separately.
type TurnInput struct {
	CommandID string
	Prompt    string
}

// Answer is the structured answer content for the original verified question.
// Its native route stays with the corresponding platform binding.
type Answer struct {
	Answers map[string][]string `json:"answers"`
}
