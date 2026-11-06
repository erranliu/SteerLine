package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

func (c *Catalog) ConfigurationCapability(operation string) (runnercontract.ExecutionConfigurationCapability, error) {
	var capability runnercontract.ExecutionConfigurationCapability
	if c == nil || json.Unmarshal(c.Capabilities.ExecutionConfiguration, &capability) != nil || capability.Contract != runnercontract.ExecutionConfigurationContract || capability.Command != "execution-config" || !capability.QueryOnly || capability.RequiresService == nil || *capability.RequiresService || !slices.Contains(capability.Operations, operation) || capability.Limits.InputBytes != runnercontract.ExecutionConfigurationLimit || capability.Limits.OutputBytes != runnercontract.ExecutionConfigurationLimit || capability.Limits.TimeoutMS != 30000 || len(capability.RequestSchema) == 0 || len(capability.ResponseSchema) == 0 {
		return capability, errors.New("Runner未发布受支持的执行配置能力：" + operation)
	}
	return capability, nil
}

// ConfigureExecution is stateless even when this Client is bound to a resident
// daemon. It never sends a state-dir, instance binding or mutation envelope.
func (c Client) ConfigureExecution(ctx context.Context, catalog *Catalog, input runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error) {
	var result runnercontract.ExecutionConfigurationResult
	capability, err := catalog.ConfigurationCapability(input.Operation)
	if err != nil {
		return result, err
	}
	request, err := json.Marshal(input)
	if err != nil || len(request) > capability.Limits.InputBytes {
		return result, errors.New("Runner configuration input exceeds its limit")
	}
	if err = validateConfigurationValue(capability.RequestSchema, input); err != nil {
		return result, fmt.Errorf("Runner configuration input: %w", err)
	}
	if err = VerifyExecutable(c.Executable, c.SHA256); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, c.Executable, "execution-config", "--input", "-", "--json", "--protocol", Protocol)
	hide(command)
	command.Stdin = bytes.NewReader(request)
	command.WaitDelay = time.Second
	output, diagnostic := &bounded{max: runnercontract.ExecutionConfigurationLimit}, &bounded{max: 64 << 10}
	command.Stdout = output
	command.Stderr = diagnostic
	callErr := command.Run()
	if err = VerifyExecutable(c.Executable, c.SHA256); err != nil {
		return result, err
	}
	if _, err = runnercontract.CanonicalJSON(output.Bytes()); err != nil {
		return result, errors.New("Runner configuration response is unavailable")
	}
	var response Response
	if decodeEnvelope(output.Bytes(), &response) != nil || response.Protocol != Protocol || response.RequestID != nil || response.OK == (response.Error != nil) || response.OK != (callErr == nil) {
		return result, errors.New("Runner configuration envelope differs")
	}
	if !response.OK {
		var failure *exec.ExitError
		if !errors.As(callErr, &failure) || failure.ExitCode() != exitCode(response.Error.Code) {
			return result, errors.New("Runner configuration exit code differs")
		}
		return result, &Failure{Code: response.Error.Code, Message: response.Error.Message, Exit: failure.ExitCode()}
	}
	if err = validateConfigurationValue(capability.ResponseSchema, json.RawMessage(response.Data)); err != nil {
		return result, errors.New("Runner configuration response schema differs")
	}
	if json.Unmarshal(response.Data, &result) != nil || result.Contract != capability.Contract || result.Operation != input.Operation || result.InputDigest != runnercontract.ConfigurationValueDigest(input) || result.Source.Version != c.BuildVersion || !strings.EqualFold(result.Source.ExecutableSHA256, c.SHA256) || result.Source.ResourceDigest != catalog.Capabilities.ResourceDigest {
		return result, errors.New("Runner configuration source or input digest differs")
	}
	var expected runnercontract.ExecutionInput
	switch input.Operation {
	case "describe":
		if input.Execution == nil || result.Identity == nil || result.Execution != nil || result.SourceIdentity != nil {
			return result, errors.New("Runner configuration description incomplete")
		}
		expected = *input.Execution
		if err = runnercontract.ValidateExecutionIdentity(expected, *result.Identity); err != nil {
			return result, err
		}
	case "derive":
		if input.Execution == nil || result.Execution == nil || result.SourceIdentity == nil || result.Identity != nil {
			return result, errors.New("Runner derived configuration incomplete")
		}
		if err = runnercontract.ValidateExecutionIdentity(*input.Execution, *result.SourceIdentity); err != nil {
			return result, err
		}
		expected = *result.Execution
		if expected.Adapter != input.Execution.Adapter || expected.Target != input.Execution.Target || result.View.AccessMode != input.AccessMode {
			return result, errors.New("Runner derived configuration changed selected target or permission")
		}
	case "compose":
		if input.Selection == nil || result.Execution == nil || result.Identity == nil || result.SourceIdentity != nil {
			return result, errors.New("Runner composed configuration incomplete")
		}
		expected = *result.Execution
		if expected.Adapter != input.Selection.Adapter || expected.Target != input.Selection.Target || result.View.Agent != input.Selection.Agent || result.View.AccessMode != input.Selection.AccessMode {
			return result, errors.New("Runner composed configuration changed selected target")
		}
		if err = runnercontract.ValidateExecutionIdentity(expected, *result.Identity); err != nil {
			return result, err
		}
	default:
		return result, errors.New("unsupported configuration operation")
	}
	if result.View.Adapter != expected.Adapter || result.View.Target != expected.Target {
		return result, errors.New("Runner logical view differs from its execution")
	}
	return result, nil
}

