package main

import (
	"bufio"
	"context"
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

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/task"
	"github.com/erranliu/SteerLine/internal/steerline/taskview"
	steerlineapp "github.com/erranliu/SteerLine/internal/steerlineapp"
	"github.com/erranliu/SteerLine/internal/steerlineclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "steerline:", e)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: steerline <console|run|start-service|stop-service|check|control|tasks|task|stop|answer|certify-backend|home|install|launchers|projects> [-config config.json]")
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
	runID := fs.String("run-id", "", "run to answer")
	dispatchMode := fs.String("dispatch", "auto", "run dispatch mode: auto|manual")
	startupID := fs.String("startup-id", "", "internal startup attempt ID")
	taskID := fs.String("task-id", "", "control task target")
	executionProfile := fs.String("execution-profile", "", "control start_task/start_candidate execution profile ID")
	candidateReason := fs.String("reason", "", "abandon_candidate: irreversible experiment exit reason")
	controlMode := fs.String("mode", "", "control set_dispatch_mode value: auto|manual")
	requestID := fs.String("request-id", "", "control request to query")
	waitReceipt := fs.Bool("wait", false, "control: wait for a terminal receipt")
	evidence := fs.String("evidence", "", "certify-backend: comma-separated Windows native evidence JSON files")
	option := fs.String("option", "", "answer: chosen decision option id")
	decisionID := fs.String("decision-id", "", "answer: pending decision id (defaults to the run's pending decision)")
	note := fs.String("note", "", "answer: optional human note")
	by := fs.String("by", "", "answer: who answered (defaults to the OS user)")
	proj := fs.String("project", "", "projects: project name or number")
	action := fs.String("action", "", "control action or projects: console|run|stop|home|list")
	console := fs.String("console", "", "service console rendering: pretty|summary|json|off (default pretty on a terminal)")
	serviceWindow := fs.Bool("service-window", false, "run inside a required visible Windows service window")
	if e := fs.Parse(os.Args[2:]); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *executionProfile != "" && (command != "control" || *action != "start_task" && *action != "start_candidate" || *requestID != "") {
		return fmt.Errorf("-execution-profile is only valid with control -action start_task|start_candidate")
	}
	if *candidateReason != "" && (command != "control" || *action != "abandon_candidate" || *requestID != "") {
		return fmt.Errorf("-reason is only valid with control -action abandon_candidate")
	}
	if command == "codex-session-launch" {
		return steerlineapp.RunSessionLauncher(context.Background(), *config, steerlineapp.NewProcessHost(30*time.Second))
	}
	if command != "run" && command != "start-service" && command != "stop-service" && command != "check" && command != "stop" && command != "control" && command != "tasks" && command != "answer" && command != "certify-backend" && command != "home" && command != "install" && command != "launchers" && command != "projects" {
		return fmt.Errorf("unknown command %q", command)
	}
	if command == "run" && *dispatchMode != "manual" && *dispatchMode != "auto" {
		return fmt.Errorf("-dispatch must be manual or auto")
	}
	if *serviceWindow && command != "run" {
		return fmt.Errorf("-service-window is only valid with run")
	}
	if command == "install" {
		dst, e := steerlineapp.InstallSelf()
		if e != nil {
			return e
		}
		fmt.Println("installed", dst)
		return nil
	}
	if command == "launchers" {
		written, e := steerlineapp.InstallLaunchers()
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var c steerline.Config
	var product api.Client
	var e error
	switch command {
	case "tasks", "control", "stop", "answer", "stop-service":
		product, e = steerlineapp.OpenClient(ctx, *config)
		if e != nil {
			return e
		}
		defer product.Close()
	default:
		c, e = steerlineapp.LoadConfig(*config)
		if e != nil {
			return e
		}
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
		report, err := steerlineapp.CertifyExecutionBackend(ctx, c, paths)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	if command == "start-service" {
		result, err := steerlineapp.StartConfiguredService(ctx, c, *config, steerline.StartServiceOptions{})
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
		result, err := product.StopService(ctx, *by, 30*time.Second)
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
		id := steerlineclient.NewRequestID()
		request, err := product.SubmitControl(ctx, productapi.ControlInput{Action: productapi.StopService, RequestID: id, RequestedBy: *by})
		if err != nil {
			fmt.Println("request_id", id)
			return err
		}
		fmt.Println("request_id", request.RequestID)
		if *waitReceipt {
			receipt, waitErr := product.WaitControl(ctx, request.RequestID, 100*time.Millisecond)
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
			var r *productapi.ControlReceipt
			var err error
			if *waitReceipt {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				r, err = product.WaitControl(ctx, *requestID, 100*time.Millisecond)
			} else {
				r, err = product.Receipt(ctx, *requestID)
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
		var payload productapi.ControlPayload
		var candidateRevision *uint64
		var candidateInstance string
		switch *action {
		case "start_candidate":
			if *taskID == "" || *controlMode != "" || *runID != "" {
				return fmt.Errorf("start_candidate requires -task-id and does not accept -mode/-run-id")
			}
			payload, e = product.StartTaskPayload(ctx, *taskID, *executionProfile)
			if e != nil {
				return e
			}
			snapshot := product.ReadProject(ctx, *taskID, api.ProjectSnapshot{})
			if snapshot.SelectedNative == nil || snapshot.NativeTasks == nil || snapshot.Connection.Snapshot.Runtime == nil {
				return fmt.Errorf("current native task/service snapshot is unavailable")
			}
			payload.ExpectedTaskVersion, payload.ExpectedGraphRevision = snapshot.SelectedNative.Version, snapshot.NativeTasks.GraphRevision
			candidateRevision, candidateInstance = &snapshot.NativeTasks.AuthorityVersion, snapshot.Connection.Snapshot.Runtime.Service.InstanceID
		case "abandon_candidate":
			if *runID == "" || strings.TrimSpace(*candidateReason) == "" || *controlMode != "" {
				return fmt.Errorf("abandon_candidate requires exact -run-id and -reason")
			}
			snapshot := product.ReadProject(ctx, *taskID, api.ProjectSnapshot{})
			runtimeState := snapshot.Connection.Snapshot.Runtime
			if runtimeState == nil {
				return fmt.Errorf("service snapshot is unavailable")
			}
			var target *contracts.Run
			for _, run := range runtimeState.Runs {
				if run.RunID == *runID {
					target = run
					break
				}
			}
			if target == nil || *taskID != "" && *taskID != target.TaskID {
				return fmt.Errorf("exact candidate does not belong to the requested task")
			}
			payload = productapi.ControlPayload{TaskID: target.TaskID, WorkflowRunID: target.RunID, ExpectedCandidateVersion: target.ControlRevision, Reason: *candidateReason}
			for _, task := range runtimeState.Tasks {
				if task.TaskID == target.TaskID {
					payload.ExpectedTaskVersion = task.Version
					break
				}
			}
			candidateRevision, candidateInstance = &runtimeState.Control.Revision, runtimeState.Service.InstanceID
		case "start_task":
			if *taskID == "" {
				return fmt.Errorf("start_task requires -task-id")
			}
			if *controlMode != "" {
				return fmt.Errorf("start_task does not accept -mode")
			}
			payload, e = product.StartTaskPayload(ctx, *taskID, *executionProfile)
			if e != nil {
				return e
			}
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
			payload = productapi.ControlPayload{Mode: *controlMode}
		case "open_terminal", "recheck_environment":
			if *runID == "" {
				return fmt.Errorf("environment action requires -run-id")
			}
			snapshot := product.ReadProject(ctx, "", api.ProjectSnapshot{})
			runtimeState := snapshot.Connection.Snapshot.Runtime
			if runtimeState == nil {
				return fmt.Errorf("service snapshot is unavailable")
			}
			var target *productapi.HostActionTarget
			for _, run := range runtimeState.Runs {
				if run.RunID == *runID {
					var targetErr error
					target, targetErr = api.EnvironmentTarget(run)
					if targetErr != nil {
						return targetErr
					}
					break
				}
			}
			if target == nil {
				return fmt.Errorf("run has no current environment blocker")
			}
			payload = productapi.ControlPayload{Environment: target}
		case "pause_dispatch", "resume_dispatch", "stop_service":
			if *taskID != "" || *controlMode != "" {
				return fmt.Errorf("%s does not accept task or mode parameters", *action)
			}
		default:
			return fmt.Errorf("unsupported control action %q", *action)
		}
		id := steerlineclient.NewRequestID()
		req, err := product.SubmitControl(ctx, productapi.ControlInput{Action: productapi.ControlAction(*action), Payload: payload, RequestID: id, RequestedBy: *by, ExpectedRevision: candidateRevision, InstanceID: candidateInstance})
		if err != nil {
			fmt.Println("request_id", id)
			return err
		}
		fmt.Println("request_id", req.RequestID)
		if *waitReceipt {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			r, err := product.WaitControl(ctx, req.RequestID, 100*time.Millisecond)
			if err != nil {
				return err
			}
			b, _ := json.Marshal(r)
			fmt.Println(string(b))
		}
		return nil
	}
	if command == "tasks" {
		snapshot, err := product.TaskSnapshot(ctx)
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
		if *requestID != "" {
			if *runID != "" || *decisionID != "" || *option != "" || *note != "" {
				return errors.New("-request-id cannot be combined with a new answer")
			}
			var result storageports.CommandRecord
			var err error
			if *waitReceipt {
				result, err = product.WaitCommand(ctx, *requestID, 2*time.Minute)
			} else {
				result, err = product.Command(ctx, *requestID)
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		id := steerlineclient.NewRequestID()
		request, _, err := product.Answer(ctx, productapi.AnswerInput{RunID: *runID, DecisionID: *decisionID, Option: *option, Note: *note, RequestedBy: *by, CommandID: id})
		if err != nil {
			fmt.Println("command_id", id)
			return err
		}
		fmt.Println("command_id", request.CommandID)
		if *waitReceipt {
			result, err := product.WaitCommand(ctx, request.CommandID, 2*time.Minute)
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
		return steerlineapp.RunConfiguredService(ctx, c, *dispatchMode, *startupID, *console)
	}
	if command == "check" {
		if e = c.Preflight(ctx, true); e != nil {
			return e
		}
		fmt.Println("Worker-flow configuration, native authority binding, Git and PR provider verified; no task claimed.")
		return nil
	}
	return fmt.Errorf("unsupported command %q", command)
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
	if !stdinIsTTY() || !stdoutIsTTY() {
		return errors.New("console requires an interactive terminal")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return steerlineapp.RunConsole(ctx, *config, *runID)
}

func runNativeTaskCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: steerline task <validate|create|discard|add-predecessors|graph|export|show|list> -config config.json")
	}
	subcommand := args[0]
	fs := flag.NewFlagSet("task "+subcommand, flag.ContinueOnError)
	config := fs.String("config", "steerline.local.json", "configuration JSON")
	taskID := fs.String("task-id", "", "native task ID")
	predecessors := fs.String("predecessors", "", "comma-separated native predecessor task IDs")
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
	reason := fs.String("reason", "", "reason for discarding or changing the task")
	by := fs.String("by", "", "task command requester")
	if subcommand != "show" && subcommand != "list" && subcommand != "validate" && subcommand != "create" && subcommand != "discard" && subcommand != "add-predecessors" && subcommand != "graph" && subcommand != "export" {
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
		"discard":          {"config": true, "task-id": true, "request-id": true, "expected-version": true, "reason": true, "by": true},
		"add-predecessors": {"config": true, "task-id": true, "predecessors": true, "request-id": true, "expected-version": true, "expected-graph-revision": true, "reason": true, "by": true},
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
		if err := taskmodel.ValidateID(*taskID); err != nil {
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
	case "discard":
		if *taskID == "" || *requestID == "" || *expectedVersion == 0 || strings.TrimSpace(*reason) == "" {
			return errors.New("task discard requires -task-id, -request-id, -expected-version and -reason")
		}
		if err := taskmodel.ValidateID(*taskID); err != nil {
			return err
		}
		if *file != "" || *cursor != "" || *expectedAuthority != 0 {
			return errors.New("task discard does not accept -file, -cursor or -expected-authority-version")
		}
		if err := taskmodel.ValidateID(*taskID); err != nil {
			return err
		}
	case "add-predecessors":
		if *taskID == "" || strings.TrimSpace(*predecessors) == "" || *requestID == "" || *expectedVersion == 0 || !graphRevisionWasSet || strings.TrimSpace(*reason) == "" || strings.TrimSpace(*by) == "" {
			return errors.New("task add-predecessors requires -task-id, -predecessors, -request-id, -expected-version, -expected-graph-revision, -reason and -by")
		}
		if err := taskmodel.ValidateID(*taskID); err != nil {
			return err
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
	ctx := context.Background()
	opened, err := steerlineapp.OpenClient(ctx, *config)
	var product interface {
		api.Lifecycle
		api.TaskClient
	} = opened
	if err != nil {
		return err
	}
	defer product.Close()
	var result contracts.NativeTaskPage
	switch subcommand {
	case "show":
		result, err = product.ShowTask(ctx, *taskID)
	case "list":
		result, err = product.ListTasks(ctx, *cursor, *limit)
	case "validate":
		payload, readErr := os.ReadFile(*file)
		if readErr != nil {
			return readErr
		}
		validation, validateErr := product.ValidateTaskBatch(ctx, payload)
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
		receipt, createErr := product.CreateTaskBatch(ctx, productapi.CreateInput{Payload: payload, RequestID: *requestID, ExpectedAuthorityVersion: *expectedAuthority, RequestedBy: *by})
		if createErr != nil {
			return createErr
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "discard":
		receipt, discardErr := product.DiscardTask(ctx, productapi.DiscardInput{TaskID: *taskID, RequestID: *requestID, ExpectedVersion: *expectedVersion, Reason: *reason, RequestedBy: *by})
		if discardErr != nil {
			return discardErr
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "add-predecessors":
		ids := strings.Split(*predecessors, ",")
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
		receipt, addErr := product.AddPredecessors(ctx, task.TaskPredecessorAddInput{TaskID: *taskID, PredecessorIDs: ids, RequestID: *requestID, ExpectedTaskVersion: *expectedVersion, ExpectedGraphRevision: *expectedGraphRevision, Reason: *reason, RequestedBy: *by})
		if addErr != nil {
			return addErr
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "graph":
		if *taskID == "" && *status == "" && *direction == "" && *depth == 1 && (cursorWasSet || limitWasSet) {
			graph, graphErr := product.TaskGraph(ctx, *cursor, *limit)
			if graphErr != nil {
				return graphErr
			}
			return json.NewEncoder(os.Stdout).Encode(graph)
		}
		graph, graphErr := product.QueryTaskGraph(ctx, taskview.Filter{Status: *status, CenterTaskID: *taskID, Direction: *direction, Depth: *depth, Limit: effectiveLimit})
		if graphErr != nil {
			return graphErr
		}
		return json.NewEncoder(os.Stdout).Encode(graph)
	case "export":
		graph, graphErr := product.QueryTaskGraph(ctx, taskview.Filter{Status: *status, CenterTaskID: *taskID, Direction: *direction, Depth: *depth, Limit: effectiveLimit})
		if graphErr != nil {
			return graphErr
		}
		destination, pathErr := filepath.Abs(*output)
		if pathErr != nil {
			return pathErr
		}
		if taskExportTargetsNativeState(destination, product.Project().RunDir) {
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

// runProjects picks a project under <STEERLINE_HOME>\etc and re-execs this
// binary with the chosen action, so run reuses its normal code path
// while several projects can be launched side by side.
func runProjects(project, action string) error {
	act, e := pickAction(action)
	if e != nil {
		return e
	}
	if act == "console" {
		if project != "" {
			projects, listErr := steerlineapp.ListProjects()
			if listErr != nil {
				return listErr
			}
			chosen, resolveErr := steerlineapp.ResolveProject(projects, project)
			if resolveErr != nil {
				return resolveErr
			}
			return runConsoleCommand([]string{"-config", chosen.ConfigPath})
		}
		return runConsoleCommand(nil)
	}
	projects, e := steerlineapp.ListProjects()
	if e != nil {
		return e
	}
	if len(projects) == 0 {
		return fmt.Errorf("no project configs under %s", steerlineapp.EtcDir())
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

func pickProject(projects []steerlineapp.ProjectInfo, sel string) (steerlineapp.ProjectInfo, error) {
	if sel != "" {
		return steerlineapp.ResolveProject(projects, sel)
	}
	if len(projects) == 1 {
		return projects[0], nil
	}
	printProjects(projects)
	if !stdinIsTTY() {
		return steerlineapp.ProjectInfo{}, fmt.Errorf("multiple projects; pass -project <name|number>")
	}
	line, e := readLine("project (number or name): ")
	if e != nil {
		return steerlineapp.ProjectInfo{}, e
	}
	return steerlineapp.ResolveProject(projects, line)
}

func pickAction(action string) (string, error) {
	switch action {
	case "console", "1":
		return "console", nil
	case "run", "stop", "home", "list":
		return action, nil
	case "":
	default:
		return "", fmt.Errorf("unknown action %q (console|run|stop|home|list)", action)
	}
	if !stdinIsTTY() {
		return "console", nil
	}
	fmt.Println("Actions:  1) console   2) run service   3) stop service   4) home   5) list")
	line, e := readLine("action (1-5): ")
	if e != nil {
		return "", e
	}
	switch strings.TrimSpace(line) {
	case "1", "console":
		return "console", nil
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

func printProjects(projects []steerlineapp.ProjectInfo) {
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

func stdoutIsTTY() bool {
	info, e := os.Stdout.Stat()
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
	fmt.Println("STEERLINE_HOME", steerlineapp.HomeDir())
	fmt.Println("bin           ", steerlineapp.BinDir())
	fmt.Println("etc           ", steerlineapp.EtcDir())
	fmt.Println("state root    ", steerlineapp.StateRoot())
	c, e := steerlineapp.LoadQueryConfig(configPath)
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
