// Package model defines environment records and their value validation without service dependencies.
package model

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const DocumentPath = "SteerlineFlow/environment.md"
const GenerationRulesPath = "SteerlineFlow/environment-generation.md"
const GenerationContextPath = "SteerlineFlow/environment-context.json"
const ExecutionRulesPath = "SteerlineFlow/environment-execution.md"
const MaxRounds = 3

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
func Target(e runnercontract.ExecutionInput) string {
	if e.Target.Kind != "orca_environment" {
		return ""
	}
	b, _ := json.Marshal(struct {
		Adapter string
		Target  runnercontract.ExecutionTarget
	}{e.Adapter, e.Target})
	return string(b)
}
func ExecutionDigest(e runnercontract.ExecutionInput) string {
	b, _ := json.Marshal(e)
	b, _ = runnercontract.CanonicalJSON(b)
	return Digest(b)
}

type Manifest struct {
	Version        int      `json:"version"`
	Script         string   `json:"script"`
	Interpreter    string   `json:"interpreter"`
	ValidationBase string   `json:"validation_base"`
	Dependencies   []string `json:"dependencies"`
}

func ParseManifest(document []byte) (Manifest, error) {
	var m Manifest
	document = bytes.ReplaceAll(document, []byte("\r\n"), []byte("\n"))
	start := bytes.Index(document, []byte("```steerline-environment\n"))
	if start < 0 {
		document = bytes.ReplaceAll(document, []byte("\r\n"), []byte("\n"))
		start = bytes.Index(document, []byte("```steerline-environment\n"))
	}
	if start < 0 {
		return m, errors.New("environment.md requires one steerline-environment JSON block")
	}
	b := document[start+len("```steerline-environment\n"):]
	end := bytes.Index(b, []byte("\n```"))
	if end < 0 || bytes.Count(document, []byte("```steerline-environment")) != 1 {
		return m, errors.New("invalid environment metadata block")
	}
	if e := Decode(b[:end], &m); e != nil {
		return m, e
	}
	if m.Version != 1 || (m.Script != "scripts/verify-environment.ps1" && m.Script != "scripts/verify-environment.sh") || (m.Script == "scripts/verify-environment.ps1" && m.Interpreter != "pwsh") || (m.Script == "scripts/verify-environment.sh" && m.Interpreter != "bash") || strings.TrimSpace(m.ValidationBase) == "" {
		return m, errors.New("invalid script, interpreter or pre-existing validation base")
	}
	seen := map[string]bool{}
	for _, p := range m.Dependencies {
		if !fs.ValidPath(p) || strings.ContainsAny(p, "\\:\x00") || seen[p] {
			return m, errors.New("dependencies require unique repository-relative files")
		}
		seen[p] = true
	}
	sort.Strings(m.Dependencies)
	return m, nil
}

type Prepared struct {
	Commit       string                  `json:"commit"`
	Inputs       string                  `json:"inputs"`
	ScriptDigest string                  `json:"script_digest"`
	Manifest     Manifest                `json:"manifest"`
	L2           []string                `json:"l2"`
	Input        runnercontract.RunInput `json:"input"`
}
type Start struct {
	Profile        string `json:"profile_id"`
	TimeoutSeconds int    `json:"round_timeout_seconds"`
}
type Request struct {
	ID       string          `json:"request_id"`
	Original json.RawMessage `json:"original"`
	Action   string          `json:"action"`
}
type Round struct {
	Number        int                          `json:"number"`
	Request       runnercontract.Request       `json:"request"`
	Submission    string                       `json:"submission"`
	RunID         string                       `json:"run_id"`
	InputDigest   string                       `json:"input_digest"`
	Deadline      time.Time                    `json:"deadline"`
	Run           *runnercontract.Run          `json:"run,omitempty"`
	Progress      *runnercontract.ProgressFile `json:"progress,omitempty"`
	Result        *runnercontract.ResultFile   `json:"result,omitempty"`
	Report        *Report                      `json:"report,omitempty"`
	Diagnostic    string                       `json:"diagnostic,omitempty"`
	Stop          *runnercontract.Request      `json:"stop,omitempty"`
	StopState     string                       `json:"stop_state,omitempty"`
	RejectedStops []runnercontract.Request     `json:"rejected_stops,omitempty"`
	Settled       bool                         `json:"settled"`
	TimedOut      bool                         `json:"timed_out"`
}
type Generated struct {
	Document string   `json:"document"`
	Script   string   `json:"script"`
	Gaps     []string `json:"gaps"`
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
	Generation     *Generated                    `json:"generated,omitempty"`
}

func (r Record) Settled() bool {
	for _, a := range r.Rounds {
		if !a.Settled {
			return false
		}
	}
	return r.State == "ready" || r.State == "failed" || r.State == "cancelled" || r.State == "generated"
}
func (r Record) Status() string {
	if r.Kind == "adaptation" && r.State == "ready" && r.Settled() {
		return "ready"
	}
	return "not_ready"
}

