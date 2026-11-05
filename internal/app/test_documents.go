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
	plan, arrangement, err := c.testArrangement(ctx, work.Contract, work.FrozenTestPlan)
	if err != nil {
		return nil, err
	}
	if run.ResultFile != nil {
		return nil, errors.New("Runner 文件通道已被当前候选占用")
	}
	if err := c.runtimeCatalog.ValidateResultTarget(run.Execution, run.Workflow); err != nil {
		return nil, err
	}
	if err := c.runtimeCatalog.ValidateProgress(); err != nil {
		return nil, err
	}
	progress, err := testmanagement.InitialProgress(plan)
	if err != nil {
		return nil, err
	}
	workflow, err := c.runtimeCatalog.WithTestProgressContext(run.Workflow, work.FrozenTestPlan)
	if err != nil {
		return nil, fmt.Errorf("所选Runner无法绑定系统测试安排：%w", err)
	}
	if err := c.runtimeCatalog.ValidateConfiguration(run.Execution, workflow); err != nil {
		return nil, err
	}
	run.ProgressFile = progress
	run.Workflow = workflow
	run.ResultFile = &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(arrangement)}
	return append(json.RawMessage(nil), work.FrozenTestPlan...), nil
}

func (c Config) testArrangement(ctx context.Context, contract taskmodel.TaskContract, snapshot json.RawMessage) (testmodel.Snapshot, []byte, error) {
	var plan testmodel.Snapshot
	if err := json.Unmarshal(snapshot, &plan); err != nil {
		return plan, nil, err
	}
	if plan.Status != "ready" || plan.PlanPath != contract.PlanRef.Path || plan.Commit != contract.PlanRef.Commit || plan.Digest != testmanagement.Digest(plan.Definitions) {
		return plan, nil, errors.New("固定测试安排与 Plan、来源或定义摘要不一致")
	}
	reader := taskDocuments{repository: c.Repository, git: c.Git, timeout: positiveDuration(c.CommandTimeout, 30*time.Second), processHost: c.ProcessHost}
	read := func(path string) ([]byte, error) {
		return reader.ReadCommittedRegularFile(ctx, taskmodel.FileReference{Path: path, Commit: plan.Commit, SHA256: strings.Repeat("0", 64)})
	}
	entry, e := testmodel.ReadProjectEntry(read)
	if e != nil {
		return plan, nil, e
	}
	if !reflect.DeepEqual(entry, plan.Project) {
		return plan, nil, errors.New("固定项目测试来源与原安排不一致")
	}
	for _, path := range append([]string{plan.PlanPath}, entry.Paths()...) {
		body, err := read(path)
		if err != nil {
			return plan, nil, err
		}
		if len(body) == 0 || len(body) > testmanagement.MaxDocumentBytes || !utf8.Valid(body) || path == plan.PlanPath && taskPayloadDigest(body) != contract.PlanRef.SHA256 {
			return plan, nil, fmt.Errorf("固定测试材料无效或摘要不符：%s", path)
		}
	}
	rawCatalogue, err := read(entry.TestingCatalogue)
	if err != nil {
		return plan, nil, err
	}
	catalogue, err := testmanagement.ParseCatalogue(rawCatalogue)
	if err != nil {
		return plan, nil, err
	}
	arrangement, err := boundedTestArrangement(plan)
	if err != nil {
		return plan, nil, err
	}
	if err = testmodel.ValidateArrangement(plan, arrangement, catalogue); err != nil {
		return plan, nil, err
	}
	rules, err := read(testmodel.ProtocolPath)
	if err != nil {
		return plan, nil, fmt.Errorf("材料准备不足：执行协议 %s 不可读：%w", testmodel.ProtocolPath, err)
	}
	expectedRules, err := prompts.TestingRules()
	if err != nil {
		return plan, nil, err
	}
	if !prompts.SameRules(rules, expectedRules) {
		return plan, nil, errors.New("材料准备不足：固定执行协议与正式规则不同源")
	}
	if err = verifyTestWriter(read); err != nil {
		return plan, nil, err
	}
	return plan, arrangement, nil
}

var errMismatchedTestWriter = errors.New("材料准备不足：固定提交缺少同源 writer，不支持内联兜底")

func verifyTestWriter(read func(string) ([]byte, error)) error {
	writer, writerErr := read(testmodel.WriterPath)
	if writerErr != nil {
		return fmt.Errorf("材料准备不足：无法读取固定提交的 writer：%w", writerErr)
	}
	referencedWriter := bytes.Equal(bytes.ReplaceAll(writer, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll([]byte(progresstools.PowerShellWriter), []byte("\r\n"), []byte("\n")))
	if !referencedWriter {
		return errMismatchedTestWriter
	}
	return nil
}

func boundedTestArrangement(plan testmodel.Snapshot) ([]byte, error) {
	arrangement, err := testmodel.FrozenArrangement(plan)
	if len(arrangement) > runnercontract.MaxProgressBytes {
		return nil, errors.New("固定测试安排超过 Runner 文件上限")
	}
	return arrangement, err
}

func (c Config) testArrangementTargets() error {
	if err := c.runtimeCatalog.ValidateProgress(); err != nil {
		return err
	}
	for _, profile := range c.RunnerExecution.Profiles {
		if profile.Workflow.ID == "development" || profile.Workflow.ID == "development-audited" {
			if c.validateTestArrangementProfile(profile.Execution, profile.Workflow) == nil {
				return nil
			}
		}
	}
	return errors.New("没有支持测试安排文件的开发执行方案")
}

func (c Config) validateTestArrangementProfile(execution runnerclient.ExecutionInput, workflow runnerclient.Workflow) error {
	if err := c.runtimeCatalog.ValidateResultTarget(execution, workflow); err != nil {
		return err
	}
	workflow.ProjectPolicySlots = []string{}
	return c.runtimeCatalog.ValidateConfiguration(execution, workflow)
}