// ConfigurationSnapshot caches only pure results for one fixed executable and
// catalog. Results are serialized on return so no caller can mutate the cache.
type ConfigurationSnapshot struct {
	// Query is an injected public protocol peer for isolated owner tests.
	// Production snapshots use Client.ConfigureExecution.
	Query   func(context.Context, runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error)
	Client  Client
	Catalog *Catalog
	mu      sync.Mutex
	values  map[string]configurationSnapshotValue
}
type configurationSnapshotValue struct {
	body []byte
	err  error
}

// Identity reads only independently obtained facts in this frozen snapshot.
// Storage uses it as a trusted lookup, never as an external query in a transaction.
func (s *ConfigurationSnapshot) Identity(execution runnercontract.ExecutionInput) (runnercontract.ExecutionIdentity, error) {
	if s == nil {
		return runnercontract.ExecutionIdentity{}, errors.New("Runner执行配置快照不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	digest := runnercontract.ConfigurationValueDigest(execution)
	for _, value := range s.values {
		if value.err != nil {
			continue
		}
		var result runnercontract.ExecutionConfigurationResult
		_ = json.Unmarshal(value.body, &result)
		for _, identity := range []*runnercontract.ExecutionIdentity{result.Identity, result.SourceIdentity} {
			if identity != nil && identity.SourceExecutionDigest == digest {
				return *identity, nil
			}
		}
	}
	return runnercontract.ExecutionIdentity{}, errors.New("原执行配置没有可信 Runner 身份快照")
}

// RefreshFailures is used only by explicit user preparation/refresh actions.
// Ordinary maintenance polling keeps both successful facts and diagnostics cached.
func (s *ConfigurationSnapshot) RefreshFailures() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range s.values {
		if value.err != nil {
			delete(s.values, key)
		}
	}
}

// Lookup never invokes a process; product status projections use only the
// snapshots prepared before the project's runtime was exposed.
func (s *ConfigurationSnapshot) Lookup(in runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error) {
	if s == nil {
		return runnercontract.ExecutionConfigurationResult{}, errors.New("Runner执行配置快照不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[runnercontract.ConfigurationValueDigest(in)]
	if !ok {
		return runnercontract.ExecutionConfigurationResult{}, errors.New("Runner执行配置快照尚未准备")
	}
	var result runnercontract.ExecutionConfigurationResult
	_ = json.Unmarshal(value.body, &result)
	return result, value.err
}

func (s *ConfigurationSnapshot) Resolve(ctx context.Context, in runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error) {
	if s == nil {
		return runnercontract.ExecutionConfigurationResult{}, errors.New("Runner执行配置快照不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Catalog == nil && s.Query == nil {
		version, err := s.Client.Version(ctx)
		if err != nil {
			return runnercontract.ExecutionConfigurationResult{}, err
		}
		catalog, err := s.Client.OfflineCatalog(ctx, version)
		if err != nil {
			return runnercontract.ExecutionConfigurationResult{}, err
		}
		s.Client.BuildVersion, s.Client.ResourceDigest = version.Version, version.ResourceDigest
		s.Catalog = catalog
	}
	key := runnercontract.ConfigurationValueDigest(in)
	if value, ok := s.values[key]; ok {
		var result runnercontract.ExecutionConfigurationResult
		_ = json.Unmarshal(value.body, &result)
		return result, value.err
	}
	query := s.Query
	if query == nil {
		query = func(ctx context.Context, in runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error) {
			return s.Client.ConfigureExecution(ctx, s.Catalog, in)
		}
	}
	result, err := query(ctx, in)
	if ctx.Err() != nil {
		return result, err
	}
	if s.values == nil {
		s.values = map[string]configurationSnapshotValue{}
	}
	body, _ := json.Marshal(result)
	s.values[key] = configurationSnapshotValue{body, err}
	return result, err
}

func validateConfigurationValue(raw json.RawMessage, value any) error {
	schema, err := compilePublicSchema(raw, "execution-configuration")
	if err != nil {
		return err
	}
	return validatePublicValue(schema, value)
}
