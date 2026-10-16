package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Capabilities struct {
	NativeSessionViewer struct {
		Adapters []string `json:"adapters"`
	} `json:"native_session_viewer"`
	Protocol        string `json:"protocol"`
	Version         string `json:"version"`
	ResourceVersion int    `json:"resource_version"`
	ResourceDigest  string `json:"resource_digest"`
	Workflow        struct {
		ID                    string   `json:"id"`
		Version               int      `json:"version"`
		Steps                 []string `json:"steps"`
		PublicationReportStep string   `json:"publication_report_step"`
	} `json:"workflow"`
	ConfigurationSchemas struct {
		Version   int                        `json:"version"`
		Adapters  map[string]json.RawMessage `json:"adapters"`
		Providers map[string]json.RawMessage `json:"providers"`
	} `json:"configuration_schemas"`
}
type Catalog struct {
	Capabilities        Capabilities
	adapters, providers map[string]*jsonschema.Schema
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
	return compileCatalog(caps)
}

func compileCatalog(caps Capabilities) (*Catalog, error) {
	result := &Catalog{Capabilities: caps, adapters: map[string]*jsonschema.Schema{}, providers: map[string]*jsonschema.Schema{}}
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
	w := c.Capabilities.Workflow
	if input.ID != w.ID || input.Version != w.Version {
		return errors.New("workflow differs from the selected Runner capabilities")
	}
	allowed := map[string]bool{w.PublicationReportStep: true}
	for _, step := range w.Steps {
		allowed[step] = true
	}
	for step, body := range input.PromptOverrides {
		if !allowed[step] || body == "" {
			return errors.New("prompt override requires an advertised workflow step and nonempty body")
		}
	}
	return nil
}
