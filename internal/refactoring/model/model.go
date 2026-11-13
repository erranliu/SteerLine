// Package model contains maintenance records, public bindings and budget rules.
package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"time"

	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

var ErrNotFound = errors.New("维护行动不存在")
var ErrConflict = errors.New("维护版本或项目在途名额冲突，请刷新")
var validID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var revisionID = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
var decimal = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,18})?$`)
var directionID = regexp.MustCompile(`^(custom:)?[a-z0-9][a-z0-9-]{0,63}$`)
var directionRevision = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidDirectionID(id string) bool { return directionID.MatchString(id) }

type Direction struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Prompt   string `json:"prompt"`
	Revision string `json:"revision"`
}

func ID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func Digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

type Budget struct {
	Dollars string `json:"dollars,omitempty"`
	Minutes string `json:"minutes,omitempty"`
	Rounds  int    `json:"rounds,omitempty"`
}
type Start struct {
	Direction         string `json:"direction"`
	DirectionRevision string `json:"direction_revision"`
	Requirements      string `json:"requirements"`
	Profile           string `json:"profile"`
	Strategy          string `json:"strategy"`
	Budget            Budget `json:"budget"`
}

func (s Start) Validate() error {
	if !ValidDirectionID(s.Direction) || !directionRevision.MatchString(s.DirectionRevision) {
		return errors.New("重构方向或预览版本无效，请刷新后重试")
	}
	if s.Profile == "" || (s.Strategy != "cautious" && s.Strategy != "autonomous" && s.Strategy != "aggressive") || len(s.Requirements) > 32<<10 {
		return errors.New("方向、执行配置或策略无效")
	}
	if s.Budget.Rounds < 0 || s.Budget.Rounds > 10000 || (s.Budget.Rounds == 0 && s.Budget.Dollars == "" && s.Budget.Minutes == "") {
		return errors.New("至少设置一项有效正数预算")
	}
	for _, v := range []string{s.Budget.Dollars, s.Budget.Minutes} {
		if v != "" {
			if !decimal.MatchString(v) || len(v) > 32 {
				return errors.New("预算必须是有限正十进制数")
			}
			n, _ := new(big.Rat).SetString(v)
			if n.Sign() <= 0 {
				return errors.New("预算必须大于零")
			}
		}
	}
	return nil
}

type Frozen struct {
	Direction   Direction                     `json:"direction"`
	Publication map[string]string             `json:"publication"`
	Repository  runnercontract.Repository     `json:"repository"`
	Documents   runnercontract.Documents      `json:"documents"`
	Write       runnercontract.ExecutionInput `json:"write"`
	Read        runnercontract.ExecutionInput `json:"read"`
	Runner      runnercontract.BuildPin       `json:"runner"`
	FirstRules  string                        `json:"first_rules"`
	FollowRules string                        `json:"follow_rules"`
	ReportRules string                        `json:"report_rules"`
	Strategy    string                        `json:"strategy_prompt"`
	Pricing     *billing.Policy               `json:"pricing,omitempty"`
}
type Command struct {
	ID, Action, Target string
	Expected           uint64
	Raw                json.RawMessage
	OptionID           string
	Note               string
	QuestionID         string
}
type Cost struct {
	Amount     *string   `json:"amount,omitempty"`
	Complete   bool      `json:"complete"`
	Diagnostic string    `json:"diagnostic,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
	Snapshot   uint64    `json:"snapshot"`
	Untracked  []string  `json:"untracked"`
}
type Question struct {
	ID          string `json:"id"`
	Mode        string `json:"mode"`
	OperationID string `json:"operation_id"`
	QuestionBody
}
type QuestionBody struct {
	Prompt               string           `json:"prompt"`
	Facts                []string         `json:"facts"`
	Attempts             []string         `json:"attempts"`
	Options              []QuestionOption `json:"options"`
	RecommendedOptionID  string           `json:"recommended_option_id"`
	RecommendationReason string           `json:"recommendation_reason"`
}
type QuestionOption struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Impact string `json:"impact"`
}
type Answer struct {
	QuestionID string         `json:"question_id"`
	Option     QuestionOption `json:"option"`
	Note       string         `json:"note,omitempty"`
}
type Result struct {
	ActionID    string `json:"action_id"`
	OperationID string `json:"operation_id"`
	Outcome     string `json:"outcome"`
	Summary     string `json:"summary"`
	Head        string `json:"head"`
	Base        string `json:"base"`
	PR          string `json:"pr"`
	NoChanges   bool   `json:"no_changes"`
	Question    string `json:"question"`
}
type Report struct {
	Body           string    `json:"body"`
	Recommendation string    `json:"recommendation"`
	Head           string    `json:"head"`
	Base           string    `json:"base"`
	PR             string    `json:"pr"`
	Checks         string    `json:"checks,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type ReviewEvidence struct {
	Diff       string `json:"diff"`
	Checks     string `json:"checks"`
	Limitation string `json:"limitation,omitempty"`
}
type StopAttempt struct {
	Request runnercontract.Request `json:"request"`
	State   string                 `json:"state"`
}
type Operation struct {
	Closing           bool                         `json:"closing"`
	PriorStops        []StopAttempt                `json:"prior_stops,omitempty"`
	Prompt            string                       `json:"prompt"`
	Investigation     string                       `json:"investigation,omitempty"`
	DecisionSummary   string                       `json:"decision_summary,omitempty"`
	DecisionKind      string                       `json:"decision_kind,omitempty"`
	PRVersion         string                       `json:"pr_version,omitempty"`
	ReviewChecks      string                       `json:"review_checks,omitempty"`
	AcceptedOperation string                       `json:"accepted_operation,omitempty"`
	Question          *Question                    `json:"question,omitempty"`
	ID                string                       `json:"id"`
	Purpose           string                       `json:"purpose"`
	Round             int                          `json:"round"`
	Request           runnercontract.Request       `json:"request"`
	InputDigest       string                       `json:"input_digest,omitempty"`
	RunID             string                       `json:"run_id,omitempty"`
	State             string                       `json:"state"`
	Run               *runnercontract.Run          `json:"run,omitempty"`
	WorkspaceGit      *runnercontract.WorkspaceGit `json:"workspace_git,omitempty"`
	Stop              *runnercontract.Request      `json:"stop,omitempty"`
	StopState         string                       `json:"stop_state,omitempty"`
	Feedback          string                       `json:"feedback,omitempty"`
	Result            *Result                      `json:"result,omitempty"`
	Cost              Cost                         `json:"cost"`
	CreatedAt         time.Time                    `json:"created_at"`
	CompletedAt       *time.Time                   `json:"completed_at,omitempty"`
}

func (o Operation) Settled() bool {
	return o.State == "complete" || o.State == "rejected" || o.State == "cancelled"
}

// ObserveRun records an already validated observation.
func (o *Operation) ObserveRun(run *runnercontract.Run) {
	o.RunID, o.InputDigest, o.Run = run.RunID, run.InputDigest, run
}

func (o Operation) Content(r Record) runnerbinding.Content {
	w := "execute"
	switch o.Purpose {
	case "first", "select", "audit", "finish", "report":
		w = "generate"
	}
	var rev uint64
	if o.Run != nil {
		rev = o.Run.Revision
	}
	return runnerbinding.Content{StateDir: r.Frozen.Runner.StateDir, Request: o.Request, RunID: o.RunID, InputDigest: o.InputDigest, Revision: rev, Project: r.Project, ObjectKey: "steerline_refactoring", ObjectID: r.ID, WorkflowID: w, WorkflowVersion: 1, RequireDigest: true, Unsubmitted: o.State == "pending" || o.State == "cancelled", Rejected: o.State == "rejected"}
}

type Record struct {
	ID              string                 `json:"id"`
	Project         string                 `json:"project"`
	Revision        uint64                 `json:"revision"`
	Input           Start                  `json:"input"`
	Frozen          Frozen                 `json:"frozen"`
	Phase           string                 `json:"phase"`
	FinishRequested bool                   `json:"finish_requested"`
	StopRequested   bool                   `json:"stop_requested"`
	Round           int                    `json:"round"`
	LatestOwner     string                 `json:"latest_owner,omitempty"`
	Origin          string                 `json:"origin,omitempty"`
	Identity        string                 `json:"workspace_identity,omitempty"`
	Workspace       string                 `json:"workspace,omitempty"`
	PendingPrompt   string                 `json:"pending_prompt,omitempty"`
	WriteKind       string                 `json:"write_kind,omitempty"`
	Question        *Question              `json:"question,omitempty"`
	Answers         []Answer               `json:"answers"`
	Operations      []Operation            `json:"operations"`
	Candidate       *Result                `json:"candidate,omitempty"`
	PR              *runnercontract.PRFact `json:"pr,omitempty"`
	Reports         []Report               `json:"reports"`
	Diagnostic      string                 `json:"diagnostic,omitempty"`
	Summary         string                 `json:"summary,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	ActiveSince     *time.Time             `json:"active_since,omitempty"`
	ElapsedNanos    int64                  `json:"elapsed_nanos"`
	FinishedAt      *time.Time             `json:"finished_at,omitempty"`
}

