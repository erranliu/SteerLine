package contracts

import (
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestContractDecoderPreservesStrictFieldsAndSingleValue(t *testing.T) {
	for _, body := range []string{`{"executable":"git","unknown":true}`, `{"executable":"git"}{}`, `{"executable":"git"} null`} {
		var command executionbackend.Command
		if err := DecodeJSON([]byte(body), &command); err == nil {
			t.Fatalf("unsupported contract JSON was accepted: %s", body)
		}
	}
	var command executionbackend.Command
	if err := DecodeJSON(append([]byte{0xef, 0xbb, 0xbf}, []byte(`{"executable":"git","args":["status"]}`)...), &command); err != nil || command.Executable != "git" || len(command.Args) != 1 || command.Args[0] != "status" {
		t.Fatalf("current BOM-prefixed contract did not decode: %+v %v", command, err)
	}
}
