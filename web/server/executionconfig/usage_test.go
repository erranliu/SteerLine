package executionconfig

import (
	"context"
	"encoding/json"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"strings"
	"testing"
	"time"
)

func TestUsagePolicyRequiresObservedExactHostAndLeaf(t *testing.T) {
	host := "sha256:" + strings.Repeat("a", 64)
	root := "C:/Users/fixture/.codex/sessions/2026/11/05"
	raw := json.RawMessage(`{"enabled":true,"remote_log_access":{"mode":"temporary","grants":[{"environment":"env","host_id":"` + host + `","session_roots":["` + root + `"]}]}}`)
	p, err := cfg.DecodeUsagePolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	view := cfg.UsageCollection{Status: "ready", InstanceID: "i", PolicyRevision: "i", Requirements: []cfg.UsageRequirement{{SourceRef: "source", Environment: "env", HostID: host, SessionRoot: root, ObservedAt: time.Now().UTC()}}}
	if err = VerifyUsageRequirements(p, view); err != nil {
		t.Fatal(err)
	}
	view.Requirements[0].HostID = "sha256:" + strings.Repeat("b", 64)
	if VerifyUsageRequirements(p, view) == nil {
		t.Fatal("different host accepted")
	}
	p.RemoteLogAccess.Grants[0].SessionRoots = []string{"C:/Users/fixture/.codex"}
	bad, _ := json.Marshal(p)
	if _, err = cfg.DecodeUsagePolicy(bad); err == nil {
		t.Fatal("credential root accepted")
	}
	s, c := fixture(t)
	base, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.UsagePolicy = raw
	if _, err = s.Save(context.Background(), base.SavedDigest, encode(t, c)); err == nil {
		t.Fatal("unverified remote grants saved")
	}
	s.VerifyUsagePolicy = func(_ context.Context, p cfg.UsagePolicy) error {
		view.Requirements[0].HostID = host
		return VerifyUsageRequirements(p, view)
	}
	if _, err = s.Save(context.Background(), base.SavedDigest, encode(t, c)); err != nil {
		t.Fatal(err)
	}
}
