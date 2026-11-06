package executionconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/web/server/nativequery"
)

type ProfileView struct {
	View     *runnercontract.ExecutionSelection `json:"view,omitempty"`
	Editable bool                               `json:"editable"`
	Reason   string                             `json:"reason,omitempty"`
}
type ProfileEdit struct {
	ProfileID     string                            `json:"profile_id"`
	BaseProfileID string                            `json:"base_profile_id,omitempty"`
	Selection     runnercontract.ExecutionSelection `json:"selection"`
	Unset         []string                          `json:"unset"`
}
type EditRequest struct {
	SavedDigest  string        `json:"saved_digest"`
	Config       Config        `json:"config"`
	ProfileEdits []ProfileEdit `json:"profile_edits"`
}
type ComposeRequest struct {
	SavedDigest   string                            `json:"saved_digest"`
	BaseProfileID string                            `json:"base_profile_id,omitempty"`
	Workflow      runnercontract.Workflow           `json:"workflow"`
	Selection     runnercontract.ExecutionSelection `json:"selection"`
	Unset         []string                          `json:"unset"`
}
type configurationValue struct {
	result runnercontract.ExecutionConfigurationResult
	err    error
}

func (s *Store) refreshConfigurationFailures() {
	for key, value := range s.configurationCache {
		if value.err != nil {
			delete(s.configurationCache, key)
		}
	}
}

