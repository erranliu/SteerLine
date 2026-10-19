package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	app "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/localservice"
)

func runApplication(command string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	noOpen := flags.Bool("no-open", false, "start services without opening a browser")
	detached := flags.Bool("detached", false, "verify resident process lifetime before opening resources")
	jsonOutput := flags.Bool("json", false, "print verified application status as JSON")
	runnerExe := flags.String("runner-exe", "", "verify-stop/verify-restart: absolute selected Runner executable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected application arguments")
	}
	verification := command == "verify-stop" || command == "verify-restart"
	if *noOpen && command != "start" && command != "restart" || *jsonOutput && command != "status" && !verification || *detached && command != "serve" || *runnerExe != "" && !verification {
		return errors.New("--no-open is for start/restart; --json is for status/verify-stop/verify-restart; --detached is for serve; --runner-exe is for verification")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch command {
	case "verify-restart":
		result, err := app.VerifyApplicationRestartable(ctx, *runnerExe)
		if err != nil {
			return err
		}
		body, _ := json.Marshal(result)
		fmt.Println(string(body))
		return nil
	case "verify-stop":
		result, err := app.VerifyApplicationStopped(ctx, *runnerExe)
		if err != nil {
			return err
		}
		body, _ := json.Marshal(result)
		fmt.Println(string(body))
		return nil
	case "serve":
		if *detached {
			if err := localservice.VerifyDetachedLifetime(); err != nil {
				return err
			}
		}
		return app.ServeApplication(ctx)
	case "stop":
		return app.StopAll(ctx)
	case "start", "restart", "status":
		var status app.ApplicationStatus
		var err error
		if command == "start" {
			status, err = app.StartApplication(ctx, *noOpen)
		} else if command == "restart" {
			status, err = app.RestartApplication(ctx, *noOpen)
		} else {
			status, err = app.StatusApplication(ctx)
		}
		if *jsonOutput {
			body, _ := json.Marshal(status)
			fmt.Println(string(body))
		} else {
			fmt.Printf("Backend: %s; Web: %s; degraded=%t\n", status.Backend.State, status.Web.State, status.Degraded)
			for name, component := range map[string]app.ComponentStatus{"backend": status.Backend, "web": status.Web} {
				if component.Identity != nil {
					fmt.Printf("%s pid=%d address=http://%s\n", name, component.Identity.Process.PID, component.Identity.Address)
				}
				fmt.Printf("%s log=%s\n", name, component.LogPath)
				if component.Diagnostic != "" {
					fmt.Println(component.Diagnostic)
				}
			}
			counts := map[string]int{}
			for _, project := range status.Projects {
				counts[project.RuntimeState]++
			}
			fmt.Printf("Projects: ready=%d starting=%d failed=%d\n", counts["ready"], counts["starting"], counts["failed"])
		}
		return err
	}
	return errors.New("unsupported application command")
}
