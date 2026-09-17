package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "steerline:", e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: steerline <run|check|stop|accept-result|mark-stopped> -config config.json")
	}
	command := os.Args[1]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	config := fs.String("config", "steerline.local.json", "configuration JSON")
	runID := fs.String("run-id", "", "manual recovery run ID")
	result := fs.String("result", "", "manual recovery result file")
	confirmed := fs.Bool("confirm-process-stopped", false, "operator verified that the old process tree is stopped")
	if e := fs.Parse(os.Args[2:]); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if command != "run" && command != "check" && command != "stop" && command != "accept-result" && command != "mark-stopped" {
		return fmt.Errorf("unknown command %q", command)
	}
	c, e := steerline.LoadConfig(*config)
	if e != nil {
		return e
	}
	if command == "stop" {
		return steerline.RequestStop(c)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e = c.Preflight(ctx, command == "run" || command == "check"); e != nil {
		return e
	}
	if command == "check" {
		fmt.Println("Configuration, tools, Beads, PR provider and skill contracts verified; no task claimed.")
		return nil
	}
	lock, e := steerline.LockRepository(c.Identity())
	if e != nil {
		return e
	}
	defer lock.Close()
	runLock, e := steerline.LockRepository(c.RunDir)
	if e != nil {
		return e
	}
	defer runLock.Close()
	s, e := steerline.NewService(c)
	if e != nil {
		return e
	}
	if command == "mark-stopped" {
		if !*confirmed || *runID == "" {
			return fmt.Errorf("mark-stopped requires -run-id and -confirm-process-stopped")
		}
		return s.MarkStopped(*runID)
	}
	if command == "accept-result" {
		if !*confirmed || *runID == "" || *result == "" {
			return fmt.Errorf("accept-result requires -run-id, -result and -confirm-process-stopped")
		}
		return s.AcceptResult(ctx, *runID, *result)
	}
	fmt.Printf("Steerline M0 running: %s -> %s/%s; Ctrl+C or steerline stop to exit\n", c.Repository, c.Remote, c.Branch)
	return s.Run(ctx)
}
