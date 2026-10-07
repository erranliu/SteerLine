package app

import (
	"strings"
	"testing"
)

func TestPS3RemovedPRCloseRejectedIncludingEmpty(t *testing.T) {
	for _, value := range []string{`null`, `{}`, `{"executable":"gh"}`} {
		var input ProjectConfigInput
		err := decodeStrictJSON([]byte(`{"pr":{"close":`+value+`}}`), &input)
		if err == nil || !strings.Contains(err.Error(), `unknown field "close"`) {
			t.Fatalf("removed close accepted: %s: %v", value, err)
		}
	}
}
