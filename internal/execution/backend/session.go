package backend

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

const (
	QuestionRouteVersion       = ImplementationDataVersion
	QuestionRouteMaxBytes      = ImplementationDataMaxBytes
	QuestionRouteAnswerPurpose = ImplementationQuestionPurpose
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
