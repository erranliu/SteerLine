package executionconfig

import (
	"context"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Native model discovery runs before the write lock; Save subsequently rechecks
// the exact saved digest and deployment pin before publishing the configuration.
func (s *Store) validateContinuationSuggestionEdit(ctx context.Context, raw []byte) error {
	var edit EditRequest
	if err := cfg.Decode(raw, &edit); err != nil {
		return err
	}
	current, err := s.Current()
	if err != nil {
		return err
	}
	if sameValue(current.ContinuationSuggestion, edit.Config.ContinuationSuggestion) {
		return nil
	}
	settings, err := cfg.ContinuationSuggestionSettings(edit.Config.ContinuationSuggestion)
	if err != nil {
		return failure(400, "invalid_main_agent", err.Error())
	}
	result, err := s.QueryNativeCatalog(ctx, current.Runner, runnercontract.NativeCatalogRequest{Kind: "models", Adapter: "codex-app-server", Target: &runnercontract.ExecutionTarget{Kind: "local"}, Agent: "codex", Execution: cfg.ContinuationSuggestionBase(current.Profiles)})
	if err != nil {
		return err
	}
	return validateContinuationSuggestionModel(settings, result)
}

func validateContinuationSuggestionModel(settings cfg.ContinuationSuggestion, result runnercontract.NativeCatalogResult) error {
	// context_unverified is part of the directory contract: metadata does not
	// prove login, permission or execution readiness. Source identity is already
	// checked by QueryNativeCatalog; it must not disable every model edit.
	if result.State != "ready" || !result.Complete || result.Models == nil {
		return failure(422, "main_agent_models_unavailable", "本地 Codex 模型目录尚未核实，请刷新后再保存")
	}
	for _, model := range *result.Models {
		if model.ID == settings.Model {
			for _, effort := range model.Efforts {
				if effort.Value == settings.ReasoningEffort {
					return nil
				}
			}
		}
	}
	return failure(400, "invalid_main_agent", "所选对话续作 · 提示词建议 模型或思考强度不在当前本地 Codex 目录中")
}
