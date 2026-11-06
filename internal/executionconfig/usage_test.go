package executionconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUsagePolicyRequiresExplicitNarrowGrants(t *testing.T) {
	defaults, err := DecodeUsagePolicy(nil)
	if err != nil || !defaults.Enabled || defaults.RemoteLogAccess.Mode != "disabled" || len(defaults.RemoteLogAccess.Grants) != 0 {
		t.Fatal(defaults, err)
	}
	host := "sha256:" + strings.Repeat("a", 64)
	leaf := `C:\Users\remote\home\sessions\2026\11\05`
	makePolicy := func(root string) []byte {
		body, _ := json.Marshal(map[string]any{"enabled": true, "remote_log_access": map[string]any{"mode": "temporary", "grants": []any{map[string]any{"environment": "paired", "host_id": host, "session_roots": []string{root}}}}})
		return body
	}
	if _, err := DecodeUsagePolicy(makePolicy(leaf)); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{`C:\Users`, `\\server\sessions\2026\11\05`, `C:\home\..\sessions\2026\11\05`, `C:\*\sessions\2026\11\05`, `C:\home\sessions\2026\11`} {
		if _, err := DecodeUsagePolicy(makePolicy(root)); err == nil {
			t.Fatal("unsafe root accepted", root)
		}
	}
	for _, raw := range []string{`null`, `{}`, `{"enabled":null,"remote_log_access":{"mode":"disabled","grants":[]}}`, `{"enabled":true,"remote_log_access":{"mode":"disabled","grants":null}}`, `{"enabled":true,"remote_log_access":null}`, `{"enabled":true,"enabled":false,"remote_log_access":{"mode":"disabled","grants":[]}}`, strings.Replace(string(makePolicy(leaf)), host, "unverified", 1), strings.Replace(string(makePolicy(leaf)), `"temporary"`, `"disabled"`, 1)} {
		if _, err := DecodeUsagePolicy([]byte(raw)); err == nil {
			t.Fatal("invalid policy accepted", raw)
		}
	}
}
