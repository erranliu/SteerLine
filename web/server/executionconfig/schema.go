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
	"sort"
	"strings"
	"time"
)

type catalog struct {
	Schema       *jsonschema.Schema
	Workflows    []runnercontract.WorkflowChoice
	Efforts      []string
	Combinations []Combination
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
	reader := readCatalog
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
func readCatalog(ctx context.Context, pin cfg.Pin) (*catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	type metadata struct {
		Protocol             string `json:"protocol"`
		Version              string `json:"version"`
		ResourceDigest       string `json:"resource_digest"`
		ConfigurationSchemas struct {
			Version          int                        `json:"version"`
			RunConfiguration json.RawMessage            `json:"run_configuration"`
			Adapters         map[string]json.RawMessage `json:"adapters"`
		} `json:"configuration_schemas"`
		Workflows []runnercontract.WorkflowChoice `json:"workflows"`
	}
	read := func(args []string) (metadata, error) {
		var result metadata
		raw, err := nativequery.Run(ctx, pin.Executable, args)
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
	return compileCatalog(caps.ConfigurationSchemas.RunConfiguration, caps.Workflows, caps.ConfigurationSchemas.Adapters["codex-app-server"])
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
	cat.project(doc)
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

func (c *catalog) project(doc any) {
	candidates := []string{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if e, ok := x["reasoning_effort"].(map[string]any); ok {
				if values, ok := e["enum"].([]any); ok {
					for _, v := range values {
						if str, ok := v.(string); ok {
							found := false
							for _, a := range candidates {
								found = found || a == str
							}
							if !found {
								candidates = append(candidates, str)
							}
						}
					}
				}
			}
			for _, v := range x {
				walk(v)
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(doc)
	sort.Strings(candidates)
	for _, mapping := range [][3]string{{"local", "codex-app-server", "codex"}, {"local", "orca-terminal", "codex"}, {"local", "orca-terminal", "pi"}, {"local", "orca-terminal", "dsh"}, {"orca_environment", "orca-terminal", "codex"}} {
		if mapping[0] == "local" && mapping[1] == "orca-terminal" && (runtime.GOOS != "windows" || runtime.GOARCH != "amd64") {
			continue
		}
		choice := Combination{Host: mapping[0], Adapter: mapping[1], Agent: mapping[2], Efforts: []string{}, Workflows: []runnercontract.WorkflowChoice{}}
		for _, w := range c.Workflows {
			if w.ID != "development" && w.ID != "development-audited" {
				continue
			}
			settings := map[string]any{"model": "web-schema-model"}
			if choice.Adapter == "orca-terminal" {
				launch := map[string]any{"model": "web-schema-model"}
				if choice.Host == "local" {
					launch["runtime_executable"] = "C:/deployment/steerrunner-agent.exe"
					launch["runtime_sha256"] = strings.Repeat("a", 64)
				}
				settings = map[string]any{"agent": choice.Agent, "agent_launch": launch}
			}
			raw, _ := json.Marshal(settings)
			p := cfg.Profile{Execution: runnercontract.ExecutionInput{Adapter: choice.Adapter, Target: runnercontract.ExecutionTarget{Kind: choice.Host}, Settings: raw}, Workflow: runnercontract.Workflow{ID: w.ID, Version: w.Version}}
			if choice.Host == "orca_environment" {
				p.Execution.Target.Environment = "web-schema-target"
			}
			if w.AuditModelRequired {
				p.Workflow.AuditModel = &runnercontract.ModelSettings{Model: "web-schema-audit"}
			}
			if c.Validate(p) != nil {
				continue
			}
			choice.Supported = true
			choice.Workflows = append(choice.Workflows, w)
			if len(choice.Efforts) == 0 {
				for _, effort := range candidates {
					if choice.Adapter == "orca-terminal" {
						settings["agent_launch"].(map[string]any)["reasoning_effort"] = effort
					} else {
						settings["reasoning_effort"] = effort
					}
					p.Execution.Settings, _ = json.Marshal(settings)
					if c.Validate(p) == nil {
						choice.Efforts = append(choice.Efforts, effort)
					}
				}
			}
		}
		c.Combinations = append(c.Combinations, choice)
	}
}
