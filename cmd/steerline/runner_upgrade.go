package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/app"
)

func runRunnerUpgrade(args []string) error {
	flags := flag.NewFlagSet("verify-runner-upgrade", flag.ContinueOnError)
	executable := flags.String("runner-exe", "", "absolute staged Runner executable")
	home := flags.String("home", "", "explicit installation home")
	sha := flags.String("runner-sha256", "", "expected staged executable SHA256")
	flags.Bool("json", false, "print the verified upgrade preflight")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*executable) || !filepath.IsAbs(*home) || *sha == "" {
		return errors.New("upgrade preflight requires absolute Runner/home paths and the staged SHA256")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	report, err := app.VerifyRunnerUpgrade(ctx, *home, *executable, *sha)
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
