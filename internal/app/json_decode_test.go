package app

import (
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestProjectConfigDecoderPreservesStrictSingleJSONValueAndBOM(t *testing.T) {
	var command executionbackend.Command
	if err := decodeStrictJSON(append([]byte{0xef, 0xbb, 0xbf}, []byte(`{"executable":"git"}`)...), &command); err != nil {
		t.Fatalf("current BOM-prefixed Git command was rejected: %v", err)
	}
	if command.Executable != "git" {
		t.Fatalf("Git command decoded incorrectly: %+v", command)
	}
	for _, input := range []string{
		`{"executable":"git","unknown":true}`,
		`{"executable":"git"} {"executable":"git"}`,
		`{"executable":"git"} null`,
	} {
		if err := decodeStrictJSON([]byte(input), &command); err == nil {
			t.Errorf("strict decoder accepted %s", input)
		}
	}
}
