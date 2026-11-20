package boundary

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/steeragent/maintenance"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestMaintenanceDecisionSchemaMatchesOptionalQuestion(t *testing.T) {
	for _, mode := range []string{"first", "select", "audit"} {
		t.Run(mode, func(t *testing.T) {
			var document any
			if err := json.Unmarshal(maintenance.Schema(mode), &document); err != nil {
				t.Fatal(err)
			}
			compiler := jsonschema.NewCompiler()
			const url = "https://schema.invalid/maintenance-decision"
			if err := compiler.AddResource(url, document); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile(url)
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				body  string
				valid bool
			}{
				{`{"kind":"execute","summary":"文档过期","prompt":"修正文档"}`, true},
				{`{"kind":"execute","summary":"文档过期","prompt":"修正文档","question": null}`, true},
				{`{"kind":"finish","summary":"无需改动","question":null}`, true},
				{`{"kind":"blocked","summary":"缺少事实","question":null}`, true},
				{`{"kind":"accept","summary":"证据充分","question":null}`, mode == "audit"},
				{`{"kind":"ask","summary":"需要决定","question":null}`, false},
				{`{"kind":"ask","summary":"需要决定"}`, false},
				{`{"kind":"ask","summary":"需要决定","question":{}}`, false},
				{`{"kind":"execute","summary":"文档过期","prompt":"修正文档","question":""}`, false},
				{`{"kind":"execute","summary":"文档过期","prompt":"修正文档","question":{}}`, false},
				{`{"kind":"execute","summary":"文档过期","prompt":"修正文档","question":{"prompt":"是否继续？","facts":["范围改变"],"attempts":[],"options":[{"id":"keep","label":"保留","impact":"不扩范围"}],"recommended_option_id":"keep","recommendation_reason":"保持边界"}}`, false},
				{`{"kind":"ask","summary":"需要决定","question":{"prompt":"是否继续？","facts":["范围改变"],"attempts":[],"options":[{"id":"keep","label":"保留","impact":"不扩范围"}],"recommended_option_id":"keep","recommendation_reason":"保持边界"}}`, true},
			} {
				var value any
				if err := json.Unmarshal([]byte(tc.body), &value); err != nil {
					t.Fatal(err)
				}
				schemaErr := schema.Validate(value)
				decision, decisionErr := maintenance.Accept([]byte(tc.body), mode)
				if valid := schemaErr == nil && decisionErr == nil; valid != tc.valid {
					t.Fatalf("%s: schema=%v decision=%v", tc.body, schemaErr, decisionErr)
				}
				if tc.valid && decision.Kind != "ask" && decision.Question != nil {
					t.Fatalf("optional null was retained as a question: %+v", decision)
				}
			}
		})
	}
}
