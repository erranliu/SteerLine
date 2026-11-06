package executionconfig

import (
	"errors"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"strings"
)

// Only an observed exact host/session leaf or an already effective grant may
// authorize a proposed root. Browser-entered paths alone are not proof.
func VerifyUsageRequirements(policy cfg.UsagePolicy, current cfg.UsageCollection) error {
	if current.Status != "ready" || current.InstanceID == "" || current.PolicyRevision == "" {
		return errors.New("effective usage source identity unavailable")
	}
	for _, g := range policy.RemoteLogAccess.Grants {
		for _, root := range g.SessionRoots {
			found := false
			for _, r := range current.Requirements {
				if r.Environment == g.Environment && r.HostID == g.HostID && r.SourceRef != "" && !r.ObservedAt.IsZero() && sameRoot(r.SessionRoot, root) {
					found = true
				}
			}
			for _, old := range current.Policy.RemoteLogAccess.Grants {
				if old.Environment == g.Environment && old.HostID == g.HostID {
					for _, r := range old.SessionRoots {
						found = found || sameRoot(r, root)
					}
				}
			}
			if !found {
				return errors.New("授权须匹配Runner已核实的主机与最小session叶目录")
			}
		}
	}
	return nil
}
func sameRoot(a, b string) bool {
	return strings.EqualFold(strings.ReplaceAll(a, "\\", "/"), strings.ReplaceAll(b, "\\", "/"))
}
