package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
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
		return fmt.Errorf("usage: steerline <run|retry|check|stop|watch|answer|accept-result|mark-stopped|home|install|launchers|projects> -config config.json")
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
	proj := fs.String("project", "", "projects: project name or number")
	action := fs.String("action", "", "projects: watch|run|stop|home|list")
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
	if command != "run" && command != "retry" && command != "check" && command != "stop" && command != "answer" && command != "accept-result" && command != "mark-stopped" && command != "watch" && command != "home" && command != "install" && command != "launchers" && command != "projects" {
		return fmt.Errorf("unknown command %q", command)
	}
	if command == "retry" && (!*confirmed || *runID == "") {
		return fmt.Errorf("retry requires -run-id and -confirm-process-stopped")
	}
	if command == "install" {
		dst, e := steerline.InstallSelf()
		if e != nil {
			return e
		}
		fmt.Println("installed", dst)
		return nil
	}
	if command == "launchers" {
		written, e := steerline.InstallLaunchers()
		if e != nil {
			return e
		}
		for _, p := range written {
			fmt.Println("wrote", p)
		}
		return nil
	}
	if command == "home" {
		printHome(*config)
		return nil
	}
	if command == "projects" {
		return runProjects(*proj, *action)
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

// runProjects picks a project under <STEERLINE_HOME>\etc and re-execs this
// binary with the chosen action, so run/watch reuse their normal code paths
// while several projects can be launched side by side.
func runProjects(project, action string) error {
	projects, e := steerline.ListProjects()
	if e != nil {
		return e
	}
	if len(projects) == 0 {
		return fmt.Errorf("no project configs under %s", steerline.SteerlineEtcDir())
	}
	chosen, e := pickProject(projects, project)
	if e != nil {
		return e
	}
	if chosen.Err != nil {
		return fmt.Errorf("config %s is invalid: %w", chosen.ConfigPath, chosen.Err)
	}
	act, e := pickAction(action)
	if e != nil {
		return e
	}
	if act == "list" {
		printProjects(projects)
		return nil
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	args := []string{act, "-config", chosen.ConfigPath}
	if act == "run" {
		args = append(args, "-console", "pretty")
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func pickProject(projects []steerline.ProjectInfo, sel string) (steerline.ProjectInfo, error) {
	if sel != "" {
		return steerline.ResolveProject(projects, sel)
	}
	if len(projects) == 1 {
		return projects[0], nil
	}
	printProjects(projects)
	if !stdinIsTTY() {
		return steerline.ProjectInfo{}, fmt.Errorf("multiple projects; pass -project <name|number>")
	}
	line, e := readLine("project (number or name): ")
	if e != nil {
		return steerline.ProjectInfo{}, e
	}
	return steerline.ResolveProject(projects, line)
}

func pickAction(action string) (string, error) {
	switch action {
	case "watch", "run", "stop", "home", "list":
		return action, nil
	case "":
	default:
		return "", fmt.Errorf("unknown action %q (watch|run|stop|home|list)", action)
	}
	if !stdinIsTTY() {
		return "watch", nil
	}
	fmt.Println("Actions:  1) viewer   2) run service   3) stop service   4) home   5) list")
	line, e := readLine("action (1-5): ")
	if e != nil {
		return "", e
	}
	switch strings.TrimSpace(line) {
	case "1", "viewer", "watch":
		return "watch", nil
	case "2", "run":
		return "run", nil
	case "3", "stop":
		return "stop", nil
	case "4", "home":
		return "home", nil
	case "5", "list":
		return "list", nil
	}
	return "", fmt.Errorf("invalid action %q", line)
}

func printProjects(projects []steerline.ProjectInfo) {
	fmt.Println("Projects:")
	for i, p := range projects {
		state := p.RunDir
		if p.Err != nil {
			state = "(invalid: " + p.Err.Error() + ")"
		}
		fmt.Printf("  %d) %-24s %s\n", i+1, p.Name, state)
	}
}

func stdinIsTTY() bool {
	info, e := os.Stdin.Stat()
	return e == nil && info.Mode()&os.ModeCharDevice != 0
}

func readLine(prompt string) (string, error) {
	fmt.Print(prompt)
	line, e := bufio.NewReader(os.Stdin).ReadString('\n')
	if e != nil && line == "" {
		return "", e
	}
	return strings.TrimSpace(line), nil
}

// printHome shows where this machine keeps the shared SteerLine binary, configs
// and per-repo state, resolved independently of the current source worktree.
func printHome(configPath string) {
	fmt.Println("STEERLINE_HOME", steerline.SteerlineHome())
	fmt.Println("bin           ", steerline.SteerlineBinDir())
	fmt.Println("etc           ", steerline.SteerlineEtcDir())
	fmt.Println("state root    ", steerline.SteerlineStateRoot())
	c, e := steerline.LoadConfig(configPath)
	if e != nil {
		fmt.Println("config        ", "(not loaded:", e.Error()+")")
		return
	}
	abs, _ := filepath.Abs(configPath)
	fmt.Println("config        ", abs)
	fmt.Println("repository    ", c.Repository)
	fmt.Println("run_dir       ", c.RunDir)
	fmt.Println("worktree_root ", c.WorktreeRoot)
}
