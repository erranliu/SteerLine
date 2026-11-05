package executionconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/web/server/nativequery"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"strings"
	"time"
)

func nativeSchema(raw json.RawMessage, value any) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return errors.New("Runner native catalog schema unavailable")
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(noRemoteSchema{})
	compiler.AssertFormat()
	const uri = "https://steerrunner.invalid/native-catalog"
	if err = compiler.AddResource(uri, doc); err != nil {
		return err
	}
	schema, err := compiler.Compile(uri)
	if err != nil {
		return err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return err
	}
	return schema.Validate(parsed)
}

// QueryNativeCatalog consumes only the selected executable's public metadata.
func (s *Store) QueryNativeCatalog(ctx context.Context, pin cfg.Pin, in runnercontract.NativeCatalogRequest) (runnercontract.NativeCatalogResult, error) {
	var result runnercontract.NativeCatalogResult
	s.mu.Lock()
	cat, err := s.contract(ctx, pin)
	s.mu.Unlock()
	if err != nil {
		return result, err
	}
	var capability runnercontract.NativeCatalogCapability
	if json.Unmarshal(cat.Native, &capability) != nil || capability.Contract != runnercontract.NativeCatalogContract || capability.Command != "native-catalog" || !capability.QueryOnly || capability.RequiresService == nil || *capability.RequiresService || capability.Limits.InputBytes != 65536 || capability.Limits.OutputBytes != nativequery.Limit || capability.Limits.TimeoutMS != 30000 || capability.Limits.CleanupTimeoutMS != 5000 {
		return result, failure(422, "unsupported_capability", "Runner 未发布受支持的原生目录接口")
	}
	supported := false
	for _, query := range capability.Queries {
		if query.Kind == in.Kind && query.Adapter == in.Adapter && (in.Target == nil && query.TargetKind == "" || in.Target != nil && query.TargetKind == in.Target.Kind) {
			supported = true
		}
	}
	if !supported {
		return result, failure(422, "unsupported_capability", "Runner 未实现所选原生目录查询")
	}
	if err = nativeSchema(capability.RequestSchema, in); err != nil {
		return result, errors.New("Runner native catalog request schema rejected the query")
	}
	body, err := json.Marshal(in)
	if err != nil || len(body) > capability.Limits.InputBytes {
		return result, errors.New("native catalog input exceeds limit")
	}
	// The query context owns the 30 s collection budget. Cleanup has its own 5 s bound.
	raw, callErr := nativequery.RunInput(ctx, s.Home, pin.Executable, []string{"native-catalog", "--input", "-", "--json", "--protocol", "steerrunner-cli/v1"}, body)
	if _, err = runnercontract.CanonicalJSON(raw); err != nil {
		return result, errors.New("invalid Runner native catalog response")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || string(fields["request_id"]) != "null" || fields["data"] == nil || fields["error"] == nil {
		return result, errors.New("Runner native catalog envelope is incomplete")
	}
	var envelope struct {
		Protocol  string          `json:"protocol"`
		RequestID *string         `json:"request_id"`
		OK        bool            `json:"ok"`
		Data      json.RawMessage `json:"data"`
		Error     *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Protocol != "steerrunner-cli/v1" || envelope.RequestID != nil {
		return result, errors.New("Runner native catalog envelope differs")
	}
	if err = verifyPin(pin); err != nil {
		return result, err
	}
	if !envelope.OK || envelope.Error != nil || callErr != nil {
		if envelope.Error != nil && envelope.Error.Code == "unsupported_capability" {
			return result, failure(422, "unsupported_capability", "目标不支持所选原生目录查询")
		}
		return result, errors.New("Runner 原生目录查询失败、超时或清理未核实")
	}
	var value any
	if json.Unmarshal(envelope.Data, &value) != nil || nativeSchema(capability.ResponseSchema, value) != nil || json.Unmarshal(envelope.Data, &result) != nil {
		return result, errors.New("Runner native catalog data is incomplete")
	}
	if result.Contract != capability.Contract || result.Kind != in.Kind || result.Adapter != in.Adapter || result.Agent != in.Agent || !sameValue(result.Target, in.Target) || result.Source.Version != pin.Version || !strings.EqualFold(result.Source.ExecutableSHA256, pin.SHA256) || result.Source.ResourceDigest != cat.ResourceDigest || result.ObservedAt.IsZero() || !result.ContextUnverified || result.ObservedAt.After(time.Now().Add(time.Minute)) {
		return result, errors.New("Runner native catalog source or target differs")
	}
	return result, nil
}
