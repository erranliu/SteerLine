package executionconfig

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/web/server/nativequery"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type catalog struct {
	Configuration  json.RawMessage
	Native         json.RawMessage
	ResourceDigest string
	Schema         *jsonschema.Schema
	Workflows      []runnercontract.WorkflowChoice
	Efforts        []string
	Combinations   []Combination
}
type noRemoteSchema struct{}

func (noRemoteSchema) Load(string) (any, error) {
	return nil, errors.New("configuration schemas must be self contained")
}
func verifyPin(pin cfg.Pin) error {
	if !filepath.IsAbs(pin.Executable) || pin.Version == "" || len(pin.SHA256) != 64 {
		return errors.New("Runner release pin is incomplete")
	}
	f, err := os.Open(pin.Executable)
	if err != nil {
		return errors.New("selected Runner executable is unavailable")
	}
	defer f.Close()
	// EXE identity, not a service status or the Web capability cache.
	h, err := hashReader(f)
	if err != nil || !strings.EqualFold(h, pin.SHA256) {
		return errors.New("selected Runner executable differs from its release SHA-256")
	}
	return nil
}
func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	_, err := io.Copy(h, r)
	return hex.EncodeToString(h.Sum(nil)), err
}
func (s *Store) contract(ctx context.Context, pin cfg.Pin) (*catalog, error) {
	if err := verifyPin(pin); err != nil {
		return nil, err
	}
	if s.catalog != nil && s.catalogPin == pin {
		return s.catalog, nil
	}
	reader := func(ctx context.Context, pin cfg.Pin) (*catalog, error) { return readCatalog(ctx, s.Home, pin) }
	if s.ReadCatalog != nil {
		reader = s.ReadCatalog
	}
	cat, err := reader(ctx, pin)
	if err != nil {
		return nil, err
	}
	s.catalog = cat
	s.catalogPin = pin
	return cat, nil
}
func readCatalog(ctx context.Context, home string, pin cfg.Pin) (*catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	type metadata struct {
		Configuration        json.RawMessage `json:"execution_configuration"`
		CapabilityCatalog    json.RawMessage `json:"capability_catalog"`
		Native               json.RawMessage `json:"native_catalog"`
		Protocol             string          `json:"protocol"`
		Version              string          `json:"version"`
		ResourceDigest       string          `json:"resource_digest"`
		ConfigurationSchemas struct {
			Version          int                        `json:"version"`
			RunConfiguration json.RawMessage            `json:"run_configuration"`
			Adapters         map[string]json.RawMessage `json:"adapters"`
		} `json:"configuration_schemas"`
		Workflows []runnercontract.WorkflowChoice `json:"workflows"`
	}
	read := func(args []string) (metadata, error) {
		var result metadata
		raw, err := nativequery.Run(ctx, home, pin.Executable, args)
		if err != nil {
			return result, errors.New("Runner offline configuration contract is unavailable")
		}
		var envelope struct {
			OK       bool     `json:"ok"`
			Protocol string   `json:"protocol"`
			Data     metadata `json:"data"`
		}
		if json.Unmarshal(raw, &envelope) != nil || !envelope.OK || envelope.Protocol != "steerrunner-cli/v1" {
			return result, errors.New("Runner offline metadata protocol is unsupported")
		}
		return envelope.Data, nil
	}
	version, err := read([]string{"version", "--json"})
	if err != nil {
		return nil, err
	}
	caps, err := read([]string{"capabilities", "--offline", "--json"})
	if err != nil {
		return nil, err
	}
	if err = verifyPin(pin); err != nil {
		return nil, err
	}
	if version.Version != pin.Version || caps.Version != pin.Version || len(version.ResourceDigest) != 64 || caps.ResourceDigest != version.ResourceDigest || caps.ConfigurationSchemas.Version != 1 {
		return nil, errors.New("Runner offline configuration identity differs from its release")
	}
	result, err := compileCatalog(caps.ConfigurationSchemas.RunConfiguration, caps.Workflows, caps.ConfigurationSchemas.Adapters["codex-app-server"])
	if err != nil {
		return nil, err
	}
	result.Configuration = caps.Configuration
	result.project(caps.CapabilityCatalog)
	result.Native = caps.Native
	result.ResourceDigest = caps.ResourceDigest
	return result, nil
}
func compileCatalog(raw json.RawMessage, workflows []runnercontract.WorkflowChoice, adapter json.RawMessage) (*catalog, error) {
	if len(raw) == 0 || len(workflows) == 0 {
		return nil, errors.New("Runner does not publish configuration schemas")
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(noRemoteSchema{})
	c.AssertFormat()
	if err = c.AddResource("https://steerline.invalid/run-configuration", doc); err != nil {
		return nil, err
	}
	schema, err := c.Compile("https://steerline.invalid/run-configuration")
	if err != nil {
		return nil, err
	}
	var fields struct {
		Properties struct {
			Effort struct {
				Enum []string `json:"enum"`
			} `json:"reasoning_effort"`
		} `json:"properties"`
	}
	json.Unmarshal(adapter, &fields)
	if fields.Properties.Effort.Enum == nil {
		fields.Properties.Effort.Enum = []string{}
	}
	cat := &catalog{Schema: schema, Workflows: workflows, Efforts: fields.Properties.Effort.Enum}
	return cat, nil
}
func (c *catalog) Validate(p cfg.Profile) error {
	raw, err := json.Marshal(map[string]any{"execution": p.Execution, "workflow": p.Workflow})
	if err != nil {
		return err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return c.Schema.Validate(doc)
}

// Combinations are the intersection of Web's explicit parameter mappings and
// the fixed release's complete public schema, never inferred from a live catalog.
type Combination struct {
	Host      string                          `json:"host"`
	Adapter   string                          `json:"adapter"`
	Agent     string                          `json:"agent"`
	Supported bool                            `json:"supported"`
	Efforts   []string                        `json:"efforts"`
	Workflows []runnercontract.WorkflowChoice `json:"workflows"`
}

// project consumes the finite release catalog. It never probes fabricated
// native settings against a schema to infer execution support.
func (c *catalog) project(raw json.RawMessage) {
	var published struct {
		Version      int `json:"version"`
		Combinations []struct {
			Adapter    string                  `json:"adapter"`
			Host       string                  `json:"target_kind"`
			Agent      string                  `json:"agent"`
			Workflow   runnercontract.Workflow `json:"workflow"`
			AccessMode string                  `json:"access_mode"`
		} `json:"combinations"`
	}
	if json.Unmarshal(raw, &published) != nil || published.Version != 1 {
		return
	}
	var logical runnercontract.ExecutionConfigurationCapability
	if json.Unmarshal(c.Configuration, &logical) != nil {
		return
	}
	for _, entry := range published.Combinations {
		if entry.AccessMode != "full_access" || !productSelection(runnercontract.ExecutionSelection{Adapter: entry.Adapter, Target: runnercontract.ExecutionTarget{Kind: entry.Host, Environment: map[bool]string{true: "catalog-target"}[entry.Host == "orca_environment"]}, Agent: entry.Agent}) {
			continue
		}
		for _, workflow := range c.Workflows {
			if workflow.ID != entry.Workflow.ID || workflow.Version != entry.Workflow.Version || (workflow.ID != "development" && workflow.ID != "development-audited") {
				continue
			}
			i := -1
			for j, choice := range c.Combinations {
				if choice.Host == entry.Host && choice.Adapter == entry.Adapter && choice.Agent == entry.Agent {
					i = j
					break
				}
			}
			if i < 0 {
				c.Combinations = append(c.Combinations, Combination{Host: entry.Host, Adapter: entry.Adapter, Agent: entry.Agent, Supported: true, Efforts: logical.ReasoningEfforts[entry.Agent], Workflows: []runnercontract.WorkflowChoice{}})
				i = len(c.Combinations) - 1
			}
			c.Combinations[i].Workflows = append(c.Combinations[i].Workflows, workflow)
		}
	}
}
func productSelection(view runnercontract.ExecutionSelection) bool {
	if view.Target.Kind == "local" && view.Target.Environment == "" {
		return view.Adapter == "codex-app-server" && view.Agent == "codex" || view.Adapter == "orca-terminal" && runtime.GOOS == "windows" && runtime.GOARCH == "amd64" && (view.Agent == "codex" || view.Agent == "pi")
	}
	return view.Adapter == "orca-terminal" && view.Target.Kind == "orca_environment" && strings.TrimSpace(view.Target.Environment) != "" && view.Agent == "codex"
}
