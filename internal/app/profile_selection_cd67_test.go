package app

import (
	"strings"
	"testing"
)

func TestUnsupportedLocalAgentProfileIsMarkedUnavailable(t *testing.T) {
	profile := ExecutionProfile{ID: "local-pi", Name: "Local Pi", Host: "local", Agent: "pi"}
	config := bindUnitDirectory(t, Config{ExecutionProfiles: []ExecutionProfile{profile}})
	profile = boundProfile(config, profile)
	if reason := profile.StaticUnavailableReason(); !strings.Contains(reason, "no implementation") {
		t.Fatalf("unsupported native agent was shown as selectable: %q", reason)
	}
}
