package execution

type DecisionOption struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Detail   string `json:"detail,omitempty"`
	Receiver string `json:"receiver,omitempty"`
	Action   string `json:"action,omitempty"`
}
type Question struct {
	ExecutionID   string           `json:"execution_id,omitempty"`
	AttemptID     string           `json:"attempt_id,omitempty"`
	Generation    string           `json:"generation,omitempty"`
	TurnID        string           `json:"turn_id,omitempty"`
	ID            string           `json:"id"`
	Header        string           `json:"header"`
	Question      string           `json:"question"`
	Options       []DecisionOption `json:"options"`
	AllowFreeText bool             `json:"allow_free_text"`
}

// QuestionRoute is a bounded, versioned implementation-owned answer route.
// The center verifies its identity and digest but does not inspect Payload.
type QuestionRoute struct {
	Version           int    `json:"version"`
	ImplementationKey string `json:"implementation_key"`
	CodecVersion      int    `json:"codec_version"`
	Purpose           string `json:"purpose"`
	ProjectID         string `json:"project_id"`
	WorkflowRunID     string `json:"workflow_run_id"`
	ExecutionID       string `json:"execution_id"`
	AttemptID         string `json:"attempt_id"`
	Generation        string `json:"generation"`
	TurnID            string `json:"turn_id"`
	QuestionID        string `json:"question_id"`
	Digest            string `json:"digest"`
	Payload           []byte `json:"payload"`
}

const (
	QuestionRouteVersion       = 1
	QuestionRouteMaxBytes      = 64 << 10
	QuestionRouteAnswerPurpose = "answer"
)

type QuestionRouteIdentity struct {
	ProjectID     string
	WorkflowRunID string
	ExecutionID   string
	AttemptID     string
	Generation    string
	TurnID        string
	QuestionID    string
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
