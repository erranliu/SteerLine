package testmanagement

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestTMGenerationOutputSchemaAcceptsOnlyFrozenCatalogueIDs(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "catalogue", true: "empty-catalogue"}[empty], func(t *testing.T) {
			s, _, peer, in := generationFixture(t)
			first, second := definition(), definition()
			first.ID, second.ID = "N101-CONTENT", "demo.progress.retry"
			definitions := []Definition{first, second}
			if empty {
				definitions = []Definition{}
			}
			peer.files[CataloguePath], _ = json.Marshal(Catalogue{Tests: definitions})
			if _, err := s.Generate(context.Background(), in); err != nil {
				t.Fatal(err)
			}
			var request runnercontract.GenerateRunInput
			if err := json.Unmarshal(peer.request.Input, &request); err != nil {
				t.Fatal(err)
			}
			document, err := jsonschema.UnmarshalJSON(bytes.NewReader(request.Generate.OutputSchema))
			if err != nil {
				t.Fatal(err)
			}
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("urn:test:actual-generation-output", document); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile("urn:test:actual-generation-output")
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				body  string
				valid bool
			}{
				{`{"tests":[]}`, true},
				{`{"tests":["N101-CONTENT","demo.progress.retry"]}`, !empty},
				{`{"tests":["N101-CONTENT — SteerlineFlow/plans/native-101.plan.md"]}`, false},
				{`{"tests":["N101-CONTENT (L4) — python tools/test-native-evidence.py"]}`, false},
				{`{"tests":["unknown"]}`, false},
				{`{"tests":["N101-CONTENT","N101-CONTENT"]}`, false},
				{`{"tests":[],"explanation":"extra"}`, false},
			} {
				value, err := jsonschema.UnmarshalJSON(strings.NewReader(test.body))
				if err != nil {
					t.Fatal(err)
				}
				if err := schema.Validate(value); (err == nil) != test.valid {
					t.Errorf("actual outgoing schema: %s valid=%v: %v", test.body, test.valid, err)
				}
			}
		})
	}
}
