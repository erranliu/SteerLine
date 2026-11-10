// Package model defines Roadmap inputs, confirmed local identities and validation.
// Formal Task numbers and DAG writes remain owned by task.
package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

var ErrNotFound = errors.New("roadmap record not found")
var IDPattern = regexp.MustCompile("^[0-9a-f]{32}$")

const Root = "SteerlineFlow/Roadmap/drafts/"
const MaxBytes = 2 << 20

type Source struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Body       string `json:"body"`
	Digest     string `json:"digest"`
	Head       string `json:"head"`
	Modified   bool   `json:"modified"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type Existing struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Status         string   `json:"status,omitempty"`
	Spec           string   `json:"spec"`
	SpecText       string   `json:"spec_text"`
	MaterialDigest string   `json:"material_digest"`
	Predecessors   []string `json:"predecessors"`
}
type Dependencies struct {
	Local  []string `json:"local"`
	Formal []string `json:"formal"`
}
type Node struct {
	ProvidesTests []string     `json:"provides_tests"`
	Key           string       `json:"local_key"`
	KeyOrigin     string       `json:"key_origin"`
	Title         string       `json:"title"`
	Goal          string       `json:"goal"`
	Scope         string       `json:"scope"`
	Constraints   string       `json:"constraints"`
	Acceptance    string       `json:"acceptance"`
	Evidence      string       `json:"evidence"`
	Original      Dependencies `json:"original_dependencies"`
	Proposed      Dependencies `json:"proposed_dependencies"`
	Reason        string       `json:"reason"`
	ExistingID    string       `json:"existing_task_id"`
}
type Analysis struct {
	Coordination string   `json:"coordination"`
	Tasks        []Node   `json:"tasks"`
	Questions    []string `json:"questions"`
	Overlap      string   `json:"overlap"`
}
type Document struct {
	Path string `json:"path"`
	Body string `json:"body"`
}
type Documents struct {
	Key           string     `json:"local_key"`
	Spec          Document   `json:"spec"`
	Plan          Document   `json:"plan"`
	Tests         []string   `json:"tests"`
	ProvidesTests []string   `json:"provides_tests"`
	Acceptance    []Document `json:"acceptance"`
}
type Publication struct {
	RequestID    string `json:"request_id"`
	ReviewDigest string `json:"review_digest"`
	Branch       string `json:"branch"`
	Base         string `json:"base"`
	Commit       string `json:"commit"`
	URL          string `json:"url"`
	Merge        bool   `json:"merge_authorized"`
	MergedCommit string `json:"merged_commit"`
	State        string `json:"state"`
	Diagnostic   string `json:"diagnostic,omitempty"`
}
type Record struct {
	Generations []Generation `json:"generations,omitempty"`
	// GenerationID selects a transient single-execution view, never stored.
	GenerationID        string                    `json:"-"`
	PlanningMaterials   []runnercontract.Document `json:"planning_materials,omitempty"`
	Purpose             string                    `json:"purpose"`
	ParentID            string                    `json:"parent_id,omitempty"`
	RepositoryTree      string                    `json:"repository_tree,omitempty"`
	Materials           []runnercontract.Document `json:"materials,omitempty"`
	Design              string                    `json:"design,omitempty"`
	Investigation       []Investigation           `json:"investigation,omitempty"`
	Questions           []string                  `json:"document_questions,omitempty"`
	ID                  string                    `json:"id"`
	Project             string                    `json:"project"`
	Revision            uint64                    `json:"revision"`
	RequestID           string                    `json:"request_id"`
	Source              Source                    `json:"source"`
	Existing            []Existing                `json:"existing"`
	GraphDigest         string                    `json:"graph_digest"`
	Profile             string                    `json:"profile"`
	CreatedAt           time.Time                 `json:"created_at"`
	State               string                    `json:"state"`
	Diagnostic          string                    `json:"diagnostic,omitempty"`
	Runner              runnercontract.BuildPin   `json:"-"`
	Request             runnercontract.Request    `json:"-"`
	Submission          string                    `json:"-"`
	RunID               string                    `json:"run_id,omitempty"`
	InputDigest         string                    `json:"input_digest,omitempty"`
	Projection          *runnercontract.Run       `json:"-"`
	Result              json.RawMessage           `json:"result,omitempty"`
	Analysis            *Analysis                 `json:"analysis,omitempty"`
	ConfirmedDigest     string                    `json:"confirmed_digest,omitempty"`
	ConfirmationRequest string                    `json:"confirmation_request,omitempty"`
	Cancelled           bool                      `json:"cancelled"`
	StopID              string                    `json:"stop_id,omitempty"`
	StopRequest         *runnercontract.Request   `json:"-"`
	StopState           string                    `json:"stop_state,omitempty"`
	CodeCommit          string                    `json:"code_commit,omitempty"`
	Documents           []Documents               `json:"documents"`
	DocumentsVerified   bool                      `json:"documents_verified"`
	Review              string                    `json:"review_digest,omitempty"`
	AdmissionBatch      json.RawMessage           `json:"admission_batch,omitempty"`
	Publication         *Publication              `json:"publication,omitempty"`
	AdmissionRequest    string                    `json:"admission_request,omitempty"`
	Mapping             map[string]string         `json:"formal_mapping"`
}

// Stored keeps transport bindings private while public records remain readable.
type Stored struct {
	Record
	Generations []StoredGeneration      `json:"generations,omitempty"`
	Runner      runnercontract.BuildPin `json:"runner"`
	Request     runnercontract.Request  `json:"runner_request"`
	Submission  string                  `json:"submission"`
	Projection  *runnercontract.Run     `json:"projection,omitempty"`
	StopRequest *runnercontract.Request `json:"stop_request,omitempty"`
}

func StoreValue(r Record) Stored {
	s := Stored{Record: r, Runner: r.Runner, Request: r.Request, Submission: r.Submission, Projection: r.Projection, StopRequest: r.StopRequest}
	for _, g := range r.Generations {
		s.Generations = append(s.Generations, StoredGeneration{Generation: g, Result: g.Result, Request: g.Request, Projection: g.Projection, StopRequest: g.StopRequest})
	}
	return s
}
func (s Stored) Value() Record {
	r := s.Record
	r.Runner = s.Runner
	r.Request = s.Request
	r.Submission = s.Submission
	r.Projection = s.Projection
	r.StopRequest = s.StopRequest
	r.Generations = nil
	for _, g := range s.Generations {
		v := g.Generation
		v.Result = g.Result
		v.Request, v.Projection, v.StopRequest = g.Request, g.Projection, g.StopRequest
		r.Generations = append(r.Generations, v)
	}
	return r
}
func Digest(b []byte) string                   { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func ValueDigest(v any) string                 { b, _ := json.Marshal(v); return Digest(b) }
func SourceID(project, relative string) string { return Digest([]byte(project + "\n" + relative))[:32] }

// SameMarkdown tolerates checkout line endings only; it never rewrites either
// source or its raw digest. Spaces, blank lines and the final newline still matter.
func SameMarkdown(actual, expected []byte) bool {
	return bytes.Equal(bytes.ReplaceAll(actual, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n")))
}

// SameContent checks the original snapshots before comparing their Markdown.
func (s Source) SameContent(other Source) bool {
	return s.Digest == Digest([]byte(s.Body)) && other.Digest == Digest([]byte(other.Body)) &&
		SameMarkdown([]byte(s.Body), []byte(other.Body))
}

func SourcePath(relative string) bool {
	return strings.HasPrefix(relative, Root) && strings.HasSuffix(strings.ToLower(relative), ".md") && path.Clean(relative) == relative && !strings.ContainsAny(relative, "\\:\x00")
}
func GraphDigest(graph []Existing) string {
	graph = append([]Existing(nil), graph...)
	for i := range graph {
		// Runtime progress does not change the confirmed dependency contract.
		graph[i].Status = ""
		graph[i].Predecessors = append([]string(nil), graph[i].Predecessors...)
		sort.Strings(graph[i].Predecessors)
	}
	sort.Slice(graph, func(i, j int) bool { return graph[i].ID < graph[j].ID })
	return ValueDigest(graph)
}
func DecodeAnalysis(raw []byte, source Source, graph []Existing) (Analysis, error) {
	var a Analysis
	if len(raw) > MaxBytes {
		return a, errors.New("解析结果超过大小上限")
	}
	canonical, e := runnercontract.CanonicalJSON(raw)
	if e != nil {
		return a, e
	}
	raw = canonical
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > MaxBytes {
		return a, errors.New("解析结果超过大小上限")
	}
	if e := d.Decode(&a); e != nil {
		return a, e
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		return a, errors.New("解析结果必须是一个完整对象")
	}
	return ValidateAnalysis(a, source, graph)
}

// ValidateAnalysis also resolves local aliases of formal Tasks before admission.
// Original references remain available for review; only proposed edges normalize.
func ValidateAnalysis(a Analysis, source Source, graph []Existing) (Analysis, error) {
	if strings.TrimSpace(a.Coordination) == "" {
		return a, errors.New("缺少整批协作安排")
	}
	a.Tasks = append([]Node(nil), a.Tasks...)
	if len(a.Tasks) == 0 {
		return a, errors.New("未识别到可独立交付的任务，请在外部补充 Markdown")
	}
	known := map[string]Existing{}
	for _, n := range graph {
		known[n.ID] = n
	}
	keys := map[string]bool{}
	existing := map[string]bool{}
	for _, n := range a.Tasks {
		if strings.TrimSpace(n.Key) == "" || len(n.Key) > 80 || strings.TrimSpace(n.Title) == "" || strings.TrimSpace(n.Goal) == "" || strings.TrimSpace(n.Scope) == "" || strings.TrimSpace(n.Acceptance) == "" {
			return a, errors.New("任务必须有清晰的内部代号、名称、目标、范围与通过条件")
		}
		if n.KeyOrigin != "markdown" && n.KeyOrigin != "suggested" {
			return a, errors.New("内部代号来源无效")
		}
		if keys[n.Key] {
			return a, fmt.Errorf("重复内部代号 %s", n.Key)
		}
		keys[n.Key] = true
		if n.Evidence == "" || !strings.Contains(source.Body, n.Evidence) {
			return a, fmt.Errorf("%s 的原文依据不存在于本次 Markdown", n.Key)
		}
		if n.KeyOrigin == "markdown" && !strings.Contains(source.Body, n.Key) {
			return a, errors.New("原代号不存在于 Markdown")
		}
		if n.ExistingID != "" {
			old, ok := known[n.ExistingID]
			if !ok || old.ID == "" || existing[n.ExistingID] {
				return a, errors.New("已有任务关联无效或重复")
			}
			existing[n.ExistingID] = true
		}
	}
	byKey := map[string]Node{}
	for _, n := range a.Tasks {
		byKey[n.Key] = n
	}
	for i, n := range a.Tasks {
		original, err := resolveDependencies(n, n.Original, byKey, known)
		if err != nil {
			return a, fmt.Errorf("原依赖：%w", err)
		}
		proposed, err := resolveDependencies(n, n.Proposed, byKey, known)
		if err != nil {
			return a, fmt.Errorf("建议依赖：%w", err)
		}
		if !sameDependencies(original, proposed) && strings.TrimSpace(n.Reason) == "" {
			return a, fmt.Errorf("%s 调整依赖必须说明理由", n.Key)
		}
		a.Tasks[i].Proposed = proposed
	}
	// Reused Tasks are read-only associations, never an opportunity to alter DAG edges.
	for _, n := range a.Tasks {
		if n.ExistingID == "" {
			continue
		}
		old := known[n.ExistingID]
		want := append([]string(nil), n.Proposed.Formal...)
		for _, key := range n.Proposed.Local {
			if byKey[key].ExistingID == "" {
				return a, errors.New("不能为已有正式任务增加草稿前置")
			}
			want = append(want, byKey[key].ExistingID)
		}
		sort.Strings(want)
		have := append([]string(nil), old.Predecessors...)
		sort.Strings(have)
		if strings.Join(want, "\n") != strings.Join(have, "\n") {
			return a, errors.New("已有正式任务的依赖必须保持原状")
		}
	}
	if err := validateCombinedGraph(a.Tasks, graph); err != nil {
		return a, err
	}
	return a, nil
}

func resolveDependencies(n Node, deps Dependencies, nodes map[string]Node, known map[string]Existing) (Dependencies, error) {
	out := Dependencies{Local: []string{}, Formal: []string{}}
	seen := map[string]bool{}
	formal := func(id string) error {
		number, err := strconv.ParseInt(id, 10, 64)
		if err != nil || number < 0 || strconv.FormatInt(number, 10) != id {
			return fmt.Errorf("%s 正式任务编号无效：%s", n.Key, id)
		}
		task, ok := known[id]
		if !ok {
			return fmt.Errorf("%s 正式依赖不存在于当前项目 DAG：#%s", n.Key, id)
		}
		if task.Status == "discarded" {
			return fmt.Errorf("%s 正式依赖已丢弃：#%s", n.Key, id)
		}
		if id == n.ExistingID {
			return fmt.Errorf("%s 不能依赖自己：#%s", n.Key, id)
		}
		if !seen["formal:"+id] {
			seen["formal:"+id] = true
			out.Formal = append(out.Formal, id)
		}
		return nil
	}
	for _, key := range deps.Local {
		dep, ok := nodes[key]
		if !ok || key == n.Key {
			return out, fmt.Errorf("%s 内部依赖无效：%s", n.Key, key)
		}
		if dep.ExistingID != "" {
			if err := formal(dep.ExistingID); err != nil {
				return out, err
			}
		} else if !seen["local:"+key] {
			seen["local:"+key] = true
			out.Local = append(out.Local, key)
		}
	}
	for _, id := range deps.Formal {
		if err := formal(id); err != nil {
			return out, err
		}
	}
	return out, nil
}

func sameDependencies(a, b Dependencies) bool {
	key := func(d Dependencies) string {
		local, formal := append([]string{}, d.Local...), append([]string{}, d.Formal...)
		sort.Strings(local)
		sort.Strings(formal)
		return ValueDigest(Dependencies{Local: local, Formal: formal})
	}
	return key(a) == key(b)
}

func validateCombinedGraph(nodes []Node, graph []Existing) error {
	// Prefix namespaces so a draft key such as "8" never aliases formal #8.
	all := []Node{}
	for _, task := range graph {
		n := Node{Key: "formal:" + task.ID}
		for _, id := range task.Predecessors {
			n.Proposed.Local = append(n.Proposed.Local, "formal:"+id)
		}
		all = append(all, n)
	}
	for _, task := range nodes {
		if task.ExistingID != "" {
			continue
		}
		n := Node{Key: "local:" + task.Key}
		for _, key := range task.Proposed.Local {
			n.Proposed.Local = append(n.Proposed.Local, "local:"+key)
		}
		for _, id := range task.Proposed.Formal {
			n.Proposed.Local = append(n.Proposed.Local, "formal:"+id)
		}
		all = append(all, n)
	}
	if _, err := Order(all); err != nil {
		return errors.New("草稿与正式 DAG 的依赖存在环或引用缺失")
	}
	return nil
}
func Order(nodes []Node) ([]Node, error) {
	remaining := append([]Node(nil), nodes...)
	done := map[string]bool{}
	out := []Node{}
	for len(remaining) > 0 {
		at := -1
		for i, n := range remaining {
			ready := true
			for _, dep := range n.Proposed.Local {
				if !done[dep] {
					ready = false
				}
			}
			if ready {
				at = i
				break
			}
		}
		if at < 0 {
			return nil, errors.New("内部依赖存在环或引用缺失")
		}
		n := remaining[at]
		out = append(out, n)
		done[n.Key] = true
		remaining = append(remaining[:at], remaining[at+1:]...)
	}
	return out, nil
}

// A repeated parse may omit completed work, but cannot rebind a published local identity.
func ValidateSourceMapping(a Analysis, mapping map[string]string) error {
	for _, n := range a.Tasks {
		if id := mapping[n.Key]; id != "" && n.ExistingID != id {
			return fmt.Errorf("%s 已关联正式任务 %s，不得重新创建或替换", n.Key, id)
		}
	}
	return nil
}
func (r Record) Settled() bool {
	if r.Purpose == "documents" && r.GenerationID == "" {
		for _, g := range r.Generations {
			if !r.Execution(g).Settled() {
				return false
			}
		}
		return true
	}
	if r.Submission == "pending" {
		return r.Cancelled || r.State == "rejected"
	}
	return r.State == "rejected" || r.Projection != nil && r.Projection.StopStatus == "stopped"
}
func (r Record) ObservationComplete() bool {
	if r.Purpose == "documents" {
		if r.GenerationID == "" {
			for _, g := range r.Generations {
				if !r.Execution(g).ObservationComplete() {
					return false
				}
			}
			return true
		}
		return r.Settled() && (r.Cancelled || len(r.Result) > 0 || r.State == "failed" || r.State == "invalid" || r.State == "rejected")
	}
	return r.Settled() && (r.Cancelled || r.Analysis != nil || r.State == "failed" || r.State == "invalid" || r.State == "rejected")
}
func Confirm(r Record, source Source, graph []Existing, request string) (Record, error) {
	if r.Analysis == nil || r.Cancelled || len(r.Analysis.Questions) > 0 || r.State != "review" {
		return r, errors.New("解析尚未可确认或存在待澄清问题")
	}
	if !source.SameContent(r.Source) || GraphDigest(graph) != r.GraphDigest {
		return r, errors.New("Markdown 或已有任务/依赖材料已变化，请重新解析")
	}
	if _, err := ValidateAnalysis(*r.Analysis, r.Source, graph); err != nil {
		return r, err
	}
	r.ConfirmedDigest = ConfirmationDigest(r)
	r.ConfirmationRequest = request
	r.State = "confirmed"
	return r, nil
}
func ReviewDigest(r Record) string {
	return ValueDigest(struct {
		Confirmed, Code string
		Documents       []Documents
	}{r.ConfirmedDigest, r.CodeCommit, r.Documents})
}
