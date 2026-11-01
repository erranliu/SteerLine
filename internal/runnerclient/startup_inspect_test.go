package runnerclient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentInspectBeforeStartUsesOriginalPublicReader(t *testing.T) {
	c := takeoverCLI(t)
	c.BuildVersion, c.ResourceDigest, c.InstanceID = "fixed", "resources", "old-instance"
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "startup-inspect")
	call := filepath.Join(t.TempDir(), "call")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", call)
	t.Setenv("STEERLINE_STARTUP_INSPECTION", `{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":{"run":{"run_id":"original","revision":19,"input_digest":"input"},"command":null},"error":null}`)
	for _, scenario := range []string{"cold", "warm", "foreign-build", "startup-unverified", "foreign-directory"} {
		t.Run(scenario, func(t *testing.T) {
			info := ServiceInfo{Protocol: Protocol, StateDir: c.StateDir, Status: "ready", Version: c.BuildVersion, ResourceDigest: c.ResourceDigest, ExecutableSHA256: c.SHA256, InstanceID: "resident"}
			switch scenario {
			case "cold":
				info.Status, info.InstanceID = "not_running", ""
			case "foreign-build":
				info.ExecutableSHA256 = strings.Repeat("0", 64)
			case "startup-unverified":
				info.Status = "startup_unverified"
			case "foreign-directory":
				info.StateDir = t.TempDir()
			}
			raw, _ := json.Marshal(info)
			t.Setenv("STEERLINE_STARTUP_STATUS", `{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":`+string(raw)+`,"error":null}`)
			v, cold, err := c.InspectBeforeStart(context.Background(), "run-id", "original")
			args, _ := os.ReadFile(call)
			if strings.HasPrefix(scenario, "foreign") || scenario == "startup-unverified" {
				if err == nil || !strings.HasPrefix(string(args), "service\nstatus\n") {
					t.Fatal("unverified service queried", err, string(args))
				}
				return
			}
			if err != nil || v.Run == nil || v.Run.Revision != 19 || cold != (scenario == "cold") {
				t.Fatal(v, cold, err)
			}
			if !strings.HasPrefix(string(args), "inspect\n") || !strings.Contains(string(args), "--run-id\noriginal\n") || strings.Contains(string(args), "--refresh-pr") || strings.Contains(string(args), "service\nstart") || strings.Contains(string(args), "old-instance") {
				t.Fatal(string(args))
			}
			if cold != strings.Contains(string(args), "--offline\n") || !cold && !strings.Contains(string(args), "--expected-instance-id\nresident\n") || cold && strings.Contains(string(args), "--expected-instance-id") {
				t.Fatal("incorrect cold/resident binding", string(args))
			}
		})
	}
}
