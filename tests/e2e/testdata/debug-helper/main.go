package main

import (
	"fmt"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"os"
)

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "sandbox-probe" {
		err = driver.NativeSandboxProbe(os.Args[2:])
	} else if len(os.Args) > 1 && os.Args[1] == "runner-cleanup" {
		err = driver.MainRunnerCleanup(os.Args[2:])
	} else if len(os.Args) > 1 && os.Args[1] == "debug" {
		err = driver.DebugReleaseMain(os.Args[2:])
	} else {
		err = controlled.Main(os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
