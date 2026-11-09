// Package model contains caller-owned material-analysis facts, not an execution workflow.
package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const RulesPath = "SteerlineFlow/main-agent-rules.md"
const ContextPath = "main-agent-inputs/context.json"
const MaxBytes = 1 << 20

var ID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var ErrNotFound = errors.New("material analysis not found")

func Token() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Hash(v any) string      { b, _ := json.Marshal(v); return Digest(b) }

type Document struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
	SHA256 string `json:"sha256"`
	Body   string `json:"body"`
}
type Context struct {
	CandidateID       string                   `json:"candidate_id"`
	CandidateRevision uint64                   `json:"candidate_revision"`
	TaskID            string                   `json:"task_id"`
	TaskVersion       uint64                   `json:"task_version"`
	Title             string                   `json:"title"`
	ContractDigest    string                   `json:"contract_digest"`
	OriginalBase      string                   `json:"original_base"`
	RepositoryCommit  string                   `json:"repository_commit"`
	Original          []Document               `json:"original"`
	Current           []Document               `json:"current"`
	CurrentMissing    map[string]string        `json:"current_missing"`
	SpecPath          string                   `json:"spec_path"`
	PlanPath          string                   `json:"plan_path"`
	RunID             string                   `json:"run_id"`
	RunnerRevision    uint64                   `json:"runner_revision"`
	SourceAttemptID   string                   `json:"source_attempt_id"`
	Question          *runnercontract.Question `json:"question,omitempty"`
	Final             *runnercontract.Final    `json:"final,omitempty"`
	Prior             json.RawMessage          `json:"prior,omitempty"`
	TestPlan          json.RawMessage          `json:"test_plan,omitempty"`
	Inputs            json.RawMessage          `json:"input_history,omitempty"`
}
type Start struct {
	CandidateID       string `json:"candidate_id"`
	CandidateRevision uint64 `json:"candidate_revision"`
	Kind              string `json:"kind"`
	Note              string `json:"note"`
	ParentID          string `json:"parent_id,omitempty"`
}
type Evidence struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
	Quote  string `json:"quote"`
}
type Answer struct {
	Note     string              `json:"note"`
	OptionID string              `json:"option_id"`
	Answers  map[string][]string `json:"answers"`
}
type Change struct {
	Path         string `json:"path"`
	BeforeSHA256 string `json:"before_sha256"`
	Body         string `json:"body"`
	Reason       string `json:"reason"`
}
type Result struct {
	Disposition    string     `json:"disposition"`
	Summary        string     `json:"summary"`
	Recommendation string     `json:"recommendation"`
	Questions      []string   `json:"questions"`
	Evidence       []Evidence `json:"evidence"`
	Answer         *Answer    `json:"answer"`
	Changes        []Change   `json:"changes"`
}
type Publication struct {
	RequestID  string `json:"request_id"`
	Review     string `json:"review"`
	Base       string `json:"base"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	URL        string `json:"url"`
	State      string `json:"state"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type Record struct {
	Project      string                  `json:"project"`
	ID           string                  `json:"id"`
	Revision     uint64                  `json:"revision"`
	CreatedAt    time.Time               `json:"created_at"`
	Input        Start                   `json:"input"`
	Context      Context                 `json:"context"`
	Runner       runnercontract.BuildPin `json:"runner"`
	Request      runnercontract.Request  `json:"request"`
	State        string                  `json:"state"`
	Submission   string                  `json:"submission"`
	RunID        string                  `json:"run_id"`
	InputDigest  string                  `json:"input_digest"`
	Projection   *runnercontract.Run     `json:"projection,omitempty"`
	Result       *Result                 `json:"result,omitempty"`
	Diagnostic   string                  `json:"diagnostic,omitempty"`
	Cancelled    bool                    `json:"cancelled"`
	StopRequest  *runnercontract.Request `json:"stop_request,omitempty"`
	StopState    string                  `json:"stop_state,omitempty"`
	Publication  *Publication            `json:"publication,omitempty"`
	AnswerAction *AnswerAction           `json:"answer_action,omitempty"`
}

type AnswerAction struct {
	CallerID  string `json:"caller_id"`
	RequestID string `json:"request_id"`
	Review    string `json:"review"`
}

type Summary struct {
	ID         string `json:"id"`
	Revision   uint64 `json:"revision"`
	CreatedAt  string `json:"created_at"`
	State      string `json:"state"`
	Kind       string `json:"kind"`
	Note       string `json:"note"`
	ParentID   string `json:"parent_id"`
	Diagnostic string `json:"diagnostic"`
}

func (r Record) Settled() bool {
	return r.Submission == "cancelled" || r.Submission == "rejected" || r.Projection != nil && r.Projection.StopStatus == "stopped"
}
func (r Record) Complete() bool {
	return r.Settled() && (r.State == "review" || r.State == "failed" || r.State == "cancelled" || r.State == "rejected")
}
func Review(r Record) string {
	return Hash(struct {
		Context Context
		Result  *Result
	}{r.Context, r.Result})
}
func Validate(r Record, result Result) error {
	if result.Questions == nil || result.Changes == nil {
		return errors.New("analysis must include complete questions and changes arrays")
	}
	if strings.TrimSpace(result.Summary) == "" || strings.TrimSpace(result.Recommendation) == "" {
		return errors.New("analysis lacks summary or recommendation")
	}
	switch result.Disposition {
	case "clarification", "revision", "needs_user", "out_of_scope":
	default:
		return errors.New("unknown analysis disposition")
	}
	if result.Disposition == "needs_user" && len(result.Questions) == 0 {
		return errors.New("needs_user requires questions")
	}
	if result.Disposition == "out_of_scope" && (result.Answer != nil || len(result.Changes) > 0) {
		return errors.New("out-of-scope analysis cannot execute")
	}
	if len(result.Questions) > 0 && (result.Answer != nil || len(result.Changes) > 0) {
		return errors.New("unresolved decisions cannot have executable outputs")
	}
	if result.Answer != nil && (result.Disposition != "clarification" || r.Context.Question == nil || len(result.Changes) > 0) {
		return errors.New("answer requires original question and unchanged contract")
	}
	if len(result.Changes) > 0 && result.Disposition != "revision" {
		return errors.New("document changes require revision")
	}
	if result.Disposition == "revision" && len(result.Changes) == 0 {
		return errors.New("revision lacks complete documents")
	}
	if len(result.Evidence) == 0 {
		return errors.New("analysis requires original evidence")
	}
	if result.Answer != nil {
		if len(result.Answer.Answers) > 0 && (result.Answer.Note != "" || result.Answer.OptionID != "") {
			return errors.New("structured answer cannot mix note or option")
		}
		if len(result.Answer.Answers) == 0 && strings.TrimSpace(result.Answer.Note) == "" && result.Answer.OptionID == "" {
			return errors.New("analysis answer is empty")
		}
		original := false
		for _, ref := range result.Evidence {
			for _, doc := range r.Context.Original {
				if ref.Path == doc.Path && ref.Commit == doc.Commit && strings.TrimSpace(ref.Quote) != "" && strings.Contains(doc.Body, ref.Quote) {
					original = true
				}
			}
		}
		if !original {
			return errors.New("clarification answer requires original contract evidence")
		}
	}
	for _, ref := range result.Evidence {
		found := false
		for _, d := range append(append([]Document{}, r.Context.Original...), r.Context.Current...) {
			if ref.Path == d.Path && ref.Commit == d.Commit && strings.TrimSpace(ref.Quote) != "" && strings.Contains(d.Body, ref.Quote) {
				found = true
			}
		}
		if !found {
			return errors.New("analysis evidence is absent from provided materials")
		}
	}
	seen := map[string]bool{}
	for _, ch := range result.Changes {
		if seen[ch.Path] || ch.Path != r.Context.SpecPath && ch.Path != r.Context.PlanPath || len(ch.Body) > MaxBytes || !utf8.ValidString(ch.Body) || strings.TrimSpace(ch.Body) == "" || strings.TrimSpace(ch.Reason) == "" {
			return errors.New("revision must only contain complete bound Spec/Plan")
		}
		seen[ch.Path] = true
		found := false
		for _, d := range r.Context.Current {
			if d.Path == ch.Path && d.SHA256 == ch.BeforeSHA256 && ch.Body != d.Body {
				found = true
			}
		}
		if !found {
			return errors.New("revision baseline differs from reviewed document")
		}
	}
	return nil
}
