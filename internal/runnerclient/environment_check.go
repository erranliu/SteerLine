package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"runtime"
)

type CheckInput = runnercontract.CheckInput
type CheckStop = runnercontract.CheckStop
type CheckReceipt = runnercontract.CheckReceipt
type CheckInspection = runnercontract.CheckInspection
type Check = runnercontract.Check
type CheckCapabilities runnercontract.CheckCapabilities

func (p *CheckCapabilities) Supports(input ExecutionInput) bool {
	if p == nil || p.Contract != runnercontract.EnvironmentCheckContract {
		return false
	}
	var settings struct {
		Mode string `json:"access_mode"`
	}
	if json.Unmarshal(input.Settings, &settings) != nil {
		return false
	}
	if settings.Mode == "" {
		settings.Mode = "sandbox"
	}
	for _, target := range p.Targets {
		if target.Mechanism == "implemented" && (target.Platform == "" || target.Platform == runtime.GOOS) && target.Adapter == input.Adapter && target.Target == input.Target.Kind {
			for _, mode := range target.AccessModes {
				if mode == settings.Mode {
					return true
				}
			}
		}
	}
	return false
}

// Check schemas are self-contained assets of the selected Runner, never
// remotely loaded schemas or an alternate source of native permission rules.
func ValidateCheckSchema(schema json.RawMessage, value any) error {
	if len(schema) == 0 {
		return errors.New("Runner check schema is unavailable")
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return err
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(noSchemaReads{})
	c.AssertFormat()
	const uri = "https://steerrunner.invalid/environment-check/schema.json"
	if err = c.AddResource(uri, doc); err != nil {
		return err
	}
	s, err := c.Compile(uri)
	if err != nil {
		return err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return err
	}
	return s.Validate(v)
}

func (c Client) EnvironmentCheckCatalog(ctx context.Context) (*Catalog, error) {
	v, err := c.Version(ctx)
	if err != nil {
		return nil, err
	}
	return c.Catalog(ctx, v)
}
func (c Client) StartEnvironmentCheck(ctx context.Context, request Request) (CheckReceipt, error) {
	return c.checkMutation(ctx, "environment-check start", request)
}
func (c Client) StopEnvironmentCheck(ctx context.Context, request Request) (CheckReceipt, error) {
	return c.checkMutation(ctx, "environment-check stop", request)
}
func (c Client) checkMutation(ctx context.Context, command string, request Request) (CheckReceipt, error) {
	r, err := c.Call(ctx, command, nil, &request)
	if err != nil {
		return CheckReceipt{}, err
	}
	var receipt CheckReceipt
	if json.Unmarshal(r.Data, &receipt) != nil || receipt.CommandID == "" || receipt.CheckID == "" || receipt.ReceiptStatus != "accepted" || receipt.ActionStatus == "" {
		return receipt, &Failure{Code: "request_outcome_unknown", Message: "Check receipt is incomplete; inspect original request ID", Exit: 6, Unknown: true}
	}
	return receipt, nil
}
func (c Client) InspectEnvironmentCheck(ctx context.Context, selector, id string) (CheckInspection, error) {
	if (selector != "check-id" && selector != "request-id") || id == "" {
		return CheckInspection{}, errors.New("exact check or request selector required")
	}
	r, err := c.Call(ctx, "environment-check inspect", []string{"--" + selector, id}, nil)
	if err != nil {
		return CheckInspection{}, err
	}
	var result CheckInspection
	if err = json.Unmarshal(r.Data, &result); err != nil || result.Command == nil && result.Check == nil {
		return result, fmt.Errorf("Check inspection is incomplete")
	}
	if selector == "check-id" && (result.Check == nil || result.Check.CheckID != id) || selector == "request-id" && (result.Command == nil || result.Command.RequestID != id) {
		return result, errors.New("Check inspection identity differs from original selector")
	}
	result.Raw = append(json.RawMessage(nil), r.Data...)
	return result, nil
}

func (p CheckCapabilities) Definition(name string) json.RawMessage {
	return runnercontract.CheckCapabilities(p).Definition(name)
}
