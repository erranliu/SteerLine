package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/viewer"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "steerline:", e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: steerline <run|retry|check|stop|watch|answer|accept-result|mark-stopped> -config config.json")
	}
	command := os.Args[1]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	config := fs.String("config", "steerline.local.json", "configuration JSON")
	runID := fs.String("run-id", "", "manual recovery run ID, run to watch, or run to answer")
	result := fs.String("result", "", "manual recovery result file")
	confirmed := fs.Bool("confirm-process-stopped", false, "operator verified that the old process tree is stopped")
	option := fs.String("option", "", "answer: chosen decision option id")
	decisionID := fs.String("decision-id", "", "answer: pending decision id (defaults to the run's pending decision)")
	note := fs.String("note", "", "answer: optional human note")
	by := fs.String("by", "", "answer: who answered (defaults to the OS user)")
	console := fs.String("console", "", "service console rendering: pretty|summary|json|off (default pretty on a terminal)")
	follow := fs.Bool("follow", true, "watch: keep following new activity")
	fromStart := fs.Bool("from-start", true, "watch: replay existing activity before following")
	jsonOut := fs.Bool("json", false, "watch: emit stable activity JSONL")
	rawEvents := fs.Bool("raw", false, "watch: emit the raw agent event stream")
	noTUI := fs.Bool("no-tui", false, "watch: force streaming output even on a terminal")
	if e := fs.Parse(os.Args[2:]); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if command != "run" && command != "retry" && command != "check" && command != "stop" && command != "answer" && command != "accept-result" && command != "mark-stopped" && command != "watch" {
		return fmt.Errorf("unknown command %q", command)
	}
	if command == "retry" && (!*confirmed || *runID == "") {
		return fmt.Errorf("retry requires -run-id and -confirm-process-stopped")
	}
	c, e := steerline.LoadConfig(*config)
	if e != nil {
		return e
	}
	if command == "stop" {
		return steerline.RequestStop(c)
	}
	if command == "answer" {
		return steerline.AnswerDecision(c, steerline.AnswerOptions{RunID: *runID, DecisionID: *decisionID, Option: *option, Note: *note, By: *by})
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if command == "watch" {
		if !*jsonOut && !*rawEvents && !*noTUI && viewer.IsInteractive() {
			return viewer.Run(ctx, c, viewer.Options{RunID: *runID, Color: steerline.ColorEnabled()})
		}
		return steerline.Watch(ctx, c, *runID, steerline.WatchOptions{Follow: *follow, FromStart: *fromStart, JSON: *jsonOut, Raw: *rawEvents, Color: steerline.ColorEnabled()})
	}
	if e = c.Preflight(ctx, command == "run" || command == "retry" || command == "check"); e != nil {
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
	if *console != "" {
		s.Console = *console
	} else if steerline.ConsoleDefault() != "" {
		s.Console = steerline.ConsoleDefault()
	}
	if command == "retry" {
		fmt.Printf("Steerline retry-one: %s; no new tasks will be claimed\n", *runID)
		return s.RetryOne(ctx, *runID)
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
