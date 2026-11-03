// Package model defines the project detection interface and immutable execution records.
package model

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxRounds = 1
const ReportVersion = 3

var ErrNotFound = errors.New("environment activity not found")
var IDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func ID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func Digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func Decode(b []byte, out any) error {
	if len(b) == 0 || len(b) > 1<<20 || !utf8.Valid(b) {
		return errors.New("environment document exceeds bounds")
	}
	canonical, e := runnercontract.CanonicalJSON(b)
	if e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(canonical))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func ExecutionDigest(e runnercontract.ExecutionInput) string {
	b, _ := json.Marshal(e)
	b, _ = runnercontract.CanonicalJSON(b)
	return Digest(b)
}

type Manifest struct {
	Version      int      `json:"version"`
	Script       string   `json:"script"`
	Interpreter  string   `json:"interpreter"`
	Dependencies []string `json:"dependencies"`
}

func ParseManifest(document []byte) (Manifest, error) {
	var m Manifest
	if len(document) == 0 || len(document) > 1<<20 || !utf8.Valid(document) {
		return m, errors.New("environment.md exceeds UTF-8 bounds")
	}
	document = bytes.ReplaceAll(document, []byte("\r\n"), []byte("\n"))
	marker := []byte("```steerline-environment\n")
	start := bytes.Index(document, marker)
	if start < 0 || bytes.Count(document, []byte("```steerline-environment")) != 1 {
		return m, errors.New("environment.md requires one steerline-environment JSON block")
	}
	b := document[start+len(marker):]
	end := bytes.Index(b, []byte("\n```"))
	if end < 0 {
		return m, errors.New("invalid environment metadata block")
	}
	if e := Decode(b[:end], &m); e != nil {
		return m, e
	}
	if m.Version != 5 || m.Dependencies == nil || (m.Script != ScriptPath("ps1") && m.Script != ScriptPath("sh")) || (strings.HasSuffix(m.Script, ".ps1") && m.Interpreter != "powershell.exe") || (strings.HasSuffix(m.Script, ".sh") && m.Interpreter != "bash") {
		return m, errors.New("invalid environment invocation interface")
	}
	seen := map[string]bool{}
	for _, p := range m.Dependencies {
		if !fs.ValidPath(p) || p == "." || strings.ContainsAny(p, "\\:\x00*?[]") || seen[p] || p == m.Script || p == DocumentPath() || p == "SteerlineProjectGuideDoc/environment-adaptation.md" || p == "SteerlineFlow/environment-generation.md" || p == "SteerlineFlow/environment-execution.md" || strings.HasPrefix(p, "SteerlineFlow/environments/") {
			return m, errors.New("dependencies require unique finite project files")
		}
		seen[p] = true
	}
	sort.Strings(m.Dependencies)
	return m, nil
}

type CallContext struct {
	Adapter string                         `json:"adapter"`
	Target  runnercontract.ExecutionTarget `json:"target"`
}
type Prepared struct {
	Commit       string                  `json:"commit"`
	Inputs       string                  `json:"inputs"`
	ScriptDigest string                  `json:"script_digest"`
	Manifest     Manifest                `json:"manifest"`
	Input        runnercontract.RunInput `json:"input"`
}
type Start struct {
	EnvironmentKey          string `json:"environment_key"`
	Profile                 string `json:"profile_id"`
	ExpectedCurrentRevision uint64 `json:"expected_current_revision"`
	TimeoutSeconds          int    `json:"round_timeout_seconds"`
}
type Request struct {
	ID       string          `json:"request_id"`
	Original json.RawMessage `json:"original"`
	Action   string          `json:"action"`
}
type Round struct {
	Number        int                        `json:"number"`
	Request       runnercontract.Request     `json:"request"`
	Submission    string                     `json:"submission"`
	RunID         string                     `json:"run_id"`
	InputDigest   string                     `json:"input_digest"`
	Deadline      time.Time                  `json:"deadline"`
	Run           *runnercontract.Run        `json:"run,omitempty"`
	Result        *runnercontract.ResultFile `json:"result,omitempty"`
	Report        *Report                    `json:"report,omitempty"`
	Diagnostic    string                     `json:"diagnostic,omitempty"`
	Stop          *runnercontract.Request    `json:"stop,omitempty"`
	StopState     string                     `json:"stop_state,omitempty"`
	RejectedStops []runnercontract.Request   `json:"rejected_stops,omitempty"`
	Settled       bool                       `json:"settled"`
	TimedOut      bool                       `json:"timed_out"`
}
type Record struct {
	ID             string                        `json:"id"`
	Project        string                        `json:"project"`
	Revision       uint64                        `json:"revision"`
	Kind           string                        `json:"kind"`
	Profile        string                        `json:"profile_id"`
	Execution      runnercontract.ExecutionInput `json:"execution"`
	Build          runnercontract.BuildPin       `json:"runner"`
	CreatedAt      time.Time                     `json:"created_at"`
	Requests       []Request                     `json:"requests"`
	Prepared       Prepared                      `json:"prepared"`
	TimeoutSeconds int                           `json:"round_timeout_seconds"`
	Rounds         []Round                       `json:"rounds"`
	State          string                        `json:"state"`
	Diagnostic     string                        `json:"diagnostic,omitempty"`
	Cancelled      bool                          `json:"cancelled"`
}

