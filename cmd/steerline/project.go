package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/roadmap"
)

func runProjectCommand(args []string) error {
	if len(args) == 0 || args[0] != "prepare-roadmap" {
		return errors.New("usage: steerline project prepare-roadmap -repository <absolute-path> [-check]")
	}
	flags := flag.NewFlagSet("project prepare-roadmap", flag.ContinueOnError)
	repository := flags.String("repository", "", "existing Git project root")
	check := flags.Bool("check", false, "verify without writing")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*repository) {
		return errors.New("project preparation requires an absolute repository root")
	}
	root, err := exec.Command("git", "-C", *repository, "rev-parse", "--show-toplevel").Output()
	if err != nil || !strings.EqualFold(filepath.Clean(strings.TrimSpace(string(root))), filepath.Clean(*repository)) {
		return errors.New("project preparation requires the root of an existing Git worktree")
	}
	result, err := roadmap.PrepareStatic(*repository, *check)
	if err != nil {
		return err
	}
	body, err := json.Marshal(result)
	if err == nil {
		fmt.Println(string(body))
	}
	return err
}
