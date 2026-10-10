package integration

import (
	"context"
	"strings"
	"testing"
)

func TestPS2FixedConfigurationKeepsMandatoryChecks(t *testing.T) {
	c, _, _ := nativeAdmissionFixture(t)
	codex := nativeProfileSettings(c)
	codex.Executable = "must-not-be-called"
	setNativeProfile(t, &c, codex)
	c.PublicationPolicy.Publisher = "host_service"
	if err := requireNativeFlowCapabilities(context.Background(), c); err == nil || !strings.Contains(err.Error(), "publication_policy") {
		t.Fatalf("empty requirements bypassed publisher check: %v", err)
	}
	c.PublicationPolicy = executionPublicationFixture()
	if err := requireNativeFlowCapabilities(context.Background(), c); err == nil {
		t.Fatal("empty requirements bypassed mandatory CLI preflight")
	}
}
