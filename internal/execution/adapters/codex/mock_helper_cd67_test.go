package codexworker_test

import (
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
	"os"
	"strings"
	"testing"
)

func TestWorkerFlowMockCodexHelper(t *testing.T) {
	args := strings.Join(os.Args, " ")
	if os.Getenv("STEERLINE_WORKER_FLOW_MOCK") != "1" {
		return
	}
	if strings.Contains(args, " --version") {
		fmt.Fprintln(os.Stdout, "codex-cli mock-1")
		os.Exit(0)
	}
	if strings.Contains(args, " app-server ") && strings.Contains(args, " --help") {
		fmt.Fprintln(os.Stdout, "Codex app-server --listen ADDRESS")
		os.Exit(0)
	}
	if strings.Contains(args, " app-server ") && strings.Contains(args, " --stdio") {
		testprotocol.MockAppServerStdio()
		os.Exit(0)
	}
	if strings.Contains(args, " app-server ") && strings.Contains(args, " --listen ") {
		testprotocol.MockAppServerMain()
		os.Exit(0)
	}
	os.Exit(2)
}
