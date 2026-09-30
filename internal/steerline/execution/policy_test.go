package execution

import (
	"encoding/json"
	"testing"
)

func TestExecutionPolicyRequiresExplicitAuthority(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"filesystem":{"read_scope":"host","write_scope":"workspace"},"network":"allow","escalation":"deny"}`,
		`{"filesystem":{"read_scope":"host","write_scope":"workspace","git_metadata_write":null},"network":"allow","escalation":"deny"}`,
		`{"filesystem":{"read_scope":"host","write_scope":"host","git_metadata_write":true},"network":"allow","escalation":"deny"}`,
		`{"filesystem":{"read_scope":"workspace","write_scope":"none","git_metadata_write":false},"network":"deny","escalation":"deny"}`,
		`{"filesystem":{"read_scope":"host","write_scope":"workspace","git_metadata_write":true},"network":"inherit","escalation":"allow"}`,
	} {
		var p ExecutionPolicy
		err := json.Unmarshal([]byte(raw), &p)
		if err == nil {
			err = p.Validate()
		}
		if err == nil {
			t.Fatalf("implicit or invalid authority accepted: %s", raw)
		}
	}
	var p ExecutionPolicy
	if err := json.Unmarshal([]byte(`{"filesystem":{"read_scope":"host","write_scope":"none","git_metadata_write":false},"network":"deny","escalation":"deny"}`), &p); err != nil || p.Validate() != nil {
		t.Fatalf("explicit false was lost: %+v/%v", p, err)
	}
}
