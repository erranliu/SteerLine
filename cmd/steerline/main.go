package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	consoleui "github.com/erranliu/SteerLine/internal/steerline/console"
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
		return fmt.Errorf("usage: steerline <console|run|start-service|stop-service|retry|check|control|tasks|task|stop|watch|answer|mark-stopped|certify-backend|home|install|launchers|projects> [-config config.json]")
	}
	if os.Args[1] == "task" {
		return runNativeTaskCommand(os.Args[2:])
	}
	if os.Args[1] == "console" {
		return runConsoleCommand(os.Args[2:])
	}
	command := os.Args[1]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	config := fs.String("config", "steerline.local.json", "configuration JSON")
	runID := fs.String("run-id", "", "manual recovery run ID, run to watch, or run to answer")
	dispatchMode := fs.String("dispatch", "auto", "run dispatch mode: auto|manual")
	startupID := fs.String("startup-id", "", "internal startup attempt ID")
	taskID := fs.String("task-id", "", "control start_task target")
	controlMode := fs.String("mode", "", "control set_dispatch_mode value: auto|manual")
	requestID := fs.String("request-id", "", "control request to query")
	waitReceipt := fs.Bool("wait", false, "control: wait for a terminal receipt")
	evidence := fs.String("evidence", "", "certify-backend: comma-separated Windows native evidence JSON files")
	confirmed := fs.Bool("confirm-process-stopped", false, "operator verified that the old process tree is stopped")
	option := fs.String("option", "", "answer: chosen decision option id")
	decisionID := fs.String("decision-id", "", "answer: pending decision id (defaults to the run's pending decision)")
	note := fs.String("note", "", "answer: optional human note")
	by := fs.String("by", "", "answer: who answered (defaults to the OS user)")
	reason := fs.String("reason", "", "recovery: explicit evidence or basis for the operator confirmation")
	proj := fs.String("project", "", "projects: project name or number")
	action := fs.String("action", "", "control action or projects: watch|run|stop|home|list")
	console := fs.String("console", "", "service console rendering: pretty|summary|json|off (default pretty on a terminal)")
	serviceWindow := fs.Bool("service-window", false, "run inside a required visible Windows service window")
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
	if command == "codex-session-launch" {
		return steerline.RunCodexSessionLauncher(context.Background(), *config)
	}
	if command != "run" && command != "start-service" && command != "stop-service" && command != "retry" && command != "check" && command != "stop" && command != "control" && command != "tasks" && command != "answer" && command != "mark-stopped" && command != "watch" && command != "certify-backend" && command != "home" && command != "install" && command != "launchers" && command != "projects" {
		return fmt.Errorf("unknown command %q", command)
	}
	if command == "run" && *dispatchMode != "manual" && *dispatchMode != "auto" {
		return fmt.Errorf("-dispatch must be manual or auto")
	}
	if *serviceWindow && command != "run" {
		return fmt.Errorf("-service-window is only valid with run")
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if command == "retry" || command == "mark-stopped" {
		if command == "mark-stopped" && (!*confirmed || *runID == "" || strings.TrimSpace(*reason) == "") {
			return fmt.Errorf("mark-stopped requires -run-id, -confirm-process-stopped and -reason")
		}
		return submitRecoveryCommand(ctx, c, command, *runID, *confirmed, *reason, *by, *waitReceipt)
	}
	if command == "certify-backend" {
		if strings.TrimSpace(*evidence) == "" {
			return fmt.Errorf("certify-backend requires -evidence")
		}
		paths := make([]string, 0)
		for _, raw := range strings.Split(*evidence, ",") {
			if value := strings.TrimSpace(raw); value != "" {
				paths = append(paths, value)
			}
		}
		report, err := steerline.CertifyExecutionBackend(ctx, c, paths)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	if command == "start-service" {
		result, err := steerline.StartService(ctx, *config, steerline.StartServiceOptions{})
		if err != nil {
			return err
		}
		b, _ := json.Marshal(result)
		fmt.Println(string(b))
		if result.Status == "failed" {
			return fmt.Errorf("service startup failed: %s", result.Error)
		}
		return nil
	}
	if command == "stop-service" {
		result, err := steerline.StopService(ctx, c, *by, 30*time.Second)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(result)
		fmt.Println(string(b))
		if result.Status != "stopped" {
			return fmt.Errorf("service stop is %s: %s", result.Status, result.Diagnostic)
		}
		return nil
	}
	if command == "stop" {
		request, err := steerline.SubmitControl(c, "stop_service", nil, *by)
		if err != nil {
			return err
		}
		fmt.Println("request_id", request.RequestID)
		if *waitReceipt {
			receipt, waitErr := steerline.WaitControlReceipt(ctx, c, request.RequestID, 100*time.Millisecond)
			if waitErr != nil {
				return waitErr
			}
			return json.NewEncoder(os.Stdout).Encode(receipt)
		}
		return nil
	}
	if command == "control" {
		if *requestID != "" {
			if *action != "" || *taskID != "" || *controlMode != "" {
				return fmt.Errorf("-request-id cannot be combined with a new action or action parameters")
			}
			var r *steerline.ControlReceipt
			var err error
			if *waitReceipt {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				r, err = steerline.WaitControlReceipt(ctx, c, *requestID, 100*time.Millisecond)
			} else {
				r, err = steerline.ReadVerifiedControlReceipt(c, *requestID)
			}
			if err != nil {
				return err
			}
			b, _ := json.Marshal(r)
			fmt.Println(string(b))
			return nil
		}
		if *action == "" {
			return fmt.Errorf("control requires -action or -request-id")
		}
		var payload any
		switch *action {
		case "start_task":
			if *taskID == "" {
				return fmt.Errorf("start_task requires -task-id")
			}
			if *controlMode != "" {
				return fmt.Errorf("start_task does not accept -mode")
			}
			payload = map[string]string{"task_id": *taskID}
		case "set_dispatch_mode":
			if *controlMode == "" {
				return fmt.Errorf("set_dispatch_mode requires -mode auto|manual")
			}
			if *controlMode != "auto" && *controlMode != "manual" {
				return fmt.Errorf("-mode must be auto or manual")
			}
			if *taskID != "" {
				return fmt.Errorf("set_dispatch_mode does not accept -task-id")
			}
			payload = map[string]string{"mode": *controlMode}
		case "pause_dispatch", "resume_dispatch", "stop_service":
			if *taskID != "" || *controlMode != "" {
				return fmt.Errorf("%s does not accept task or mode parameters", *action)
			}
		default:
			return fmt.Errorf("unsupported control action %q", *action)
		}
		req, err := steerline.SubmitControl(c, *action, payload, *by)
		if err != nil {
			return err
		}
		fmt.Println("request_id", req.RequestID)
		if *waitReceipt {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			r, err := steerline.WaitControlReceipt(ctx, c, req.RequestID, 100*time.Millisecond)
			if err != nil {
				return err
			}
			b, _ := json.Marshal(r)
			fmt.Println(string(b))
		}
		return nil
	}
	if command == "tasks" {
		snapshot, err := steerline.ReadTaskSnapshot(c)
		if err != nil {
			return err
		}
		b, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	if command == "answer" {
		request, _, err := steerline.SubmitDecisionAnswer(ctx, c, steerline.AnswerOptions{RunID: *runID, DecisionID: *decisionID, Option: *option, Note: *note, By: *by})
		if err != nil {
			return err
		}
		fmt.Println("command_id", request.CommandID)
		if *waitReceipt {
			result, err := waitServiceCommand(ctx, c, request.CommandID, 2*time.Minute)
			if err != nil {
				return err
			}
			data, _ := json.Marshal(result)
			fmt.Println(string(data))
			if result.Status != "accepted" {
				return fmt.Errorf("decision command is %s: %s", result.Status, strings.TrimSpace(string(result.Result)))
			}
		}
		return nil
	}
	if command == "watch" {
		if useWatchTUI(viewer.IsInteractive(), *jsonOut, *rawEvents, *noTUI, *follow, *fromStart) {
			return viewer.Run(ctx, c, viewer.Options{RunID: *runID, Color: steerline.ColorEnabled()})
		}
		return steerline.Watch(ctx, c, *runID, steerline.WatchOptions{Follow: *follow, FromStart: *fromStart, JSON: *jsonOut, Raw: *rawEvents, Color: steerline.ColorEnabled()})
	}
	if command == "run" {
		if *serviceWindow {
			projectName := strings.TrimSuffix(filepath.Base(*config), filepath.Ext(*config))
			windowCtx, windowCancel, err := steerline.ServiceWindowContext(ctx, projectName)
			if err != nil {
				return err
			}
			defer windowCancel()
			ctx = windowCtx
		}
		return steerline.RunConfiguredService(ctx, c, *dispatchMode, *startupID, *console)
	}
	if e = c.Preflight(ctx, command == "retry" || command == "check"); e != nil {
		return e
	}
	if command == "check" {
		fmt.Println("Configuration, native authority binding, Git, PR provider and project step contracts verified; no task claimed.")
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
	fmt.Printf("Steerline M0 running: %s -> %s/%s; Ctrl+C or steerline stop to exit\n", c.Repository, c.Remote, c.Branch)
	return s.RunWithDispatch(ctx, *dispatchMode)
}

func submitRecoveryCommand(ctx context.Context, c steerline.Config, command, runID string, confirmed bool, reason, requestedBy string, wait bool) error {
	if err := c.ResolveIdentity(ctx); err != nil {
		return err
	}
	runtimeState, err := steerline.LoadDatabaseRuntime(c)
	if err != nil {
		return err
	}
	if runtimeState == nil {
		return errors.New("no current runtime projection; recovery commands require the live service")
	}
	var target *steerline.Run
	for _, run := range runtimeState.Runs {
		if run != nil && run.RunID == runID {
			target = run
			break
		}
	}
	if target == nil {
		return fmt.Errorf("run %s not found", runID)
	}
	kind := ""
	var body any
	switch command {
	case "retry":
		kind = "retry_run"
		body = map[string]any{"confirmed_process_stopped": confirmed, "confirmation_basis": strings.TrimSpace(reason)}
	case "mark-stopped":
		kind = "mark_stopped"
		body = map[string]any{"confirmed_process_stopped": true, "confirmation_basis": strings.TrimSpace(reason)}
	default:
		return fmt.Errorf("unsupported recovery command %q", command)
	}
	if requestedBy == "" {
		requestedBy = steerline.DefaultRequestedBy()
	}
	commandID := steerline.NewControlRequestID()
	queued, _, err := steerline.SubmitServiceCommand(ctx, c, kind, runID, requestedBy, commandID, target.ControlRevision, body)
	if err != nil {
		return err
	}
	fmt.Println("command_id", queued.CommandID)
	if !wait {
		return nil
	}
	result, err := waitServiceCommand(ctx, c, queued.CommandID, 2*time.Minute)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(result)
	fmt.Println(string(data))
	if result.Status != "accepted" {
		return fmt.Errorf("recovery command is %s: %s", result.Status, strings.TrimSpace(string(result.Result)))
	}
	return nil
}

func waitServiceCommand(ctx context.Context, c steerline.Config, commandID string, timeout time.Duration) (steerline.CommandRecord, error) {
	if err := c.ResolveIdentity(ctx); err != nil {
		return steerline.CommandRecord{}, fmt.Errorf("resolve project identity for command receipt: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	unconfirmed := func() (steerline.CommandRecord, error) {
		return steerline.CommandRecord{}, fmt.Errorf("command %s remains unconfirmed; query this same command ID later", commandID)
	}
	for {
		if waitCtx.Err() != nil {
			return unconfirmed()
		}
		reader, openErr := steerline.OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
		if openErr != nil {
			return steerline.CommandRecord{}, openErr
		}
		current, readErr := reader.Command(waitCtx, c.Identity(), commandID)
		_ = reader.Close()
		if readErr == nil && (current.Status == "accepted" || current.Status == "rejected" || current.Status == "failed" || current.Status == "indeterminate") {
			return current, nil
		}
		if waitCtx.Err() != nil {
			return unconfirmed()
		}
		if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) && !os.IsNotExist(readErr) {
			return steerline.CommandRecord{}, readErr
		}
		select {
		case <-waitCtx.Done():
			return unconfirmed()
		case <-ticker.C:
		}
	}
}

func runConsoleCommand(args []string) error {
	fs := flag.NewFlagSet("console", flag.ContinueOnError)
	config := fs.String("config", "", "open one project config; default enumerates STEERLINE_HOME/etc/*.json")
	runID := fs.String("run-id", "", "focus an existing run ID or task ID in the selected project")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if !viewer.IsInteractive() {
		return errors.New("console requires an interactive terminal")
	}
	projects, err := consoleui.LoadProjects(*config)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return consoleui.Run(ctx, projects, consoleui.Options{RunID: *runID})
}

func runNativeTaskCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: steerline task <validate|create|accept-existing|discard|reopen|graph|export|show|list> -config config.json")
	}
	subcommand := args[0]
	fs := flag.NewFlagSet("task "+subcommand, flag.ContinueOnError)
	config := fs.String("config", "steerline.local.json", "configuration JSON")
	taskID := fs.String("task-id", "", "native task ID")
	runID := fs.String("run-id", "", "exact workflow run to abandon when reopening a task")
	predecessors := fs.String("predecessors", "", "comma-separated native predecessor task IDs")
	confirmedStopped := fs.Bool("confirm-process-stopped", false, "operator confirmed the old worker process stopped")
	confirmReset := fs.Bool("confirm-unfinished-reset", false, "operator requests abandonment of an unfinished native Run after exact external-state checks")
	cursor := fs.String("cursor", "", "native task list cursor")
	limit := fs.Int("limit", 100, "list page size (default 100); graph/export default to 500 nodes, maximum 500")
	status := fs.String("status", "", "filter graph nodes by native task status")
	direction := fs.String("direction", "", "task neighborhood: upstream, downstream or both")
	depth := fs.Int("depth", 1, "task neighborhood depth (default 1, maximum 10)")
	format := fs.String("format", "", "export format (json)")
	output := fs.String("output", "", "task graph export path")
	overwrite := fs.Bool("overwrite", false, "allow replacing an existing export file")
	file := fs.String("file", "", "task-batch/v1 JSON file")
	requestID := fs.String("request-id", "", "stable idempotency key for this task command")
	expectedAuthority := fs.Uint64("expected-authority-version", 0, "sampled native project authority version")
	expectedVersion := fs.Uint64("expected-version", 0, "expected native task version")
	expectedGraphRevision := fs.Uint64("expected-graph-revision", 0, "sampled native graph revision")
	reason := fs.String("reason", "", "reason for discarding or reopening the task")
	by := fs.String("by", "", "task command requester")
	if subcommand != "show" && subcommand != "list" && subcommand != "validate" && subcommand != "create" && subcommand != "accept-existing" && subcommand != "discard" && subcommand != "reopen" && subcommand != "add-predecessors" && subcommand != "graph" && subcommand != "export" {
		return fmt.Errorf("unsupported task command %q", subcommand)
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	allowedFlags := map[string]map[string]bool{
		"show":             {"config": true, "task-id": true},
		"list":             {"config": true, "cursor": true, "limit": true},
		"validate":         {"config": true, "file": true},
		"create":           {"config": true, "file": true, "request-id": true, "expected-authority-version": true, "by": true},
		"accept-existing":  {"config": true, "file": true},
		"discard":          {"config": true, "task-id": true, "request-id": true, "expected-version": true, "reason": true, "by": true},
		"reopen":           {"config": true, "task-id": true, "run-id": true, "request-id": true, "expected-version": true, "confirm-process-stopped": true, "confirm-unfinished-reset": true, "reason": true, "by": true},
		"add-predecessors": {"config": true, "task-id": true, "predecessors": true, "run-id": true, "request-id": true, "expected-version": true, "expected-graph-revision": true, "confirm-process-stopped": true, "reason": true, "by": true},
		"graph":            {"config": true, "task-id": true, "cursor": true, "limit": true, "status": true, "direction": true, "depth": true},
		"export":           {"config": true, "task-id": true, "status": true, "direction": true, "depth": true, "format": true, "output": true, "overwrite": true},
	}
	var unexpectedFlag string
	limitWasSet := false
	cursorWasSet := false
	graphRevisionWasSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "limit" {
			limitWasSet = true
		}
		if f.Name == "cursor" {
			cursorWasSet = true
		}
		if f.Name == "expected-graph-revision" {
			graphRevisionWasSet = true
		}
		if !allowedFlags[subcommand][f.Name] && unexpectedFlag == "" {
			unexpectedFlag = f.Name
		}
	})
	if unexpectedFlag != "" {
		return fmt.Errorf("task %s does not accept -%s", subcommand, unexpectedFlag)
	}
	effectiveLimit := *limit
	if (subcommand == "graph" || subcommand == "export") && !limitWasSet {
		effectiveLimit = 500
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	switch subcommand {
	case "show":
		if *taskID == "" {
			return errors.New("task show requires -task-id")
		}
		if err := steerline.ValidateNativeTaskID(*taskID); err != nil {
			return err
		}
		if *cursor != "" {
			return errors.New("task show does not accept -cursor")
		}
	case "list":
		if *taskID != "" {
			return errors.New("task list does not accept -task-id")
		}
	case "validate":
		if *file == "" {
			return errors.New("task validate requires -file")
		}
		if *taskID != "" || *cursor != "" || *requestID != "" || *expectedAuthority != 0 {
			return errors.New("task validate accepts only -file and -config")
		}
	case "create":
		if *file == "" || *requestID == "" || *expectedAuthority == 0 {
			return errors.New("task create requires -file, -request-id and -expected-authority-version")
		}
		if *taskID != "" || *cursor != "" {
			return errors.New("task create does not accept -task-id or -cursor")
		}
	case "accept-existing":
		if *file == "" || *taskID != "" || *cursor != "" || *requestID != "" || *expectedAuthority != 0 || *expectedVersion != 0 || *runID != "" || *by != "" {
			return errors.New("task accept-existing accepts only -config and -file; identity and operator are fixed in the manifest")
		}
	case "discard":
		if *taskID == "" || *requestID == "" || *expectedVersion == 0 || strings.TrimSpace(*reason) == "" {
			return errors.New("task discard requires -task-id, -request-id, -expected-version and -reason")
		}
		if err := steerline.ValidateNativeTaskID(*taskID); err != nil {
			return err
		}
		if *file != "" || *cursor != "" || *expectedAuthority != 0 {
			return errors.New("task discard does not accept -file, -cursor or -expected-authority-version")
		}
	case "reopen":
		if *taskID == "" || *runID == "" || *requestID == "" || *expectedVersion == 0 || *confirmedStopped == *confirmReset || strings.TrimSpace(*reason) == "" || strings.TrimSpace(*by) == "" {
			return errors.New("task reopen requires -task-id, -run-id, -request-id, -expected-version, exactly one of -confirm-process-stopped or -confirm-unfinished-reset, -reason and -by")
		}
		if err := steerline.ValidateNativeTaskID(*taskID); err != nil {
			return err
		}
	case "add-predecessors":
		if *taskID == "" || strings.TrimSpace(*predecessors) == "" || *requestID == "" || *expectedVersion == 0 || !graphRevisionWasSet || strings.TrimSpace(*reason) == "" || strings.TrimSpace(*by) == "" {
			return errors.New("task add-predecessors requires -task-id, -predecessors, -request-id, -expected-version, -expected-graph-revision, -reason and -by")
		}
		if err := steerline.ValidateNativeTaskID(*taskID); err != nil {
			return err
		}
		if (*runID == "") != !*confirmedStopped {
			return errors.New("task add-predecessors requires -run-id and -confirm-process-stopped together for a claimed task")
		}
	case "graph":
		if *file != "" || *requestID != "" || *expectedAuthority != 0 || *expectedVersion != 0 || *reason != "" || *by != "" {
			return errors.New("task graph does not accept task mutation flags")
		}
		if cursorWasSet && (*taskID != "" || *status != "" || *direction != "" || *depth != 1) {
			return errors.New("task graph -cursor cannot be combined with status or neighborhood filters")
		}
	case "export":
		if *format != "json" || strings.TrimSpace(*output) == "" {
			return errors.New("task export requires -format json and -output <path>")
		}
		if *file != "" || *requestID != "" || *expectedAuthority != 0 || *expectedVersion != 0 || *reason != "" || *by != "" {
			return errors.New("task export does not accept task mutation flags")
		}
	default:
		return fmt.Errorf("unsupported task command %q", subcommand)
	}
	c, err := steerline.LoadConfig(*config)
	if err != nil {
		return err
	}
	if c.TaskAuthority != "native" {
		return errors.New("authority_mismatch: task commands require task_authority=native in the configuration")
	}
	var store *steerline.SQLiteStore
	if subcommand == "graph" || subcommand == "export" {
		store, err = steerline.OpenNativeTaskReader(context.Background(), &c)
	} else {
		store, err = steerline.OpenNativeTaskProject(context.Background(), &c)
	}
	if err != nil {
		return err
	}
	if subcommand == "create" || subcommand == "accept-existing" || subcommand == "discard" || subcommand == "reopen" || subcommand == "add-predecessors" {
		if err = store.Close(); err != nil {
			return err
		}
		lock, lockErr := steerline.LockRepository(c.Identity())
		if lockErr != nil {
			return lockErr
		}
		defer lock.Close()
		store, err = steerline.OpenNativeTaskCommandStore(context.Background(), &c)
		if err != nil {
			return err
		}
	}
	defer store.Close()
	query := steerline.QueryService{Store: store, Project: c.Identity()}
	var result steerline.NativeTaskPage
	switch subcommand {
	case "show":
		result, err = query.ShowNativeTask(context.Background(), *taskID)
	case "list":
		result, err = query.ListNativeTasks(context.Background(), *cursor, *limit)
	case "validate":
		payload, readErr := os.ReadFile(*file)
		if readErr != nil {
			return readErr
		}
		validation, validateErr := (steerline.CommandService{Store: store, Project: c.Identity()}).ValidateTaskBatch(context.Background(), c, payload)
		if validateErr != nil {
			return validateErr
		}
		if encodeErr := json.NewEncoder(os.Stdout).Encode(validation); encodeErr != nil {
			return encodeErr
		}
		if !validation.Valid {
			return errors.New("invalid_batch: validation failed; see reported issues")
		}
		return nil
	case "create":
		payload, readErr := os.ReadFile(*file)
		if readErr != nil {
			return readErr
		}
		receipt, createErr := (steerline.CommandService{Store: store, Project: c.Identity()}).CreateTaskBatch(context.Background(), c, payload, *requestID, *expectedAuthority, *by)
		if createErr != nil {
			return createErr
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "accept-existing":
		payload, readErr := os.ReadFile(*file)
		if readErr != nil {
			return readErr
		}
		input, parseErr := steerline.DecodeExistingAcceptanceInput(payload)
		if parseErr != nil {
			return parseErr
		}
		verifier := &steerline.PRClient{Config: c.PR, Runner: steerline.Runner{Dir: c.Repository, Timeout: 30 * time.Second}}
		receipt, completeErr := (steerline.CommandService{Store: store, Project: c.Identity()}).CompleteExistingAcceptance(context.Background(), c, input, verifier)
		if completeErr != nil {
			return completeErr
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "discard":
		receipt, discardErr := (steerline.CommandService{Store: store, Project: c.Identity()}).DiscardNativeTask(context.Background(), c, *taskID, *requestID, *expectedVersion, *reason, *by)
		if discardErr != nil {
			return discardErr
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "reopen":
		receipt, reopenErr := (steerline.CommandService{Store: store, Project: c.Identity()}).ReopenNativeTask(context.Background(), c, steerline.TaskReopenInput{TaskID: *taskID, WorkflowRunID: *runID, RequestID: *requestID, ExpectedVersion: *expectedVersion, ConfirmedProcessStopped: *confirmedStopped, ConfirmUnfinishedReset: *confirmReset, Reason: *reason, RequestedBy: *by})
		if reopenErr != nil {
			return reopenErr
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "add-predecessors":
		ids := strings.Split(*predecessors, ",")
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
		receipt, addErr := (steerline.CommandService{Store: store, Project: c.Identity()}).AddNativeTaskPredecessors(context.Background(), c, steerline.TaskPredecessorAddInput{TaskID: *taskID, PredecessorIDs: ids, RequestID: *requestID, ExpectedTaskVersion: *expectedVersion, ExpectedGraphRevision: *expectedGraphRevision, WorkflowRunID: *runID, ConfirmedProcessStopped: *confirmedStopped, Reason: *reason, RequestedBy: *by})
		if addErr != nil {
			return addErr
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "graph":
		if *taskID == "" && *status == "" && *direction == "" && *depth == 1 && (cursorWasSet || limitWasSet) {
			graph, graphErr := query.TaskGraph(context.Background(), *cursor, *limit)
			if graphErr != nil {
				return graphErr
			}
			return json.NewEncoder(os.Stdout).Encode(graph)
		}
		graph, graphErr := query.QueryTaskGraph(context.Background(), steerline.TaskGraphFilter{ProjectID: c.Identity(), Status: *status, CenterTaskID: *taskID, Direction: *direction, Depth: *depth, Limit: effectiveLimit})
		if graphErr != nil {
			return graphErr
		}
		return json.NewEncoder(os.Stdout).Encode(graph)
	case "export":
		graph, graphErr := query.QueryTaskGraph(context.Background(), steerline.TaskGraphFilter{ProjectID: c.Identity(), Status: *status, CenterTaskID: *taskID, Direction: *direction, Depth: *depth, Limit: effectiveLimit})
		if graphErr != nil {
			return graphErr
		}
		destination, pathErr := filepath.Abs(*output)
		if pathErr != nil {
			return pathErr
		}
		if taskExportTargetsNativeState(destination, c.RunDir) {
			return errors.New("task export output cannot replace native task state files")
		}
		receipt, exportErr := steerline.WriteTaskGraphExport(destination, graph, *overwrite)
		if exportErr != nil {
			return exportErr
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func sameTaskExportPath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	return aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo)
}

func taskExportTargetsNativeState(destination, runDir string) bool {
	stateDir, stateErr := filepath.EvalSymlinks(runDir)
	outputDir, outputErr := filepath.EvalSymlinks(filepath.Dir(destination))
	if stateErr == nil && outputErr == nil {
		relative, err := filepath.Rel(stateDir, outputDir)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	database := filepath.Join(runDir, "steerline.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if sameTaskExportPath(destination, database+suffix) {
			return true
		}
	}
	return false
}

func useWatchTUI(tty, jsonOut, raw, noTUI, follow, fromStart bool) bool {
	return tty && !jsonOut && !raw && !noTUI && follow && fromStart
}

// runProjects picks a project under <STEERLINE_HOME>\etc and re-execs this
// binary with the chosen action, so run/watch reuse their normal code paths
// while several projects can be launched side by side.
func runProjects(project, action string) error {
	act, e := pickAction(action)
	if e != nil {
		return e
	}
	if act == "console" {
		if project != "" {
			projects, listErr := steerline.ListProjects()
			if listErr != nil {
				return listErr
			}
			chosen, resolveErr := steerline.ResolveProject(projects, project)
			if resolveErr != nil {
				return resolveErr
			}
			return runConsoleCommand([]string{"-config", chosen.ConfigPath})
		}
		return runConsoleCommand(nil)
	}
	projects, e := steerline.ListProjects()
	if e != nil {
		return e
	}
	if len(projects) == 0 {
		return fmt.Errorf("no project configs under %s", steerline.SteerlineEtcDir())
	}
	if strings.EqualFold(act, "list") {
		printProjects(projects)
		return nil
	}
	chosen, e := pickProject(projects, project)
	if e != nil {
		return e
	}
	if chosen.Err != nil {
		return fmt.Errorf("config %s is invalid: %w", chosen.ConfigPath, chosen.Err)
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
	case "console", "1":
		return "console", nil
	case "watch", "run", "stop", "home", "list":
		return action, nil
	case "":
	default:
		return "", fmt.Errorf("unknown action %q (console|watch|run|stop|home|list)", action)
	}
	if !stdinIsTTY() {
		return "console", nil
	}
	fmt.Println("Actions:  1) console   2) watch logs   3) run service   4) stop service   5) home   6) list")
	line, e := readLine("action (1-6): ")
	if e != nil {
		return "", e
	}
	switch strings.TrimSpace(line) {
	case "1", "console":
		return "console", nil
	case "2", "viewer", "watch":
		return "watch", nil
	case "3", "run":
		return "run", nil
	case "4", "stop":
		return "stop", nil
	case "5", "home":
		return "home", nil
	case "6", "list":
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
