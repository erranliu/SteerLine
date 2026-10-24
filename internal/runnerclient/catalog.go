package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"net/url"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Capabilities struct {
	ProgressFile       *ProgressCapability `json:"progress_file,omitempty"`
	ProgressFileSchema json.RawMessage     `json:"progress_file_schema,omitempty"`
	RunUsage           *struct {
		Schema    string `json:"schema"`
		QueryOnly bool   `json:"query_only"`
	} `json:"run_usage,omitempty"`
	RunCost *struct {
		Schema        string `json:"schema"`
		UsageContract string `json:"usage_contract"`
		QueryOnly     bool   `json:"query_only"`
		Basis         string `json:"basis"`
	} `json:"run_cost,omitempty"`
	RunObservation *struct {
		Contract     string `json:"contract"`
		SameSnapshot bool   `json:"same_snapshot"`
		MaxEvents    int    `json:"max_events"`
	} `json:"run_observation,omitempty"`
	InstanceBinding struct {
		ExpectedInstanceID bool                      `json:"expected_instance_id"`
		FullBinding        *ServiceBindingCapability `json:"full_binding,omitempty"`
	} `json:"instance_binding"`
	NativeSessionViewer struct {
		Adapters []string `json:"adapters"`
	} `json:"native_session_viewer"`
	EnvironmentCheck         *CheckCapabilities              `json:"environment_check,omitempty"`
	EnvironmentCheckSchema   json.RawMessage                 `json:"environment_check_schema,omitempty"`
	EnvironmentCheckFixtures json.RawMessage                 `json:"environment_check_fixtures,omitempty"`
	Protocol                 string                          `json:"protocol"`
	Version                  string                          `json:"version"`
	ResourceVersion          int                             `json:"resource_version"`
	ResourceDigest           string                          `json:"resource_digest"`
	Workflows                []runnercontract.WorkflowChoice `json:"workflows"`
	Workflow                 struct {
		ID                    string   `json:"id"`
		Version               int      `json:"version"`
		Steps                 []string `json:"steps"`
		PublicationReportStep string   `json:"publication_report_step"`
	} `json:"workflow"`
	ConfigurationSchemas struct {
		Version          int                        `json:"version"`
		Adapters         map[string]json.RawMessage `json:"adapters"`
		Providers        map[string]json.RawMessage `json:"providers"`
		Workflow         json.RawMessage            `json:"workflow"`
		RunConfiguration json.RawMessage            `json:"run_configuration"`
		GenerateInput    json.RawMessage            `json:"generate_input"`
	} `json:"configuration_schemas"`
}
type Catalog struct {
	generate                *jsonschema.Schema
	Capabilities            Capabilities
	adapters, providers     map[string]*jsonschema.Schema
	workflow, configuration *jsonschema.Schema
}
type noSchemaReads struct{}

func (noSchemaReads) Load(string) (any, error) {
	return nil, errors.New("configuration schemas must be self contained in Runner capabilities")
}

func (c Client) Catalog(ctx context.Context, version Version) (*Catalog, error) {
	response, err := c.Call(ctx, "capabilities", nil, nil)
	if err != nil {
		return nil, err
	}
	var caps Capabilities
	if err := json.Unmarshal(response.Data, &caps); err != nil {
		return nil, err
	}
	if caps.Protocol != Protocol || caps.Version != version.Version || caps.ResourceVersion != version.ResourceVersion || caps.ResourceDigest == "" || caps.ResourceDigest != version.ResourceDigest || caps.ConfigurationSchemas.Version != 1 {
		return nil, errors.New("Runner capabilities differ from its selected executable/version")
	}
	if !caps.InstanceBinding.ExpectedInstanceID {
		return nil, errors.New("selected Runner does not publish expected-instance fencing")
	}
	if binding := caps.InstanceBinding.FullBinding; binding != nil {
		if binding.Contract != ServiceBindingContract || !binding.BeforeEffects || !binding.NoImplicitStart || len(binding.Commands) == 0 {
			return nil, errors.New("selected Runner publishes incomplete full-binding admission")
		}
		seen := map[string]bool{}
		for _, command := range binding.Commands {
			if command == "" || seen[command] {
				return nil, errors.New("selected Runner publishes invalid full-binding command coverage")
			}
			seen[command] = true
		}
	}
	if len(caps.Workflows) == 0 || len(caps.ConfigurationSchemas.Workflow) == 0 || len(caps.ConfigurationSchemas.RunConfiguration) == 0 {
		return nil, errors.New("selected Runner does not publish workflow selection and run configuration contracts")
	}
	if caps.EnvironmentCheck != nil {
		caps.EnvironmentCheck.Schema = caps.EnvironmentCheckSchema
		if caps.EnvironmentCheck.Contract == runnercontract.EnvironmentCheckContract {
			var schema struct {
				Contract    string                     `json:"contract"`
				Definitions map[string]json.RawMessage `json:"$defs"`
			}
			if json.Unmarshal(caps.EnvironmentCheckSchema, &schema) != nil || schema.Contract != caps.EnvironmentCheck.Contract || caps.EnvironmentCheck.InputSchema != "steerrunner/environment-check/v1#/$defs/input" || caps.EnvironmentCheck.ReportSchema != "steerrunner/environment-check/v1#/$defs/report" {
				return nil, errors.New("Runner environment check schema differs from its advertised contract")
			}
			for _, name := range []string{"input", "receipt", "report", "inspection"} {
				if len(schema.Definitions[name]) == 0 {
					return nil, errors.New("Runner environment check schema is incomplete")
				}
			}
		}
	}
	return compileCatalog(caps)
}

