package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "steerline:", e)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: steerline <serve-app|stop-app|run|start-service|stop-service|check|control|task|answer|home|install|launchers> [-config config.json]")
	}
	if os.Args[1] == "task" {
		return runNativeTaskCommand(os.Args[2:])
	}
	if os.Args[1] == "serve-app" || os.Args[1] == "stop-app" {
		if len(os.Args) != 2 {
			return errors.New("application service takes no project arguments")
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if os.Args[1] == "serve-app" {
			return steerlineapp.ServeApplication(ctx)
		}
		ctx, timeout := context.WithTimeout(ctx, 45*time.Second)
		defer timeout()
		return steerlineapp.StopApplication(ctx)
	}
	command := os.Args[1]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	config := fs.String("config", "steerline.local.json", "configuration JSON")
	runID := fs.String("run-id", "", "run to answer")
	dispatchMode := fs.String("dispatch", "auto", "run dispatch mode: auto|manual")
	startupID := fs.String("startup-id", "", "internal startup attempt ID")
	taskID := fs.String("task-id", "", "control task target")
	executionProfile := fs.String("execution-profile", "", "control start_task/start_candidate execution profile ID")
	controlMode := fs.String("mode", "", "control set_dispatch_mode value: auto|manual")
	requestID := fs.String("request-id", "", "control request to query")
	waitReceipt := fs.Bool("wait", false, "control: wait for a terminal receipt")
	option := fs.String("option", "", "answer: chosen decision option id")
	decisionID := fs.String("decision-id", "", "answer: pending decision id (defaults to the run's pending decision)")
	note := fs.String("note", "", "answer: optional human note")
	by := fs.String("by", "", "answer: who answered (defaults to the OS user)")
	action := fs.String("action", "", "control action")
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

	if command != "run" && command != "start-service" && command != "stop-service" && command != "check" && command != "control" && command != "answer" && command != "home" && command != "install" && command != "launchers" {
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var c steerlineapp.Config
	var product *productclient.Client
	var e error
	switch command {
	case "control", "answer", "stop-service":
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
			snapshot := product.ReadProject(ctx, *taskID, productapi.ProjectSnapshot{})
			if snapshot.SelectedNative == nil || snapshot.NativeTasks == nil || snapshot.Connection.Snapshot.Page == nil {
				return fmt.Errorf("current native task/service snapshot is unavailable")
			}
			payload.ExpectedTaskVersion, payload.ExpectedGraphRevision = snapshot.SelectedNative.Version, snapshot.NativeTasks.GraphRevision
			candidateRevision, candidateInstance = &snapshot.NativeTasks.AuthorityVersion, snapshot.Connection.Snapshot.Page.Service.InstanceID
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
			snapshot := product.ReadProject(ctx, "", productapi.ProjectSnapshot{})
			runtimeState := snapshot.Connection.Snapshot.Page
			if runtimeState == nil {
				return fmt.Errorf("service snapshot is unavailable")
			}
			var target *productapi.HostActionTarget
			for _, run := range runtimeState.Runs {
				if run.RunID == *runID {
					var targetErr error
					target, targetErr = productapi.EnvironmentTarget(run)
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
		id := productclient.NewRequestID()
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
	if command == "answer" {
		if *requestID != "" {
			if *runID != "" || *decisionID != "" || *option != "" || *note != "" {
				return errors.New("-request-id cannot be combined with a new answer")
			}
			var result productapi.CommandReceipt
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
		id := productclient.NewRequestID()
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
				return fmt.Errorf("decision command is %s: %s", result.Status, strings.TrimSpace(result.ResultError))
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
		if e = steerlineapp.Preflight(ctx, &c, true); e != nil {
			return e
		}
		fmt.Println("Worker-flow configuration, native authority binding, Git and PR provider verified; no task claimed.")
		return nil
	}
	return fmt.Errorf("unsupported command %q", command)
}

// printHome shows where this machine keeps the shared SteerLine binary, configs
// and per-repo state, resolved independently of the current source worktree.
func printHome(configPath string) {
	runtimeInfo := steerlineapp.CurrentRuntimeIdentity(configPath)
	fmt.Println("STEERLINE_HOME", runtimeInfo.Home)
	fmt.Println("executable    ", runtimeInfo.Executable)
	fmt.Println("source commit ", runtimeInfo.SourceCommit)
	fmt.Println("source dirty  ", runtimeInfo.SourceDirty)
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
}