// configure is called under the store lock, and caches immutable pure facts for
// one pin/resource/input. Failed operations cannot silently select a new program.
func (s *Store) configure(ctx context.Context, pin cfg.Pin, in runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error) {
	var out runnercontract.ExecutionConfigurationResult
	cat, err := s.contract(ctx, pin)
	if err != nil {
		return out, err
	}
	key := runnercontract.ConfigurationValueDigest([]any{pin, cat.ResourceDigest, in})
	if value, ok := s.configurationCache[key]; ok {
		raw, _ := json.Marshal(value.result)
		_ = json.Unmarshal(raw, &out)
		return out, value.err
	}
	query := s.Configure
	if query == nil {
		query = s.queryConfiguration
	}
	out, err = query(ctx, pin, in)
	if ctx.Err() == nil {
		if s.configurationCache == nil {
			s.configurationCache = map[string]configurationValue{}
		}
		if len(s.configurationCache) >= 256 {
			s.configurationCache = map[string]configurationValue{}
		}
		body, _ := json.Marshal(out)
		var frozen runnercontract.ExecutionConfigurationResult
		_ = json.Unmarshal(body, &frozen)
		s.configurationCache[key] = configurationValue{frozen, err}
	}
	return out, err
}
func (s *Store) queryConfiguration(ctx context.Context, pin cfg.Pin, in runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error) {
	var result runnercontract.ExecutionConfigurationResult
	cat, err := s.contract(ctx, pin)
	if err != nil {
		return result, err
	}
	var capability runnercontract.ExecutionConfigurationCapability
	if json.Unmarshal(cat.Configuration, &capability) != nil || capability.Contract != runnercontract.ExecutionConfigurationContract || capability.Command != "execution-config" || !capability.QueryOnly || capability.RequiresService == nil || *capability.RequiresService || capability.Limits.InputBytes != 1<<20 || capability.Limits.OutputBytes != 1<<20 || capability.Limits.TimeoutMS != 30000 {
		return result, failure(422, "unsupported_capability", "Runner 未发布受支持的执行配置接口")
	}
	supported := false
	for _, op := range capability.Operations {
		supported = supported || op == in.Operation
	}
	if !supported {
		return result, failure(422, "unsupported_capability", "Runner 未实现所选执行配置操作")
	}
	if err = nativeSchema(capability.RequestSchema, in); err != nil {
		return result, failure(400, "invalid_selection", "Runner 拒绝所选逻辑配置")
	}
	body, _ := json.Marshal(in)
	raw, callErr := nativequery.RunInput(ctx, s.Home, pin.Executable, []string{"execution-config", "--input", "-", "--json", "--protocol", runnercontract.Protocol}, body)
	if _, err = runnercontract.CanonicalJSON(raw); err != nil {
		return result, errors.New("Runner 执行配置响应无效")
	}
	var envelope struct {
		Protocol  string          `json:"protocol"`
		OK        bool            `json:"ok"`
		RequestID *string         `json:"request_id"`
		Data      json.RawMessage `json:"data"`
		Error     *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || string(fields["request_id"]) != "null" || fields["error"] == nil || fields["data"] == nil || json.Unmarshal(raw, &envelope) != nil || envelope.Protocol != runnercontract.Protocol || envelope.RequestID != nil {
		return result, errors.New("Runner 执行配置协议不符")
	}
	if err = verifyPin(pin); err != nil {
		return result, err
	}
	if envelope.OK != (callErr == nil) || envelope.OK == (envelope.Error != nil) {
		return result, errors.New("Runner execution configuration acceptance differs")
	}
	if !envelope.OK {
		var exit *exec.ExitError
		expected := map[string]int{"invalid_input": 2, "unsupported_capability": 3, "execution_configuration_timeout": 4, "internal_error": 1}[envelope.Error.Code]
		if !errors.As(callErr, &exit) || expected == 0 || exit.ExitCode() != expected || string(envelope.Data) != "null" {
			return result, errors.New("Runner execution configuration error or exit differs")
		}
	}
	if !envelope.OK || envelope.Error != nil || callErr != nil {
		if envelope.Error != nil {
			return result, failure(422, envelope.Error.Code, envelope.Error.Message)
		}
		return result, errors.New("Runner 执行配置失败或超时")
	}
	if nativeSchema(capability.ResponseSchema, envelope.Data) != nil || json.Unmarshal(envelope.Data, &result) != nil || result.Contract != capability.Contract || result.Operation != in.Operation || result.InputDigest != runnercontract.ConfigurationValueDigest(in) || result.Source.Version != pin.Version || !strings.EqualFold(result.Source.ExecutableSHA256, pin.SHA256) || result.Source.ResourceDigest != cat.ResourceDigest {
		return result, errors.New("Runner 执行配置来源或摘要不符")
	}
	if result.View.Adapter == "" {
		return result, errors.New("Runner logical view missing")
	}
	if in.Operation == "describe" {
		if in.Execution == nil || result.Identity == nil {
			return result, errors.New("Runner 身份缺失")
		}
		err = runnercontract.ValidateExecutionIdentity(*in.Execution, *result.Identity)
		if result.View.Adapter != in.Execution.Adapter || result.View.Target != in.Execution.Target {
			err = errors.New("Runner logical view differs from selected source")
		}
	} else if in.Operation == "compose" {
		if in.Selection == nil || result.Execution == nil || result.Identity == nil {
			return result, errors.New("Runner 构造结果缺失")
		}
		err = runnercontract.ValidateExecutionIdentity(*result.Execution, *result.Identity)
		if result.Execution.Adapter != in.Selection.Adapter || result.Execution.Target != in.Selection.Target || result.View.Adapter != in.Selection.Adapter || result.View.Target != in.Selection.Target || result.View.Agent != in.Selection.Agent || result.View.AccessMode != in.Selection.AccessMode {
			err = errors.New("Runner 更改了所选目标")
		}
	}
	if err != nil {
		return result, err
	}
	return result, nil
}
func (s *Store) profileViews(ctx context.Context, c Config) map[string]ProfileView {
	out := map[string]ProfileView{}
	for _, p := range c.Profiles {
		result, err := s.configure(ctx, c.Runner, runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: &p.Execution, Workflow: runnercontract.Workflow{ID: p.Workflow.ID, Version: p.Workflow.Version}})
		if err != nil {
			out[p.ID] = ProfileView{Reason: err.Error()}
			continue
		}
		view := result.View
		editable := productSelection(view)
		reason := ""
		if !editable {
			reason = "当前产品不支持编辑此方案"
		}
		out[p.ID] = ProfileView{View: &view, Editable: editable, Reason: reason}
	}
	return out
}

