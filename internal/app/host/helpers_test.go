package host

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--desktop-open-fixture" {
		desktopOpenFixture()
	}
	for i, arg := range os.Args {
		if arg != "--fake" {
			continue
		}
		args := os.Args[i+1:]
		if len(args) == 0 {
			os.Exit(2)
		}
		switch args[0] {
		case "sleep":
			time.Sleep(time.Hour)
			os.Exit(0)
		case "spawn-owned-child":
			if len(args) != 2 {
				os.Exit(2)
			}
			child := exec.Command(os.Args[0], "--fake", "sleep")
			if child.Start() != nil {
				os.Exit(1)
			}
			if os.WriteFile(args[1], []byte(fmt.Sprint(child.Process.Pid)), 0600) != nil {
				child.Process.Kill()
				os.Exit(1)
			}
			time.Sleep(time.Hour)
			os.Exit(0)
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}
