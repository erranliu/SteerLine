package backend

import (
	"reflect"
	"regexp"
	"strings"
	"time"
)

type Observation struct {
	Liveness   string    `json:"liveness"`
	Connection string    `json:"connection"`
	ObservedAt time.Time `json:"observed_at"`
	Source     string    `json:"source"`
	Diagnostic string    `json:"diagnostic,omitempty"`
}

type OutputBatch struct {
	Data          []byte    `json:"data"`
	Source        string    `json:"source"`
	RetainedFrom  string    `json:"retained_from,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
	NextCursor    string    `json:"next_cursor,omitempty"`
	Gap           bool      `json:"gap,omitempty"`
	SourceChanged bool      `json:"source_changed,omitempty"`
}

var prURL = regexp.MustCompile("https?://[^\\s<>\"'`()]+/(?:pulls|pull)/[0-9]+")

func PRClues(text string) []string {
	seen := map[string]bool{}
	var urls []string
	for _, raw := range prURL.FindAllString(text, -1) {
		url := strings.TrimRight(raw, ".,;:!?)）")
		if !seen[url] {
			seen[url] = true
			urls = append(urls, url)
		}
	}
	return urls
}

type ToolReplyFact struct {
	ThreadID, TurnID, CallID, Status string
	Success                          *bool
	Answers                          []map[string][]string
}

func (f ToolReplyFact) MatchesAnswers(expected map[string][]string) bool {
	for _, answers := range f.Answers {
		if reflect.DeepEqual(answers, expected) {
			return true
		}
	}
	return false
}

type TurnEvidence struct {
	NativeOutcome        string `json:"native_outcome,omitempty"` // independent of successful result collection
	FinalOrigin          string `json:"final_origin,omitempty"`
	FinalDiagnostic      string
	FinalNativeSessionID string
	FinalNativeTurnID    string
	FinalSource          string
	FinalItemID          string
	FinalVerified        bool
	SourcePath           string
	TurnID               string
	CommandID            string
	SessionID            string
	Started              bool
	Completed            bool
	Succeeded            bool
	LastMessage          string
	Failure              string
	PRCandidates         []string
}

// EventFact contains verified execution facts. Empty fields convey no evidence.
// Source identity and cursor always refer to the original native observation.
type EventFact struct {
	Kind            EventKind
	Source          string
	SourceEventID   string
	SourceIdentity  string
	SourceCursor    string
	SourceSegment   string
	SourceOffset    int64
	SourceLength    int64
	SessionID       string
	StartedTurnID   string
	InteractionKind string
	Questions       []Question
	QuestionRoute   *ImplementationDataEnvelope
	QuestionText    string
	QuestionContext string
	QuestionOptions []DecisionOption
	AllowFreeText   bool
	ToolReply       *ToolReplyFact
	InputAccepted   bool
	Completed       bool
	Gap             bool
	Matched         bool
	Outcome         string
	PRCandidates    []string
	LastMessage     string
}

type EventKind string

const (
	EventInputAccepted EventKind = "input_accepted"
	EventStarted       EventKind = "started"
	EventQuestion      EventKind = "question"
	EventCompleted     EventKind = "completed"
	EventGap           EventKind = "gap"
	EventStatus        EventKind = "status"
	EventEscalation    EventKind = "escalation"
)

// EventBatch couples generic event facts to the source's consumption token.
// The token is only valid for acknowledgement after the facts are committed.
type EventBatch struct {
	Source        string
	DeliveryID    string
	ExecutionID   string
	AttemptID     string
	ExternalRunID string
	Facts         []EventFact
}