// DescribeProfile consumes a server-owned saved profile; callers cannot supply
// native settings or an executable through the discovery request.
func (s *Store) DescribeProfile(ctx context.Context, pin cfg.Pin, p cfg.Profile) (runnercontract.ExecutionSelection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.configure(ctx, pin, runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: &p.Execution, Workflow: runnercontract.Workflow{ID: p.Workflow.ID, Version: p.Workflow.Version}})
	return result.View, err
}
func (s *Store) compose(ctx context.Context, current Config, in ComposeRequest) (runnercontract.ExecutionConfigurationResult, error) {
	var empty runnercontract.ExecutionConfigurationResult
	if !productSelection(in.Selection) {
		return empty, failure(400, "unsupported_profile", "当前产品不支持所选执行组合")
	}
	cat, err := s.contract(ctx, current.Runner)
	if err != nil {
		return empty, err
	}
	allowed := false
	for _, choice := range cat.Combinations {
		if choice.Host != in.Selection.Target.Kind || choice.Adapter != in.Selection.Adapter || choice.Agent != in.Selection.Agent {
			continue
		}
		for _, w := range choice.Workflows {
			allowed = allowed || w.ID == in.Workflow.ID && w.Version == in.Workflow.Version
		}
	}
	if !allowed {
		return empty, failure(422, "unsupported_capability", "Runner 未发布所选 workflow 组合")
	}
	request := runnercontract.ExecutionConfigurationRequest{Operation: "compose", Workflow: in.Workflow, Selection: &in.Selection, Unset: in.Unset}
	if in.BaseProfileID != "" {
		for _, p := range current.Profiles {
			if p.ID == in.BaseProfileID {
				e := p.Execution
				request.BaseExecution = &e
				break
			}
		}
		if request.BaseExecution == nil {
			return empty, failure(400, "unknown_base", "原保存方案不存在")
		}
	}
	return s.configure(ctx, current.Runner, request)
}
func (s *Store) Compose(ctx context.Context, in ComposeRequest) (runnercontract.ExecutionConfigurationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshConfigurationFailures()
	current, raw, err := s.read()
	if err != nil {
		return runnercontract.ExecutionConfigurationResult{}, err
	}
	if in.SavedDigest == "" || in.SavedDigest != cfg.Digest(raw) {
		return runnercontract.ExecutionConfigurationResult{}, failure(412, "configuration_changed", "配置已变化，请重新读取后合并")
	}
	if reason := s.writeReason(); reason != "" {
		return runnercontract.ExecutionConfigurationResult{}, failure(409, "configuration_frozen", reason)
	}
	return s.compose(ctx, current, in)
}
func (s *Store) verifyEdits(ctx context.Context, current, proposed Config, edits []ProfileEdit) error {
	byID := map[string]ProfileEdit{}
	for _, edit := range edits {
		if edit.ProfileID == "" {
			return failure(400, "invalid_profile_edits", "edit 缺少 profile_id")
		}
		if _, ok := byID[edit.ProfileID]; ok {
			return failure(400, "invalid_profile_edits", "重复 profile edit")
		}
		byID[edit.ProfileID] = edit
	}
	for _, p := range proposed.Profiles {
		unchanged := false
		for _, old := range current.Profiles {
			if old.ID == p.ID {
				unchanged = sameExecution(old, p)
				break
			}
		}
		edit, has := byID[p.ID]
		if unchanged {
			if has {
				return failure(400, "invalid_profile_edits", "未变方案不能带 edit")
			}
			continue
		}
		if !has {
			return failure(400, "invalid_profile_edits", "新增或修改方案必须提供 edit")
		}
		result, err := s.compose(ctx, current, ComposeRequest{BaseProfileID: edit.BaseProfileID, Workflow: p.Workflow, Selection: edit.Selection, Unset: edit.Unset})
		if err != nil {
			return err
		}
		if result.Execution == nil || !sameValue(*result.Execution, p.Execution) {
			return failure(400, "execution_source_differs", "执行配置与可信来源重新构造的结果不同")
		}
		delete(byID, p.ID)
	}
	if len(byID) != 0 {
		return failure(400, "invalid_profile_edits", "多余或已删除方案的 edit")
	}
	return nil
}
