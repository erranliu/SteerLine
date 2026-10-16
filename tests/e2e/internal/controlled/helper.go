// Package controlled provides caller-owned Runner CLI and Git/PR fixtures.
package controlled

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func argument(args []string, key string) string {
	for i, arg := range args {
		if arg == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
func Main(args []string) error {
	if handled, err := RunnerCLI(args); handled {
		return err
	}
	if len(args) > 0 && args[0] == "git-server" {
		return gitServer(argument(args, "--root"), argument(args, "--address-file"), argument(args, "--e2e-state"))
	}
	path := argument(args, "--e2e-state")
	if path == "" {
		cwd, _ := os.Getwd()
		path = statePathForDirectory(cwd)
	}
	if len(args) > 0 && args[0] == "git" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cwd, _ := os.Getwd()
		out, err := gitBytes(ctx, cwd, args[1:]...)
		if err == nil {
			_, err = os.Stdout.Write(out)
		}
		return err
	}
	if len(args) > 0 && args[0] == "provider" {
		if err := recordProviderQuery(path); err != nil {
			return err
		}
		s, err := Read(path)
		if err != nil {
			return err
		}
		pr, err := ProviderPR(s, argument(args, "--pr-url"))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(pr)
	}
	if len(args) > 0 && args[0] == "release-fixture" {
		s, err := Read(path)
		if err != nil {
			return err
		}
		s.Hold = false
		return Write(path, s)
	}
	return fmt.Errorf("unsupported caller fixture operation")
}
