// Package suggestion prepares explicit inputs and validates a single prompt suggestion.
// Repository access, model execution, persistence and product actions belong to its host.
package suggestion

import (
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/steeragent/suggestion/contract"
)

type Prepared struct {
	RequestID string
	Text      string
	Digest    string
}

func Prepare(r contract.Request) (Prepared, error) {
	if err := contract.Validate(r); err != nil {
		return Prepared{}, err
	}
	body, err := json.Marshal(r)
	if err != nil {
		return Prepared{}, err
	}
	text, err := prompts.MainAgentAdvisory(string(body))
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{RequestID: r.RequestID, Text: text, Digest: contract.Digest([]byte(text))}, nil
}
func Accept(p Prepared, text string) (contract.Suggestion, error) {
	s := contract.Suggestion{Protocol: contract.Protocol, RequestID: p.RequestID, Prompt: text}
	if p.Text == "" || p.Digest != contract.Digest([]byte(p.Text)) {
		return s, errors.New("prepared advisory input differs")
	}
	var old map[string]json.RawMessage
	if json.Unmarshal([]byte(text), &old) == nil && old != nil {
		return s, errors.New("advisory output must be prompt text, not a result/action object")
	}
	return s, contract.ValidateSuggestion(s)
}
