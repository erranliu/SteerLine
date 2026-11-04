package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	progresstools "github.com/erranliu/SteerLine/tools"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

// Static references come from pinned Git. The generated selection is frozen
// into the same Runner request as its progress seed.
func (c Config) prepareTestPlan(ctx context.Context, work taskmodel.FixedWork, run *runnerclient.RunInput) (snapshot json.RawMessage, preparationErr error) {
	// These checks only read pinned materials, before candidate reservation or
	// Runner submission. Reject a definite preparation failure with a receipt.
	defer func() {
		if preparationErr != nil {
			preparationErr = &taskmodel.Error{Code: "test_materials_not_ready", Message: preparationErr.Error()}
		}
	}()
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
	entry, e := testmodel.ReadProjectEntry(read)
	if e != nil {
		return nil, e
	}
	if !reflect.DeepEqual(entry, plan.Project) {
		return nil, errors.New("固定项目测试来源与原安排不一致")
	}
	for _, path := range append([]string{plan.PlanPath}, entry.Paths()...) {
		body, err := read(path)
		if err != nil {
			return nil, err
		}
		if len(body) == 0 || len(body) > testmanagement.MaxDocumentBytes || !utf8.Valid(body) || path == plan.PlanPath && taskPayloadDigest(body) != work.Contract.PlanRef.SHA256 {
			return nil, fmt.Errorf("固定测试材料无效或摘要不符：%s", path)
		}
	}
	rawCatalogue, err := read(entry.TestingCatalogue)
	if err != nil {
		return nil, err
	}
	catalogue, err := testmanagement.ParseCatalogue(rawCatalogue)
	if err != nil {
		return nil, err
	}
	arrangement, err := testmodel.FrozenArrangement(plan)
	if err != nil {
		return nil, err
	}
	if err = testmodel.ValidateArrangement(plan, arrangement, catalogue); err != nil {
		return nil, err
	}
	if len(arrangement) > runnercontract.MaxProgressBytes {
		return nil, errors.New("固定测试安排超过 Runner 文件上限")
	}
	if run.ResultFile != nil {
		return nil, errors.New("Runner 文件通道已被当前候选占用")
	}
	if err := c.runtimeCatalog.ValidateRequiredInputFile(run.Execution, run.Workflow); err != nil {
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
	run.ResultFile = &runnercontract.ResultFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(arrangement), RequiredInput: true}
	seen := map[string]bool{run.Documents.Spec.Path: true, run.Documents.Plan.Path: true}
	for _, ref := range run.Documents.References {
		seen[ref.Path] = true
	}
	paths := append(entry.Paths(), testmodel.ProtocolPath, testmodel.WriterPath)
	for _, path := range paths {
		if !seen[path] {
			run.Documents.References = append(run.Documents.References, runnerclient.Document{Path: path})
			seen[path] = true
		}
	}
	return append(json.RawMessage(nil), work.FrozenTestPlan...), nil
}