func (r Record) Settled() bool {
	for _, a := range r.Rounds {
		if !a.Settled {
			return false
		}
	}
	return r.State == "ready" || r.State == "failed" || r.State == "cancelled"
}
func (r Record) Status() string {
	if r.Kind == "adaptation" && r.State == "ready" && r.Settled() {
		return "ready"
	}
	return "not_ready"
}

type Report struct {
	Version        int             `json:"version"`
	Project        string          `json:"project_identity"`
	EnvironmentKey string          `json:"environment_key"`
	ActivityID     string          `json:"activity_id"`
	Round          int             `json:"round"`
	Commit         string          `json:"source_commit"`
	Inputs         string          `json:"inputs_digest"`
	Context        CallContext     `json:"call_context"`
	Completed      bool            `json:"completed"`
	Ready          bool            `json:"ready"`
	Message        string          `json:"message"`
	Details        json.RawMessage `json:"details,omitempty"`
}

func InitialReport(r Record, n int) Report {
	k, _ := Key(r.Execution)
	return Report{Version: ReportVersion, Project: r.Project, EnvironmentKey: k, ActivityID: r.ID, Round: n, Commit: r.Prepared.Commit, Inputs: r.Prepared.Inputs, Context: CallContext{r.Execution.Adapter, r.Execution.Target}}
}
func ParseReport(raw []byte, r Record, a Round) (*Report, error) {
	var p Report
	if len(raw) > runnercontract.MaxProgressBytes {
		return nil, errors.New("environment result exceeds file contract")
	}
	if e := Decode(raw, &p); e != nil {
		return nil, e
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for _, k := range []string{"version", "project_identity", "environment_key", "activity_id", "round", "source_commit", "inputs_digest", "call_context", "completed", "ready", "message"} {
		if v, ok := fields[k]; !ok || bytes.Equal(v, []byte("null")) {
			return nil, errors.New("environment result omits a required field")
		}
	}
	want := InitialReport(r, a.Number)
	if p.Version != want.Version || p.Project != want.Project || p.EnvironmentKey != want.EnvironmentKey || p.ActivityID != want.ActivityID || p.Round != want.Round || p.Commit != want.Commit || p.Inputs != want.Inputs || p.Context != want.Context {
		return nil, errors.New("environment result differs from original invocation")
	}
	if p.Completed && strings.TrimSpace(p.Message) == "" {
		return nil, errors.New("completed detection must explain its outcome")
	}
	if !p.Completed && p.Ready {
		return nil, errors.New("unfinished detection cannot report ready")
	}
	return &p, nil
}
func (p Report) Passed() bool { return p.Completed && p.Ready }
func TrustedFinal(a Round) bool {
	if a.Run == nil || a.Run.Status != "completed" || a.Run.Final == nil {
		return false
	}
	f := a.Run.Final
	return f.Verified && f.NativeOutcome == "succeeded" && f.CommandID != "" && f.SessionID != "" && f.TurnID != ""
}
func DigestJSON(v any) string { b, _ := json.Marshal(v); return Digest(b) }
