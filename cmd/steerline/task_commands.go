package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/taskview"
	steerlineapp "github.com/erranliu/SteerLine/internal/steerlineapp"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

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
		receipt, addErr := product.AddPredecessors(ctx, productapi.TaskPredecessorAddInput{TaskID: *taskID, PredecessorIDs: ids, RequestID: *requestID, ExpectedTaskVersion: *expectedVersion, ExpectedGraphRevision: *expectedGraphRevision, Reason: *reason, RequestedBy: *by})
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
