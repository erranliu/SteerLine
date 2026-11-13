package manualverification

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/hostcontract"
)

type Source string

const (
	SourcePR     Source = "pr"
	SourceRemote Source = "remote_head"
)

type Record struct {
	Project    string    `json:"project"`
	Candidate  string    `json:"candidate_id"`
	Task       string    `json:"task_id"`
	PR         string    `json:"pr_url"`
	Head       string    `json:"head"`
	Revision   uint64    `json:"revision"`
	Definition Snapshot  `json:"configuration"`
	Draft      bool      `json:"draft"`
	Merged     bool      `json:"merged"`
	Archived   bool      `json:"archived"`
	Closure    *Closure  `json:"closure,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
	Diagnostic string    `json:"diagnostic,omitempty"`
	Attempts   []Attempt `json:"attempts"`
	Results    []Result  `json:"results"`
}

type Closure struct {
	Source Source    `json:"source"`
	Head   string    `json:"head"`
	Actor  string    `json:"actor"`
	At     time.Time `json:"at"`
}

type Attempt struct {
	ID         string                        `json:"id"`
	Head       string                        `json:"head"`
	Source     Source                        `json:"source"`
	Worktree   string                        `json:"worktree"`
	Program    string                        `json:"program,omitempty"`
	Args       []string                      `json:"args,omitempty"`
	Cwd        string                        `json:"cwd,omitempty"`
	URL        string                        `json:"url,omitempty"`
	Port       int                           `json:"port,omitempty"`
	Status     string                        `json:"status"`
	Started    bool                          `json:"started"`
	Removed    bool                          `json:"removed"`
	ExitCode   *int                          `json:"exit_code,omitempty"`
	Diagnostic string                        `json:"diagnostic,omitempty"`
	Process    *hostcontract.ProcessIdentity `json:"process,omitempty"`
}

func (a Attempt) Active() bool {
	return a.Status == "preparing" || a.Status == "running" || a.Status == "ready" || a.Status == "unknown"
}

type Result struct {
	Head      string    `json:"head"`
	Source    Source    `json:"source"`
	AttemptID string    `json:"attempt_id"`
	CheckID   string    `json:"check_id"`
	Outcome   string    `json:"outcome"`
	Notes     string    `json:"notes"`
	Evidence  []string  `json:"evidence,omitempty"`
	Actor     string    `json:"actor"`
	At        time.Time `json:"at"`
	SHA256    string    `json:"configuration_sha256"`
}

func (r Record) Allows(source Source, head string) bool {
	return !r.Archived && r.Closure == nil && (source == SourcePR && head == r.Head || source == SourceRemote && r.Merged)
}

func (r *Record) Close(closure Closure) error {
	if !r.Allows(closure.Source, closure.Head) {
		return fmt.Errorf("核验待办或选定提交已变化")
	}
	completed := make(map[string]bool)
	for _, result := range r.Results {
		if result.Source == closure.Source && result.Head == closure.Head {
			completed[result.CheckID] = true
		}
	}
	for _, check := range r.Definition.Definition.Checks {
		if !completed[check.ID] {
			return fmt.Errorf("请先记录该来源与提交的所有核验项")
		}
	}
	r.Closure = &closure
	return nil
}

func (r Record) Attempt(id string) (int, error) {
	for i, a := range r.Attempts {
		if a.ID == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("找不到原核验启动记录")
}

func (r Record) ValidateResult(result Result) error {
	if result.Outcome != "passed" && result.Outcome != "failed" {
		return fmt.Errorf("核验结论须为通过或失败")
	}
	if len(result.Notes) > 4096 || strings.ContainsRune(result.Notes, 0) || result.Outcome == "failed" && strings.TrimSpace(result.Notes) == "" {
		return fmt.Errorf("失败须填写实际表现，备注上限 4096 字节")
	}
	if len(result.Evidence) > 8 {
		return fmt.Errorf("最多提供 8 条证据链接")
	}
	for _, value := range result.Evidence {
		parsed, err := url.Parse(value)
		if err != nil || len(value) > 2048 || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
			return fmt.Errorf("证据须为不带凭据的绝对 HTTP(S) 链接")
		}
	}
	for _, check := range r.Definition.Definition.Checks {
		if check.ID == result.CheckID {
			return nil
		}
	}
	return fmt.Errorf("核验项不属于本次配置")
}
