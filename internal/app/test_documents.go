package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	progresstools "github.com/erranliu/SteerLine/tools"
	"strings"
	"time"
	"unicode/utf8"
)

// Every model-visible reference must already be a verified regular Git file.
// The immutable database snapshot remains the original arrangement authority.
func (c Config) prepareTestPlan(ctx context.Context, work taskmodel.FixedWork, run *runnerclient.RunInput) (json.RawMessage, error) {
	run.ProgressFile = nil
	if len(work.FrozenTestPlan) == 0 {
		raw, _ := json.Marshal(testmanagement.Snapshot{Status: "missing", PlanPath: work.Contract.PlanRef.Path, Commit: work.Contract.PlanRef.Commit, Diagnostic: "候选尚未绑定系统测试安排", Definitions: []testmanagement.Definition{}})
		return raw, nil
	}
	if c.runtimeCatalog == nil {
		return nil, errors.New("所选Runner未提供执行提示正文，无法绑定系统测试安排")
	}
	var plan testmanagement.Snapshot
	if err := json.Unmarshal(work.FrozenTestPlan, &plan); err != nil {
		return nil, err
	}
	if plan.Status != "ready" || plan.PlanPath != work.Contract.PlanRef.Path || plan.Commit != work.Contract.PlanRef.Commit || plan.Digest != testmanagement.Digest(plan.Definitions) {
		return nil, errors.New("固定测试安排与 Plan、来源或定义摘要不一致")
	}
	reader := taskDocuments{repository: c.Repository, git: c.Git, timeout: positiveDuration(c.CommandTimeout, 30*time.Second), processHost: c.ProcessHost}
	read := func(path string) ([]byte, error) {
		return reader.ReadCommittedRegularFile(ctx, taskmodel.FileReference{Path: path, Commit: plan.Commit, SHA256: strings.Repeat("0", 64)})
	}
	for _, path := range []string{plan.PlanPath, testmodel.RulesPath} {
		body, err := read(path)
		if err != nil {
			return nil, err
		}
		if len(body) == 0 || len(body) > testmanagement.MaxDocumentBytes || !utf8.Valid(body) || path == plan.PlanPath && taskPayloadDigest(body) != work.Contract.PlanRef.SHA256 {
			return nil, fmt.Errorf("固定测试材料无效或摘要不符：%s", path)
		}
	}
	rawCatalogue, err := read(testmodel.CataloguePath)
	if err != nil {
		return nil, err
	}
	catalogue, err := testmanagement.ParseCatalogue(rawCatalogue)
	if err != nil {
		return nil, err
	}
	index := map[string]testmanagement.Definition{}
	for _, d := range catalogue.Tests {
		index[d.ID] = d
	}
	for _, d := range plan.Definitions {
		if actual, ok := index[d.ID]; !ok || actual != d {
			return nil, fmt.Errorf("固定测试定义与原安排不一致：%s", d.ID)
		}
	}
	arrangementPath, err := testmanagement.ArrangementPath(plan.PlanPath)
	if err != nil {
		return nil, err
	}
	arrangement, err := read(arrangementPath)
	if err != nil {
		return nil, fmt.Errorf("材料准备不足：固定测试安排 %s 不可读：%w", arrangementPath, err)
	}
	if err = testmodel.ValidateArrangement(plan, arrangement, catalogue); err != nil {
		return nil, err
	}
	rules, err := read(testmodel.ProtocolPath)
	if err != nil {
		return nil, fmt.Errorf("材料准备不足：执行协议 %s 不可读：%w", testmodel.ProtocolPath, err)
	}
	expectedRules, err := prompts.TestingRules()
	if err != nil {
		return nil, err
	}
	if !prompts.SameRules(rules, expectedRules) {
		return nil, errors.New("材料准备不足：固定执行协议与正式规则不同源")
	}
	writer, writerErr := read(testmodel.WriterPath)
	referencedWriter := writerErr == nil && bytes.Equal(bytes.ReplaceAll(writer, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll([]byte(progresstools.PowerShellWriter), []byte("\r\n"), []byte("\n")))
	if !referencedWriter {
		return nil, errors.New("材料准备不足：固定提交缺少同源 writer，不支持内联兜底")
	}
	if err := c.runtimeCatalog.ValidateProgress(); err != nil {
		return nil, err
	}
	progress, err := testmanagement.InitialProgress(plan)
	if err != nil {
		return nil, err
	}
	run.ProgressFile = progress
	workflow, e := c.runtimeCatalog.WithTestProgressContext(run.Workflow, work.FrozenTestPlan, referencedWriter)
	if e != nil {
		return nil, fmt.Errorf("所选Runner无法绑定系统测试安排：%w", e)
	}
	if e = c.runtimeCatalog.ValidateConfiguration(run.Execution, workflow); e != nil {
		return nil, e
	}
	run.Workflow = workflow
	seen := map[string]bool{run.Documents.Spec.Path: true, run.Documents.Plan.Path: true}
	for _, ref := range run.Documents.References {
		seen[ref.Path] = true
	}
	paths := []string{testmanagement.CataloguePath, testmanagement.RulesPath, arrangementPath, testmodel.ProtocolPath}
	if referencedWriter {
		paths = append(paths, testmodel.WriterPath)
	}
	for _, path := range paths {
		if !seen[path] {
			run.Documents.References = append(run.Documents.References, runnerclient.Document{Path: path})
			seen[path] = true
		}
	}
	return append(json.RawMessage(nil), work.FrozenTestPlan...), nil
}
