package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

// Only repository documents are paths. System results belong to the immutable
// request's prompt context; Runner never has to create a pretend reference file.
func (c Config) prepareTestPlan(_ context.Context, work taskmodel.FixedWork, run *runnerclient.RunInput) (json.RawMessage, error) {
	if len(work.FrozenTestPlan) == 0 {
		raw, _ := json.Marshal(testmanagement.Snapshot{Status: "missing", PlanPath: work.Contract.PlanRef.Path, Commit: work.Contract.PlanRef.Commit, Diagnostic: "候选尚未绑定系统测试安排", Definitions: []testmanagement.Definition{}})
		return raw, nil
	}
	if c.runtimeCatalog == nil {
		return nil, errors.New("所选Runner未提供执行提示正文，无法绑定系统测试安排")
	}
	workflow, e := c.runtimeCatalog.WithPromptContext(run.Workflow, testmodel.TestingContext(work.FrozenTestPlan))
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
	for _, path := range []string{testmanagement.CataloguePath, testmanagement.RulesPath} {
		if !seen[path] {
			run.Documents.References = append(run.Documents.References, runnerclient.Document{Path: path})
			seen[path] = true
		}
	}
	return append(json.RawMessage(nil), work.FrozenTestPlan...), nil
}
