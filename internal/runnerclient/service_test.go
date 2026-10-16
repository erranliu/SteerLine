package runnerclient

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceSettlementRequiresOriginalFinalProof(t *testing.T) {
	c := Client{StateDir: filepath.Join(t.TempDir(), "runner"), SHA256: strings.Repeat("a", 64)}
	for _, tc := range []struct {
		name   string
		change func(*ServiceInfo)
		pass   bool
	}{
		{"final", func(*ServiceInfo) {}, true},
		{"missing", func(i *ServiceInfo) { i.LastShutdown = nil }, false},
		{"replacement", func(i *ServiceInfo) { i.LastShutdown.InstanceID = "replacement" }, false},
		{"wrong_request", func(i *ServiceInfo) { i.LastShutdown.RequestID = "other" }, false},
		{"wrong_directory", func(i *ServiceInfo) { i.LastShutdown.StateDir += "-other" }, false},
		{"wrong_executable", func(i *ServiceInfo) { i.LastShutdown.ExecutableSHA256 = strings.Repeat("b", 64) }, false},
		{"still_running", func(i *ServiceInfo) { i.Status = "ready" }, false},
		{"native_unknown", func(i *ServiceInfo) { i.LastShutdown.ResourcesSettled = false }, false},
		{"contradictory", func(i *ServiceInfo) { i.LastShutdown.Diagnostic = "unverified" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := ServiceInfo{Status: "not_running", LastShutdown: &ServiceShutdown{InstanceID: "original", RequestID: "stop-original", StateDir: c.StateDir, ExecutableSHA256: c.SHA256, ResourcesSettled: true}}
			tc.change(&i)
			if err := c.verifyServiceSettlement(i, "original", "stop-original"); (err == nil) != tc.pass {
				t.Fatal(err)
			}
		})
	}
}
