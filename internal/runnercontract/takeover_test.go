package runnercontract

import (
	"encoding/json"
	"testing"
)

func TestTakeoverWireIdentity(t *testing.T) {
	input := ServiceStartInput{PendingStops: []PendingStop{{RunID: "original-run", Request: Request{Protocol: "steerrunner-cli/v1", RequestID: "original-stop", Input: json.RawMessage(`{"expected_revision":7}`)}}}}
	body, err := json.Marshal(input)
	if err != nil || string(body) != `{"pending_stops":[{"run_id":"original-run","request":{"protocol":"steerrunner-cli/v1","request_id":"original-stop","input":{"expected_revision":7}}}]}` {
		t.Fatal(string(body), err)
	}
	if err = input.Validate(); err != nil {
		t.Fatal(err)
	}
	input.PendingStops = append(input.PendingStops, input.PendingStops[0])
	if input.Validate() == nil {
		t.Fatal("duplicate original stop admitted")
	}
}

func TestTakeoverRequestDigestPreservesOriginalEnvelope(t *testing.T) {
	r := Request{Protocol: "steerrunner-cli/v1", RequestID: "original", Input: json.RawMessage(`{"title":"original","n":1234567890123456789}`)}
	digest := RequestDigest("submit", "", r)
	r.Input = json.RawMessage(`{ "n":1234567890123456789, "title":"original" }`)
	if digest == "" || RequestDigest("submit", "", r) != digest {
		t.Fatal("canonical equivalent input changed digest")
	}
	r.Input = json.RawMessage(`{"title":"original","n":1234567890123456790}`)
	if RequestDigest("submit", "", r) == digest {
		t.Fatal("JSON number precision lost")
	}
	r.Input = json.RawMessage(`{"title":"original","n":1234567890123456789}`)
	if RequestDigest("resume", "", r) == digest || RequestDigest("submit", "other", r) == digest {
		t.Fatal("command/target identity lost")
	}
}
