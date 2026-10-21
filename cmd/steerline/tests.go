package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/erranliu/SteerLine/internal/app"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func runTestsCommand(args []string) error {
	if len(args) > 0 && args[0] == "register" {
		return runTestRegister(args[1:])
	}
	if len(args) == 0 {
		return errors.New("usage: steerline tests <generate|generation|apply|stop-generation> -project ID -request-id ID")
	}
	fs := flag.NewFlagSet("tests "+args[0], flag.ContinueOnError)
	project := fs.String("project", "", "configured project")
	request := fs.String("request-id", "", "original request ID (32 lowercase hex)")
	plan := fs.String("plan", "", "repository-relative .plan.md")
	profile := fs.String("execution-profile", "", "local Codex profile")
	revision := fs.Uint64("expected-revision", 0, "reviewed generation revision")
	review := fs.String("review-digest", "", "reviewed file and workspace digest")
	empty := fs.Bool("allow-empty", false, "explicitly approve an empty arrangement explained by the Plan")
	generation := fs.String("generation-id", "", "original generation ID")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected tests arguments")
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(*request) {
		return errors.New("-request-id must be 32 lowercase hex characters")
	}
	if args[0] != "apply" {
		applyFlag := false
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "expected-revision" || f.Name == "review-digest" || f.Name == "allow-empty" {
				applyFlag = true
			}
		})
		if applyFlag {
			return errors.New("apply flags require tests apply")
		}
	}
	switch args[0] {
	case "generate":
		if *plan == "" || *profile == "" || *generation != "" {
			return errors.New("generate requires -plan and -execution-profile")
		}
	case "generation":
		if *plan != "" || *profile != "" || *generation != "" {
			return errors.New("generation query accepts only -project and -request-id")
		}
	case "apply":
		if *generation == "" || *revision == 0 || len(*review) != 64 || *plan != "" || *profile != "" {
			return errors.New("apply requires -generation-id, -expected-revision, -review-digest")
		}
	case "stop-generation":
		if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(*generation) || *plan != "" || *profile != "" {
			return errors.New("stop-generation requires -generation-id")
		}
	default:
		return errors.New("unknown tests operation")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	p, err := app.OpenApplicationProject(ctx, *project, "")
	if err != nil {
		return err
	}
	defer p.Close()
	var result api.TestGeneration
	switch args[0] {
	case "generate":
		result, err = p.GenerateTests(ctx, api.TestGenerateRequest{RequestID: *request, Plan: *plan, Profile: *profile})
	case "generation":
		result, err = p.TestGeneration(ctx, *request)
	case "apply":
		result, err = p.ApplyTests(ctx, api.TestApplyRequest{RequestID: *request, GenerationID: *generation, ExpectedRevision: *revision, ReviewDigest: *review, AllowEmpty: *empty})
	case "stop-generation":
		result, err = p.StopTestGeneration(ctx, api.TestStopGenerationRequest{RequestID: *request, GenerationID: *generation})
	default:
		return errors.New("unknown tests operation")
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
