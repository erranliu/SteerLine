package boundary

import (
	"strings"
	"testing"
)

func TestNativeConfigurationOwnership(t *testing.T) {
	t.Run("frontend_is_opaque", func(t *testing.T) {
		root := t.TempDir()
		writeCallerSource(t, root, "web/src/features/execution-configs/editor.tsx", "const fields = draft.execution.settings.agent_launch;")
		report, err := ScanSource(root)
		if err != nil || !strings.Contains(strings.Join(report.Violations, "\n"), "native configuration mapping outside Runner") {
			t.Fatal(report.Violations, err)
		}
	})
	for _, path := range []string{"internal/app/new_request.go", "internal/environmentadaptation/model/identity_plan9.go", "internal/storage/sqlite/environment_current.go", "web/server/executionconfig/editor.go", "web/server/discovery/context.go"} {
		root := t.TempDir()
		writeCallerSource(t, root, path, "package fixture\ntype Native struct{ Launch string `json:\"agent_launch\"` }\n")
		report, err := ScanSource(root)
		if err != nil || !strings.Contains(strings.Join(report.Violations, "\n"), "native configuration mapping outside Runner") {
			t.Fatal(path, report.Violations, err)
		}
	}
	for _, path := range []string{"internal/execution/model/runner_profile.go", "internal/testkit/remotetest/assets.go"} {
		root := t.TempDir()
		writeCallerSource(t, root, path, "package fixture\ntype Frozen struct{ Launch string `json:\"agent_launch\"` }\n")
		report, err := ScanSource(root)
		if err != nil || strings.Contains(strings.Join(report.Violations, "\n"), "native configuration mapping outside Runner") {
			t.Fatal(path, report.Violations, err)
		}
	}
}
