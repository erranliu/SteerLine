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
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected application arguments")
	}
	if *noOpen && command != "start" || *jsonOutput && command != "status" {
		return errors.New("--no-open is for start; --json is for status")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch command {
	case "serve":
		if *detached {
			if err := localservice.VerifyDetachedLifetime(); err != nil {
				return err
			}
		}
		return app.ServeApplication(ctx)
	case "stop":
		return app.StopAll(ctx)
	case "start", "status":
		var status app.ApplicationStatus
		var err error
		if command == "start" {
			status, err = app.StartApplication(ctx, *noOpen)
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
