package suggestion

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steeragent/suggestion/contract"
)

func request() contract.Request {
	return contract.Request{Protocol: contract.Protocol, Capability: contract.Capability, RequestID: strings.Repeat("a", 32), Context: contract.Context{Goal: "Clarify the empty input requirement", Materials: []contract.Material{{ID: "spec", Label: "Explicit Spec", Body: "Empty input returns an error.", Source: "https://must-not-be-opened.invalid/spec"}}}}
}
func TestAdvisoryExplicitContextAndSinglePrompt(t *testing.T) {
	r := request()
	p, err := Prepare(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Context.Materials[0].Body = "changed after preparation"
	if !strings.Contains(p.Text, "Empty input returns an error.") || strings.Contains(p.Text, "changed after preparation") {
		t.Fatal("input did not freeze explicit values")
	}
	s, err := Accept(p, "请根据已提供的 Spec 继续讨论空输入行为，说明是否还需要用户决定。")
	if err != nil || s.RequestID != r.RequestID || s.Prompt == "" {
		t.Fatal(s, err)
	}
	for _, text := range []string{" ", strings.Repeat("字", contract.MaxPromptBytes), `{"answer":"approve","changes":[]}`, string([]byte{0xff})} {
		if _, err := Accept(p, text); err == nil {
			t.Fatal("invalid or legacy output accepted")
		}
	}
	p.Text += "unreviewed"
	if _, err := Accept(p, "Continue discussing"); err == nil {
		t.Fatal("changed input accepted")
	}
}
func TestAdvisoryRejectsUnsupportedAndAmbiguousInput(t *testing.T) {
	for _, change := range []func(*contract.Request){func(r *contract.Request) { r.Capability = "autonomous" }, func(r *contract.Request) { r.Protocol = "other" }, func(r *contract.Request) { r.Context.Materials = append(r.Context.Materials, r.Context.Materials[0]) }, func(r *contract.Request) { r.Context.Goal = " " }, func(r *contract.Request) { r.Context.Materials[0].Body = string([]byte{0xff}) }, func(r *contract.Request) { r.Context.Materials[0].Body = strings.Repeat("x", contract.MaxContextBytes) }} {
		r := request()
		change(&r)
		if _, err := Prepare(r); err == nil {
			t.Fatal("unsupported input accepted")
		}
	}
	raw, _ := json.Marshal(request())
	raw = append(raw[:len(raw)-1], []byte(`,"tool":"shell"}`)...)
	if _, err := contract.Decode(raw); err == nil {
		t.Fatal("tool field accepted")
	}
	if _, err := contract.Decode([]byte{0xff}); err == nil {
		t.Fatal("invalid wire UTF-8 accepted")
	}
}
