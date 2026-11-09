package executionconfig

import "testing"

func TestUsagePolicyDefaultsToCollectionWithoutDirectoryGrants(t *testing.T) {
	for _, raw := range []string{"", `{"enabled":true}`, `{"enabled":false}`} {
		p, err := DecodeUsagePolicy([]byte(raw))
		if err != nil || p.Enabled != (raw != `{"enabled":false}`) {
			t.Fatal(raw, p, err)
		}
	}
	for _, raw := range []string{`null`, `{}`, `{"enabled":null}`, `{"enabled":"true"}`, `{"enabled":true,"enabled":false}`, `{"enabled":true,"remote_log_access":{"mode":"temporary","grants":[]}}`} {
		if _, err := DecodeUsagePolicy([]byte(raw)); err == nil {
			t.Fatal("invalid policy accepted", raw)
		}
	}
}
