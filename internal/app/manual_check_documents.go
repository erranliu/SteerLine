package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/manualverification"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (c Config) prepareManualChecks(ctx context.Context, work taskmodel.FixedWork, run *runnerclient.RunInput) (*manualverification.Snapshot, error) {
	ref := work.Contract.PlanRef
	// Projects can still register Plans outside the companion-file naming convention.
	if !strings.HasSuffix(ref.Path, ".plan.md") {
		return nil, nil
	}
	name, err := manualverification.CompanionPath(ref.Path)
	if err != nil {
		return nil, taskError("manual_checks_invalid", err.Error())
	}
	reader := taskDocuments{repository: c.Repository, git: c.Git, timeout: positiveDuration(c.CommandTimeout, 30*time.Second), processHost: c.ProcessHost}
	files, err := reader.readPinnedFiles(ctx, ref.Commit, []string{name}, true)
	if err != nil {
		return nil, taskError("manual_checks_invalid", fmt.Sprintf("%s: %v", name, err))
	}
	raw, exists := files[name]
	if !exists {
		return nil, nil
	}
	frozen, err := manualverification.Freeze(name, ref.Commit, raw)
	if err != nil {
		return nil, taskError("manual_checks_invalid", err.Error())
	}
	if err := c.prepareDraftPublication(run); err != nil {
		return nil, taskError("draft_publication_unavailable", err.Error())
	}
	return frozen, nil
}

func (c Config) prepareDraftPublication(run *runnerclient.RunInput) error {
	if c.runtimeCatalog == nil || (run.Workflow.ID != "development" && run.Workflow.ID != "development-audited") || run.Publication.Provider != c.PR.Kind || !supportsManualVerificationPR(c.PR) {
		return fmt.Errorf("人工核验需要支持草稿发布的 Runner 开发工作流和 GitHub 或 Gitea 发布目标")
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(run.Publication.Settings, &settings); err != nil || settings == nil {
		return fmt.Errorf("无法读取候选的发布设置")
	}
	settings["draft"] = json.RawMessage("true")
	if c.PR.Kind == "gitea" {
		prefix := c.PR.DraftTitlePrefix
		if prefix == "" {
			prefix = "WIP:"
		}
		settings["draft_title_prefix"], _ = json.Marshal(prefix)
	}
	publication := run.Publication
	publication.Settings, _ = json.Marshal(settings)
	if err := c.runtimeCatalog.ValidateDraftPublication(publication); err != nil {
		return err
	}
	run.Publication = publication
	return nil
}

func supportsManualVerificationPR(p model.PRProviderConfig) bool {
	return p.Kind == "gitea" || p.Kind == "command" && strings.TrimRight(p.BaseURL, "/") == "https://github.com"
}