func (r Record) Active() bool {
	for _, o := range r.Operations {
		if !o.Settled() {
			return true
		}
	}
	return r.Phase != "delivered" && r.Phase != "stopped" && r.Phase != "blocked"
}

func (r Record) CanReview() bool {
	return !r.Active() && r.PR != nil && !r.PR.Merged && r.PR.State == "open" && r.WriteKind != "conflict"
}
func (r Record) Elapsed(now time.Time) time.Duration {
	d := time.Duration(r.ElapsedNanos)
	if r.ActiveSince != nil && now.After(*r.ActiveSince) {
		d += now.Sub(*r.ActiveSince)
	}
	return d
}
func (r *Record) Pause(now time.Time) { r.ElapsedNanos = int64(r.Elapsed(now)); r.ActiveSince = nil }
func (r Record) CostTotal() (*string, bool) {
	sum := new(big.Rat)
	known := false
	complete := true
	for _, o := range r.Operations {
		if o.State == "cancelled" || o.State == "rejected" {
			continue
		}
		if !o.Cost.Complete {
			complete = false
		}
		if o.Cost.Amount != nil {
			n, ok := new(big.Rat).SetString(*o.Cost.Amount)
			if ok {
				sum.Add(sum, n)
				known = true
			}
		}
	}
	if !known {
		return nil, complete && len(r.Operations) == 0
	}
	v := sum.FloatString(18)
	return &v, complete
}
func (r Record) BudgetExhausted(now time.Time) bool {
	b := r.Input.Budget
	if b.Rounds > 0 && r.Round >= b.Rounds {
		return true
	}
	if b.Minutes != "" {
		n, _ := new(big.Rat).SetString(b.Minutes)
		elapsed := new(big.Rat).SetFrac(big.NewInt(int64(r.Elapsed(now))), big.NewInt(int64(time.Minute)))
		if elapsed.Cmp(n) >= 0 {
			return true
		}
	}
	if b.Dollars != "" {
		n, _ := new(big.Rat).SetString(b.Dollars)
		v, _ := r.CostTotal()
		if v != nil {
			amount, _ := new(big.Rat).SetString(*v)
			if amount.Cmp(n) >= 0 {
				return true
			}
		}
	}
	return false
}

func ValidID(s string) bool       { return validID.MatchString(s) }
func ValidRevision(s string) bool { return revisionID.MatchString(s) }
