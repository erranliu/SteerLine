package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type ResultCapability struct {
	Supported struct {
		Adapters    []string `json:"adapters"`
		TargetKinds []string `json:"target_kinds"`
		Workflows   []string `json:"workflows"`
	} `json:"supported"`
	ProgressCapability
	Collection     string `json:"collection"`
	QueryReadsFile *bool  `json:"query_reads_file"`
}

func (c *Catalog) ValidateResult() error {
	if c == nil || c.Capabilities.ResultFile == nil {
		return errors.New("Runner result-file capability unavailable")
	}
	p := c.Capabilities.ResultFile
	if p.Contract != runnercontract.ResultContract || p.Command != "result-file --run-id R --json" || p.SubmitProperty != "result_file" || p.EnvironmentVariable != "STEERLINE_RESULT_FILE" || p.MaxBytes != runnercontract.MaxProgressBytes || p.Encoding != "base64" || p.Digest != "sha256" || p.Collection != "end_only" || p.QueryReadsFile == nil || *p.QueryReadsFile {
		return errors.New("Runner result-file capability is incomplete")
	}
	var doc struct {
		ID         string                     `json:"$id"`
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(c.Capabilities.ResultFileSchema, &doc) != nil || doc.ID != runnercontract.ResultContract {
		return errors.New("Runner result-file schema differs")
	}
	for _, name := range []string{"contract", "run_id", "version", "encoding", "size_bytes", "collection_status"} {
		found := false
		for _, v := range doc.Required {
			found = found || v == name
		}
		if !found || len(doc.Properties[name]) == 0 {
			return errors.New("Runner result-file schema is incomplete")
		}
	}
	var version struct {
		Minimum *int `json:"minimum"`
		Maximum *int `json:"maximum"`
	}
	var status struct {
		Enum []string `json:"enum"`
	}
	if json.Unmarshal(doc.Properties["version"], &version) != nil || version.Minimum == nil || *version.Minimum != 0 || version.Maximum == nil || *version.Maximum != 1 || json.Unmarshal(doc.Properties["collection_status"], &status) != nil || len(status.Enum) != 3 {
		return errors.New("Runner result-file final-only schema differs")
	}
	seen := map[string]bool{}
	for _, v := range status.Enum {
		if seen[v] || v != "disabled" && v != "pending" && v != "closed" {
			return errors.New("Runner result-file final-only statuses differ")
		}
		seen[v] = true
	}
	_, err := compilePublicSchema(c.Capabilities.ResultFileSchema, "result-file")
	return err
}

func ValidateResultFile(p runnercontract.ResultFile, run string) ([]byte, error) {
	if p.Contract != runnercontract.ResultContract {
		return nil, errors.New("Runner result channel contract differs")
	}
	if p.Version > 1 || p.CollectionStatus != "disabled" && p.CollectionStatus != "pending" && p.CollectionStatus != "closed" || p.Version == 1 && p.CollectionStatus != "closed" || p.CollectionStatus == "closed" && p.Version == 0 && p.Diagnostic == "" {
		return nil, errors.New("Runner result final snapshot state is invalid")
	}
	p.Contract = runnercontract.ProgressContract
	return ValidateProgressFile(p, run)
}

func (c Client) ResultFile(ctx context.Context, run string) (runnercontract.ResultFile, error) {
	var out runnercontract.ResultFile
	r, err := c.Call(ctx, "result-file", []string{"--run-id", run}, nil)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(r.Data, &out); err != nil {
		return out, err
	}
	_, err = ValidateResultFile(out, run)
	return out, err
}

// ValidateResultTarget checks support for the existing file channel.
func (c *Catalog) ValidateResultTarget(e runnercontract.ExecutionInput, w runnercontract.Workflow) error {
	if err := c.ValidateResult(); err != nil {
		return err
	}
	support := c.Capabilities.ResultFile.Supported
	if !slices.Contains(support.Workflows, fmt.Sprintf("%s/%d", w.ID, w.Version)) ||
		!slices.Contains(support.Adapters, e.Adapter) || !slices.Contains(support.TargetKinds, e.Target.Kind) {
		return errors.New("Runner未公布所选执行目标的结果文件能力")
	}
	return nil
}

// ValidateEnvironmentExecution consumes the Runner's generic one-shot contract.
func (c *Catalog) ValidateEnvironmentExecution(e runnercontract.ExecutionInput) error {
	if c == nil || c.Capabilities.OfflineInspect == nil {
		return errors.New("Runner未公布启动前只读查询能力")
	}
	q := c.Capabilities.OfflineInspect
	if q.Command != "inspect --offline" || !q.ReadOnly || !q.WriterAbsent || q.StartsWorkers == nil || *q.StartsWorkers {
		return errors.New("Runner启动前只读查询能力不完整")
	}
	if err := c.ValidateResult(); err != nil {
		return err
	}
	contains := func(values []string, want string) bool {
		for _, v := range values {
			if v == want {
				return true
			}
		}
		return false
	}
	support := c.Capabilities.ResultFile.Supported
	if !contains(support.Workflows, "execute/1") || !contains(support.Adapters, e.Adapter) || !contains(support.TargetKinds, e.Target.Kind) {
		return errors.New("Runner未公布execute/1的目标执行及结果封存能力")
	}
	for _, w := range c.Capabilities.Workflows {
		if w.ID == "execute" && w.Version == 1 && contains(w.SupportedAdapters, e.Adapter) {
			s, ok := w.States["execute"]
			closed, terminal := w.States["closed"]
			if ok && s.Kind == "execute" && s.Contract == "final" && terminal && closed.Kind == "terminal" && !w.AuditModelRequired && len(w.States) == 2 {
				return c.ValidateExecution(e)
			}
		}
	}
	return errors.New("Runner未公布无发布的单次执行工作流")
}
