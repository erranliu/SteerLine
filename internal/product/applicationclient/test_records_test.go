package applicationclient

import (
	"context"
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"strings"
	"testing"
)

func TestTRRegisterEncodedBudgetAndUnknownOutcome(t *testing.T) {
	p := &Project{}
	_, err := p.RegisterTestRecord(context.Background(), api.TestRecordRegistration{Report: json.RawMessage(`{"id":"` + strings.Repeat("a", 32) + `"}`), Summary: make([]byte, 1<<20)}, strings.Repeat("b", 32))
	if err == nil || RecordRegistrationStatus(err) != "rejected" {
		t.Fatal("encoded budget allowed transport", err)
	}
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&recordRegistrationError{"outcome_unknown", "unknown"}, "unknown"},
		{nil, "registered"}, {errors.New("connection lost after submission"), "unknown"},
		{&httpclient.ClientError{Status: 409, Code: "identity_conflict"}, "rejected"},
		{&httpclient.ClientError{Status: 503, Code: "unavailable"}, "unknown"},
	} {
		if got := RecordRegistrationStatus(tc.err); got != tc.want {
			t.Fatal(tc.err, got, tc.want)
		}
	}
}
