package app

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/testkit/protocol"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if runUsageResponseFixture() {
		return
	}
	if record := os.Getenv("STEERLINE_TEST_OPENER_RECORD"); record != "" && len(os.Args) == 2 && os.Args[1] == "open" {
		cwd, _ := os.Getwd()
		raw, _ := json.Marshal(map[string]any{"executable": os.Args[0], "args": os.Args[1:], "home": os.Getenv("STEERLINE_HOME"), "cwd": cwd})
		if err := os.WriteFile(record, raw, 0600); err != nil {
			os.Exit(3)
		}
		if os.Getenv("STEERLINE_TEST_OPENER_FAIL") == "1" {
			os.Exit(2)
		}
		return
	}
	if testprotocol.RunRunnerServiceFixture() {
		return
	}
	os.Exit(m.Run())
}
