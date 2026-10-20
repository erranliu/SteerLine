package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
)

// Online deployment preflight shares the runtime's public protocol validation.
// It does not acquire project resources, start/stop a service or update a pin.
func runRunnerTakeover(args []string) error {
	flags := flag.NewFlagSet("verify-runner-takeover", flag.ContinueOnError)
	executable := flags.String("runner-exe", "", "absolute staged Runner executable")
	stateDir := flags.String("state-dir", "", "original Runner state directory")
	sha := flags.String("runner-sha256", "", "expected staged executable SHA256")
	flags.Bool("json", false, "print the verified public takeover report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*executable) || !filepath.IsAbs(*stateDir) || *sha == "" {
		return errors.New("takeover preflight requires absolute Runner/state paths and the staged SHA256")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cli := runnerclient.Client{Executable: *executable, StateDir: *stateDir, SHA256: *sha, Timeout: 30 * time.Second}
	report, err := cli.VerifyTakeover(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}
