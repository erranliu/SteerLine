package integration

import (
	"context"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	terminal "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"strings"
	"testing"
)

func TestPublicationAndPermissionChecksRemainCombined(t *testing.T) {
	c := nativeRequirementConfig(t)
	c.Host = "orca"
	c.Agent = "opaque-agent"
	setNativeImplementationProfiles(t, &c, codexworker.ProfileConfig{}, &terminal.Profile{Executable: "missing-orca", Environment: "peer", RepositorySelector: "id:repo", WaitTimeout: "5s"})
	entry, err := c.ExecutionDirectory.Resolve(c.ExecutionConfig())
	require(t, err)
	require(t, entry.ValidatePublicationPolicy(c.PublicationPolicy, publication.Config(c.PR)))
	if reason := fixtureBoundProfile(c).StaticUnavailableReason(); reason != "" {
		t.Fatal(reason)
	}
	if err := requireNativeFlowCapabilities(context.Background(), c); err == nil || !strings.Contains(err.Error(), "Orca") {
		t.Fatalf("missing host preflight: %v", err)
	}
}