func compileCatalog(caps Capabilities) (*Catalog, error) {
	result := &Catalog{Capabilities: caps, adapters: map[string]*jsonschema.Schema{}, providers: map[string]*jsonschema.Schema{}}
	seen := map[string]bool{}
	for _, workflow := range caps.Workflows {
		key := fmt.Sprintf("%s/%d", workflow.ID, workflow.Version)
		if workflow.ID == "" || workflow.Version <= 0 || seen[key] || len(workflow.SupportedAdapters) == 0 {
			return nil, errors.New("Runner workflow catalog is incomplete or duplicated")
		}
		seen[key] = true
	}
	for _, entry := range []struct {
		raw    json.RawMessage
		target **jsonschema.Schema
		name   string
	}{
		{caps.ConfigurationSchemas.Workflow, &result.workflow, "workflow"},
		{caps.ConfigurationSchemas.RunConfiguration, &result.configuration, "run-configuration"},
		{caps.ConfigurationSchemas.GenerateInput, &result.generate, "generate-input"},
	} {
		if len(entry.raw) == 0 {
			continue
		}
		schema, err := compilePublicSchema(entry.raw, entry.name)
		if err != nil {
			return nil, err
		}
		*entry.target = schema
	}
	for _, group := range []struct {
		name   string
		source map[string]json.RawMessage
		target map[string]*jsonschema.Schema
	}{{"adapter", caps.ConfigurationSchemas.Adapters, result.adapters}, {"provider", caps.ConfigurationSchemas.Providers, result.providers}} {
		for key, raw := range group.source {
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				return nil, err
			}
			compiler := jsonschema.NewCompiler()
			compiler.UseLoader(noSchemaReads{})
			compiler.AssertFormat()
			location := "https://steerrunner.invalid/config/" + group.name + "/" + url.PathEscape(key) + ".json"
			if err := compiler.AddResource(location, doc); err != nil {
				return nil, err
			}
			schema, err := compiler.Compile(location)
			if err != nil {
				return nil, err
			}
			group.target[key] = schema
		}
	}
	return result, nil
}

// CompileCatalog validates a published capability document without starting a
// service; execution admission remains the selected Runner's responsibility.
func CompileCatalog(caps Capabilities) (*Catalog, error) { return compileCatalog(caps) }
func validateSettings(schemas map[string]*jsonschema.Schema, key string, raw json.RawMessage) error {
	schema := schemas[key]
	if schema == nil {
		return fmt.Errorf("selected Runner does not publish settings for %s", key)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return schema.Validate(value)
}
func (c *Catalog) ValidateExecution(input ExecutionInput) error {
	if input.Target.Kind == "" {
		return errors.New("execution target is required")
	}
	return validateSettings(c.adapters, input.Adapter, input.Settings)
}
func (c *Catalog) ValidatePublication(input Publication) error {
	return validateSettings(c.providers, input.Provider, input.Settings)
}
func (c *Catalog) ValidateWorkflow(input Workflow) error {
	for _, choice := range c.Capabilities.Workflows {
		if input.ID == choice.ID && input.Version == choice.Version {
			return validatePublicValue(c.workflow, input)
		}
	}
	return errors.New("workflow is not advertised by the selected Runner")
}

func (c *Catalog) ValidateConfiguration(execution ExecutionInput, workflow Workflow) error {
	if err := c.ValidateWorkflow(workflow); err != nil {
		return err
	}
	return validatePublicValue(c.configuration, struct {
		Execution ExecutionInput `json:"execution"`
		Workflow  Workflow       `json:"workflow"`
	}{execution, workflow})
}

func compilePublicSchema(raw json.RawMessage, name string) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(noSchemaReads{})
	compiler.AssertFormat()
	uri := "https://steerrunner.invalid/" + name
	if err := compiler.AddResource(uri, doc); err != nil {
		return nil, err
	}
	return compiler.Compile(uri)
}

func validatePublicValue(schema *jsonschema.Schema, value any) error {
	if schema == nil {
		return errors.New("selected Runner does not publish the required configuration schema")
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return err
	}
	return schema.Validate(v)
}