type Item struct {
	ID      string `json:"test_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type Check struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type PR struct {
	URL              string `json:"url"`
	Base             string `json:"base"`
	Head             string `json:"head"`
	ChecksPassed     bool   `json:"checks_passed"`
	ReviewsPassed    bool   `json:"reviews_passed"`
	Merged           bool   `json:"merged"`
	MergeCommit      string `json:"merge_commit"`
	CleanupCompleted bool   `json:"cleanup_completed"`
	OutcomeUnknown   bool   `json:"outcome_unknown"`
}
type Report struct {
	Version      int     `json:"version"`
	ActivityID   string  `json:"activity_id"`
	Round        int     `json:"round"`
	Commit       string  `json:"source_commit"`
	ScriptDigest string  `json:"script_digest"`
	Completed    bool    `json:"completed"`
	Checks       []Check `json:"checks"`
	L2           []Item  `json:"l2"`
	PR           PR      `json:"pr"`
	Message      string  `json:"message"`
}

// RoundFile carries the unchanged report and optional read-only input facts.
// Scripts may preserve those facts; they never become report/history authority.
type RoundFile struct {
	Report
	Previous json.RawMessage `json:"previous,omitempty"`
}

func ParseReport(raw []byte, r Record, a Round) (*Report, error) {
	var file RoundFile
	if e := Decode(raw, &file); e != nil {
		return nil, e
	}
	if file.Previous != nil {
		var input runnercontract.RunInput
		if json.Unmarshal(a.Request.Input, &input) != nil || input.ResultFile == nil {
			return nil, errors.New("previous facts have no frozen input file")
		}
		initial, e := base64.StdEncoding.Strict().DecodeString(input.ResultFile.InitialContentBase64)
		if e != nil {
			return nil, e
		}
		var frozen RoundFile
		if e := Decode(initial, &frozen); e != nil {
			return nil, e
		}
		got, e := runnercontract.CanonicalJSON(file.Previous)
		if e != nil {
			return nil, e
		}
		want, e := runnercontract.CanonicalJSON(frozen.Previous)
		if e != nil || !bytes.Equal(got, want) {
			return nil, errors.New("previous facts differ from the frozen initial file")
		}
	}
	p := file.Report
	if p.Version != 1 || p.ActivityID != r.ID || p.Round != a.Number || p.Commit != r.Prepared.Commit || p.ScriptDigest != r.Prepared.ScriptDigest {
		return nil, errors.New("report differs from original round/materials")
	}
	if !p.Completed {
		return &p, nil
	}
	if len(p.Checks) == 0 {
		return nil, errors.New("report must include environment requirement checks")
	}
	seen := map[string]bool{}
	for _, check := range p.Checks {
		if strings.TrimSpace(check.ID) == "" || seen[check.ID] || (check.Status != "passed" && check.Status != "failed" && check.Status != "incomplete") || strings.TrimSpace(check.Message) == "" {
			return nil, errors.New("invalid, duplicate or unexplained environment requirement check")
		}
		seen[check.ID] = true
	}
	if len(p.L2) != len(r.Prepared.L2) || len(p.L2) == 0 {
		return nil, errors.New("report must include the entire L2 selection")
	}
	want := map[string]bool{}
	for _, id := range r.Prepared.L2 {
		want[id] = true
	}
	for _, i := range p.L2 {
		if !want[i.ID] || (i.Status != "passed" && i.Status != "failed" && i.Status != "incomplete") {
			return nil, errors.New("unknown, duplicate or invalid L2 result")
		}
		if i.Status != "passed" && strings.TrimSpace(i.Message) == "" {
			return nil, errors.New("failed/incomplete L2 requires a reason")
		}
		delete(want, i.ID)
	}
	return &p, nil
}
func (p Report) Passed(base, head string) bool {
	if !p.Completed || len(p.Checks) == 0 || len(p.L2) == 0 {
		return false
	}
	tty := false
	for _, check := range p.Checks {
		if check.Status != "passed" {
			return false
		}
		if check.ID == "terminal.tty" {
			tty = true
		}
	}
	if !tty {
		return false
	}
	for _, i := range p.L2 {
		if i.Status != "passed" {
			return false
		}
	}
	u, e := url.ParseRequestURI(p.PR.URL)
	return e == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http") && !p.PR.OutcomeUnknown && p.PR.Base == base && head != "" && p.PR.Head == head && p.PR.ChecksPassed && p.PR.ReviewsPassed && p.PR.Merged && regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`).MatchString(p.PR.MergeCommit) && p.PR.CleanupCompleted
}

func DigestJSON(v any) string { b, _ := json.Marshal(v); return Digest(b) }
