package boundary

import (
	"strings"
	"testing"
)

func TestDeveloperConfigurationSourceBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, file, source string
		rejected           bool
	}{
		{"service_import", "internal/app/runner_service_wiring.go", `package app; import dev "` + module + `/internal/developerconfig"; var _ = dev.Available`, true},
		{"query_blank_import", "internal/app/environment_readiness.go", `package app; import _ "` + module + `/internal/developerconfig"`, true},
		{"query_dot_import", "web/server/discovery/query.go", `package discovery; import . "` + module + `/internal/developerconfig"; var _ = Available`, true},
		{"service_config_parameter", "internal/app/service.go", `package app; func check(c Config) bool { return c.RunnerExecution.Developer.SkipEnvironmentReadiness }`, true},
		{"query_raw_namespace", "internal/app/environment_readiness.go", `package app; func options(c Config) any { return c.RunnerExecution.Developer }`, true},
		{"helper_raw_alias", "internal/helper/config.go", `package helper; func check(raw Options) bool { return raw.SkipEnvironmentReadiness }`, true},
		{"developer_subpackage", "internal/helper/edge.go", `package helper; import _ "` + module + `/internal/developerconfig/options"`, true},
		{"inactive_query", "internal/app/query_plan9.go", "//go:build never_enabled\n\npackage app; func check(c Config) any { return c.RunnerExecution.Developer }", true},
		{"composition_name_in_other_package", "internal/helper/capabilities.go", `package helper; import _ "` + module + `/internal/developerconfig"`, true},
		{"new_app_file_is_not_composition", "internal/app/options.go", `package app; import _ "` + module + `/internal/developerconfig"`, true},
		{"definitions", "internal/developerconfig/options.go", `package developerconfig; func check(c Config) bool { return c.SkipEnvironmentReadiness }`, false},
		{"configuration_schema", "internal/executionconfig/config.go", `package executionconfig; import _ "` + module + `/internal/developerconfig"`, false},
		{"configuration_validation", "internal/app/runner_config.go", `package app; func validate(c Config) error { return c.Developer.Validate() }`, false},
		{"capability_mapping", "internal/app/capabilities.go", `package app; import dev "` + module + `/internal/developerconfig"; func resolve(c Config) bool { return dev.Available && c.RunnerExecution.Developer.SkipEnvironmentReadiness }`, false},
		{"configuration_editor", "web/server/executionconfig/store.go", `package executionconfig; import dev "` + module + `/internal/developerconfig"; func validate(c Config) error { _ = dev.Available; return c.Developer.Validate() }`, false},
		{"service_domain_policy", "internal/app/service.go", `package app; func check(c Config) bool { return c.environmentPolicy.RequireReadiness() }`, false},
		{"query_domain_policy", "internal/app/environment_readiness.go", `package app; func options(c Config) bool { return c.environmentPolicy.RequireReadiness() }`, false},
		{"test_fixture", "internal/app/service_test.go", `package app; import dev "` + module + `/internal/developerconfig"; func fixture(c Config) { _ = dev.Available; c.RunnerExecution.Developer.SkipEnvironmentReadiness = true }`, false},
		{"comment_and_string", "internal/app/service.go", "package app\n// c.Developer.SkipEnvironmentReadiness\nvar message = \"c.Developer.SkipEnvironmentReadiness\"", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeCallerSource(t, root, tc.file, tc.source)
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			found := strings.Contains(strings.Join(report.Violations, "\n"), "developer configuration boundary:")
			if found != tc.rejected {
				t.Fatalf("rejected=%v: %v", tc.rejected, report.Violations)
			}
		})
	}
}

func TestDeveloperConfigurationHelperBoundary(t *testing.T) {
	for _, source := range []string{
		`package helper; import dev "` + module + `/internal/developerconfig"; var Skip = dev.Available`,
		`package helper; func Skip(c Config) bool { return c.Developer.SkipEnvironmentReadiness }`,
	} {
		root := t.TempDir()
		writeCallerSource(t, root, "internal/app/service_linux.go", `package app; import _ "`+module+`/internal/helper"`)
		writeCallerSource(t, root, "internal/helper/options_plan9.go", "//go:build never_enabled\n\n"+source)
		report, err := ScanSource(root)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(report.Violations, "\n"), "developer configuration boundary: internal/helper/options_plan9.go") {
			t.Fatalf("helper hid raw developer configuration: %v", report.Violations)
		}
	}
}
