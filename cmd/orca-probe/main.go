// Command orca-probe captures reproducible, sanitized evidence for Spec 01.
// It never creates workers, tasks, runs, worktrees, or messages.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const reportSchema = "steerline.orca-capability-report.v1"

type commandRunner struct {
	executable string
	timeout    time.Duration
}

type commandResult struct {
	stdout []byte
	stderr []byte
	err    error
}

func (r commandRunner) run(ctx context.Context, args ...string) commandResult {
	if r.timeout <= 0 {
		r.timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.executable, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return commandResult{stdout: []byte(stdout.String()), stderr: []byte(stderr.String()), err: err}
}

type capability struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Reason string   `json:"reason"`
	CLI    []string `json:"cli_evidence,omitempty"`
}

type mapping struct {
	LogicalField string `json:"logical_field"`
	OrcaSurface  string `json:"orca_surface,omitempty"`
	WireField    string `json:"wire_field"`
	Status       string `json:"status"`
	Evidence     string `json:"evidence"`
}

type cliCommand struct {
	Command string   `json:"command"`
	Usage   string   `json:"usage"`
	Flags   []string `json:"flags,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

type report struct {
	SchemaVersion string `json:"schema_version"`
	CapturedAt    string `json:"captured_at"`
	ProbeTool     string `json:"probe_tool"`
	Evidence      struct {
		CLISchema     string `json:"cli_schema"`
		CenterRuntime string `json:"center_runtime"`
		RemoteRuntime string `json:"remote_runtime"`
		ProtocolWire  string `json:"protocol_wire"`
	} `json:"evidence_classes"`
	Center struct {
		CLIVersion       string   `json:"cli_version,omitempty"`
		RuntimeVersion   string   `json:"runtime_version,omitempty"`
		RuntimeState     string   `json:"runtime_state,omitempty"`
		RuntimeReachable *bool    `json:"runtime_reachable,omitempty"`
		Platform         string   `json:"platform"`
		Capabilities     []string `json:"capabilities,omitempty"`
		SavedEnvironment int      `json:"saved_remote_environment_count"`
	} `json:"center"`
	Remote struct {
		Status         string `json:"status"`
		Reason         string `json:"reason"`
		RuntimeVersion string `json:"runtime_version"`
		Agent          string `json:"agent"`
		Platform       string `json:"platform"`
	} `json:"remote"`
	CLICommands  []cliCommand `json:"cli_commands"`
	Capabilities []capability `json:"capabilities"`
	Mappings     []mapping    `json:"field_mappings"`
	Limitations  []string     `json:"limitations"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "orca-probe:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(os.Stdout)
		return nil
	}
	switch args[0] {
	case "snapshot":
		return snapshot(args[1:])
	case "capture":
		return capture(args[1:])
	case "sanitize":
		return sanitizeFile(args[1:])
	default:
		return fmt.Errorf("unknown command %q; run orca-probe --help", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `orca-probe captures sanitized evidence for the SteerLine Orca remote protocol spec.

Usage:
  orca-probe snapshot [-orca <executable>] [-out <report.json>]
  orca-probe capture -operation <name> [operation flags] [-orca <executable>] [-out <sample.json>]
  orca-probe sanitize -in <input.json> [-out <sanitized.json>]

Snapshot is read-only. Capture only permits scoped read operations:
  home-status, remote-status, environment-show, repo-show, worktree-show,
  run-show, task-list, dispatch-show, worker-show, worker-read, worker-list,
  request-show, check-peek, check-all

Worker/run captures are scoped by exact IDs and use the local Run home.
Remote status/repository captures require an explicit saved environment.`)
}

func snapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	orca := fs.String("orca", "orca", "Orca CLI executable")
	out := fs.String("out", "", "write report to this path; stdout when omitted")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("snapshot accepts no positional arguments")
	}
	r := commandRunner{executable: *orca, timeout: 30 * time.Second}
	ctx := context.Background()

	report := newReport()
	version := r.run(ctx, "--version")
	if version.err == nil {
		report.Center.CLIVersion = strings.TrimSpace(string(version.stdout))
	} else {
		report.Limitations = append(report.Limitations, "Could not read the Orca CLI version.")
	}

	contextResult := r.run(ctx, "agent-context", "--json")
	if contextResult.err == nil {
		if commands, err := relevantCommands(contextResult.stdout); err == nil {
			report.CLICommands = commands
			report.Evidence.CLISchema = "captured"
		} else {
			report.Limitations = append(report.Limitations, "Orca agent-context output was not recognized as JSON.")
		}
	} else {
		report.Limitations = append(report.Limitations, "Could not read the Orca CLI command schema.")
	}

	statusResult := r.run(ctx, "status", "--json")
	if statusResult.err == nil {
		if err := readLocalStatus(statusResult.stdout, &report); err != nil {
			report.Limitations = append(report.Limitations, "Orca status output was not recognized: "+err.Error())
		} else {
			report.Evidence.CenterRuntime = "captured"
		}
	} else {
		report.Limitations = append(report.Limitations, "Could not read the local Orca runtime status.")
	}

	environmentsResult := r.run(ctx, "environment", "list", "--json")
	if environmentsResult.err == nil {
		if n, err := environmentCount(environmentsResult.stdout); err == nil {
			report.Center.SavedEnvironment = n
			if n == 0 {
				report.Remote.Reason = "No saved remote Orca environments were available to this probe."
			}
		} else {
			report.Limitations = append(report.Limitations, "Orca environment list output was not recognized.")
		}
	} else {
		report.Limitations = append(report.Limitations, "Could not inspect saved Orca environment count.")
	}

	return writeJSON(*out, report)
}

func newReport() report {
	r := report{SchemaVersion: reportSchema, CapturedAt: time.Now().UTC().Format(time.RFC3339), ProbeTool: "cmd/orca-probe"}
	r.Evidence.CLISchema = "not_tested"
	r.Evidence.CenterRuntime = "not_tested"
	r.Evidence.RemoteRuntime = "not_tested"
	r.Evidence.ProtocolWire = "not_tested"
	r.Center.Platform = runtime.GOOS
	r.Remote.Status = "not_tested"
	r.Remote.Reason = "No remote runtime version or remote execution evidence was captured."
	r.Remote.RuntimeVersion = "not_tested"
	r.Remote.Agent = "not_tested"
	r.Remote.Platform = "not_tested"
	r.Limitations = []string{}
	r.Capabilities = []capability{
		{ID: "background_coordinator", Status: "not_tested", Reason: "Requires a persistent coordinator Run on the selected home runtime.", CLI: []string{"orchestration run-create", "orchestration run-show"}},
		{ID: "remote_dispatch", Status: "not_tested", Reason: "Requires worker-start against an explicit disposable remote repo.", CLI: []string{"orchestration worker-start"}},
		{ID: "staged_same_workspace", Status: "not_tested", Reason: "Requires at least two sequential Dispatches on one remote worktree.", CLI: []string{"orchestration worker-start", "orchestration worker-show"}},
		{ID: "workspace_preparation", Status: "not_tested", Reason: "Remote base commit, branch, worktree and prompt access need live evidence.", CLI: []string{"orchestration worker-start", "repo show"}},
		{ID: "state_observation", Status: "not_tested", Reason: "Requires live, unverifiable and exited observations from a remote Dispatch.", CLI: []string{"orchestration worker-show", "orchestration worker-list"}},
		{ID: "incremental_output", Status: "not_tested", Reason: "Requires paged reads, cursor replay and source-change observations.", CLI: []string{"orchestration worker-read"}},
		{ID: "structured_messages", Status: "not_tested", Reason: "Requires cross-runtime status, question and worker_done round trips.", CLI: []string{"orchestration send", "orchestration check"}},
		{ID: "human_decisions", Status: "not_tested", Reason: "Requires ask timeout/resume, reply and worker-applied evidence.", CLI: []string{"orchestration ask", "orchestration reply"}},
		{ID: "completion_report", Status: "not_tested", Reason: "Requires accepted success/failure settlements and payload fidelity checks.", CLI: []string{"orchestration send", "orchestration task-update"}},
		{ID: "mutation_recovery", Status: "not_tested", Reason: "Requires controlled response-loss injections and request-ID recovery.", CLI: []string{"orchestration worker-start", "orchestration ask", "orchestration reply"}},
		{ID: "stop_and_release", Status: "not_tested", Reason: "Requires target-only stop, settled release and disconnected outcomes.", CLI: []string{"orchestration worker-stop", "orchestration worker-release"}},
		{ID: "step_script_receipt", Status: "not_tested", Reason: "Requires a deterministic remote script and its structured exit receipt.", CLI: []string{"orchestration worker-start", "orchestration send"}},
		{ID: "direct_script_execution", Status: "not_tested", Reason: "Requires a no-agent script launch, input/output and restart probe.", CLI: []string{"terminal create", "terminal read", "terminal wait"}},
		{ID: "compatibility_and_limits", Status: "not_tested", Reason: "Requires old/new runtime, unknown-field, payload-size and replay tests.", CLI: []string{"orca agent-context", "orchestration check"}},
	}
	r.Mappings = []mapping{
		{LogicalField: "workflow_run_id", OrcaSurface: "--run <run_id>", WireField: "unknown", Status: "not_tested", Evidence: "CLI schema only"},
		{LogicalField: "task_id", OrcaSurface: "--task <task_id> / task-create receipt", WireField: "unknown", Status: "not_tested", Evidence: "CLI schema only"},
		{LogicalField: "execution_id", OrcaSurface: "worker-start Dispatch receipt", WireField: "unknown", Status: "not_tested", Evidence: "CLI help describes Dispatch identity; no receipt captured"},
		{LogicalField: "operation_id", OrcaSurface: "--retry-request <request_id>", WireField: "unknown", Status: "not_tested", Evidence: "CLI schema exposes retry flag; durable ID lifetime unverified"},
		{LogicalField: "observation", OrcaSurface: "worker-show / worker-list", WireField: "unknown", Status: "not_tested", Evidence: "CLI schema only"},
		{LogicalField: "output_cursor", OrcaSurface: "worker-read --cursor <cursor>", WireField: "unknown", Status: "not_tested", Evidence: "CLI help documents opaque cursor/source behavior"},
		{LogicalField: "delivery", OrcaSurface: "orchestration check --peek / --ack <delivery_id>", WireField: "unknown", Status: "not_tested", Evidence: "CLI help documents FIFO batch and acknowledgement interface"},
		{LogicalField: "question", OrcaSurface: "ask --question / --resume; reply --id", WireField: "unknown", Status: "not_tested", Evidence: "CLI schema only"},
		{LogicalField: "decision_applied", OrcaSurface: "custom message payload", WireField: "unknown", Status: "not_tested", Evidence: "No cross-runtime payload captured"},
		{LogicalField: "step_result", OrcaSurface: "worker_done message and task settlement", WireField: "unknown", Status: "not_tested", Evidence: "CLI exposes outcome succeeded|failed; payload fidelity unverified"},
		{LogicalField: "resource_settlement", OrcaSurface: "worker-stop / worker-release / worker-retain", WireField: "unknown", Status: "not_tested", Evidence: "CLI help only"},
	}
	return r
}

func readLocalStatus(data []byte, r *report) error {
	var envelope struct {
		OK     bool `json:"ok"`
		Result struct {
			Target struct {
				Kind string `json:"kind"`
			} `json:"target"`
			Runtime struct {
				State        string   `json:"state"`
				Reachable    bool     `json:"reachable"`
				AppVersion   string   `json:"appVersion"`
				Capabilities []string `json:"capabilities"`
			} `json:"runtime"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if !envelope.OK {
		return errors.New("status response was not ok")
	}
	r.Center.RuntimeVersion = envelope.Result.Runtime.AppVersion
	r.Center.RuntimeState = envelope.Result.Runtime.State
	reachable := envelope.Result.Runtime.Reachable
	r.Center.RuntimeReachable = &reachable
	r.Center.Capabilities = envelope.Result.Runtime.Capabilities
	sort.Strings(r.Center.Capabilities)
	return nil
}

func environmentCount(data []byte) (int, error) {
	var envelope struct {
		OK     bool `json:"ok"`
		Result struct {
			Environments []json.RawMessage `json:"environments"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return 0, err
	}
	if !envelope.OK {
		return 0, errors.New("environment list response was not ok")
	}
	return len(envelope.Result.Environments), nil
}

func relevantCommands(data []byte) ([]cliCommand, error) {
	var schema struct {
		Commands []struct {
			Command string   `json:"command"`
			Usage   string   `json:"usage"`
			Flags   []string `json:"flags"`
			Notes   []string `json:"notes"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	wanted := map[string]bool{
		"orchestration run-create": true, "orchestration run-show": true, "orchestration worker-start": true,
		"orchestration worker-show": true, "orchestration worker-read": true, "orchestration worker-list": true,
		"orchestration worker-stop": true, "orchestration worker-abandon": true, "orchestration worker-release": true,
		"orchestration worker-retain": true, "orchestration ask": true, "orchestration reply": true,
		"orchestration check": true, "orchestration send": true, "orchestration request-show": true, "orchestration task-update": true,
		"orchestration task-list": true, "orchestration dispatch-show": true, "environment show": true,
		"repo show": true, "terminal create": true, "terminal read": true, "terminal wait": true,
	}
	var result []cliCommand
	for _, item := range schema.Commands {
		if wanted[item.Command] {
			result = append(result, cliCommand{Command: item.Command, Usage: item.Usage, Flags: item.Flags, Notes: item.Notes})
		}
	}
	if len(result) == 0 {
		return nil, errors.New("no relevant commands found")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Command < result[j].Command })
	return result, nil
}

func capture(args []string) error {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	orca := fs.String("orca", "orca", "Orca CLI executable")
	operation := fs.String("operation", "", "read-only operation to capture")
	environment := fs.String("environment", "", "explicit saved environment selector")
	repo := fs.String("repo", "", "exact repo selector, preferably id:<id>")
	worktree := fs.String("worktree", "", "exact remote worktree selector")
	runID := fs.String("run", "", "exact Run ID")
	taskID := fs.String("task", "", "exact task ID")
	dispatchID := fs.String("dispatch", "", "exact Dispatch ID")
	terminal := fs.String("terminal", "", "exact coordinator terminal handle for inbox reads")
	requestID := fs.String("request", "", "exact Orca mutation request ID")
	source := fs.String("source", "auto", "worker-read source: auto, transcript or terminal")
	cursor := fs.String("cursor", "", "opaque worker-read cursor")
	limit := fs.Int("limit", 100, "bounded worker-read rows")
	out := fs.String("out", "", "write sanitized sample to this path; stdout when omitted")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("capture accepts no positional arguments")
	}
	if *operation == "" {
		return errors.New("capture requires -operation")
	}

	var commandArgs []string
	var scope string
	switch *operation {
	case "home-status":
		commandArgs, scope = []string{"status", "--json"}, "home"
	case "remote-status":
		if *environment == "" {
			return errors.New("remote-status requires an explicit -environment")
		}
		commandArgs, scope = []string{"status", "--json", "--environment", *environment}, "selected environment"
	case "environment-show":
		if *environment == "" {
			return errors.New("environment-show requires an explicit -environment")
		}
		commandArgs, scope = []string{"environment", "show", "--environment", *environment, "--json"}, "selected environment"
	case "repo-show":
		if *environment == "" || *repo == "" {
			return errors.New("repo-show requires explicit -environment and -repo")
		}
		commandArgs, scope = []string{"repo", "show", "--repo", *repo, "--environment", *environment, "--json"}, "selected environment and repo"
	case "worktree-show":
		if *environment == "" || *worktree == "" {
			return errors.New("worktree-show requires explicit -environment and -worktree")
		}
		commandArgs, scope = []string{"worktree", "show", "--worktree", *worktree, "--environment", *environment, "--json"}, "selected environment and worktree"
	case "run-show":
		if *runID == "" {
			return errors.New("run-show requires -run")
		}
		commandArgs, scope = []string{"orchestration", "run-show", "--id", *runID, "--json"}, "home Run"
	case "task-list":
		if *runID == "" {
			return errors.New("task-list requires -run to scope the query")
		}
		commandArgs, scope = []string{"orchestration", "task-list", "--run", *runID, "--json"}, "home Run"
	case "dispatch-show":
		if *taskID == "" {
			return errors.New("dispatch-show requires -task")
		}
		commandArgs, scope = []string{"orchestration", "dispatch-show", "--task", *taskID, "--json"}, "home Run task"
	case "worker-show":
		if *dispatchID == "" {
			return errors.New("worker-show requires -dispatch")
		}
		commandArgs, scope = []string{"orchestration", "worker-show", "--dispatch", *dispatchID, "--json"}, "home Run Dispatch"
	case "worker-read":
		if *dispatchID == "" {
			return errors.New("worker-read requires -dispatch")
		}
		if *limit < 1 || *limit > 500 {
			return errors.New("worker-read -limit must be between 1 and 500")
		}
		if *source != "auto" && *source != "transcript" && *source != "terminal" {
			return errors.New("worker-read -source must be auto, transcript, or terminal")
		}
		commandArgs = []string{"orchestration", "worker-read", "--dispatch", *dispatchID, "--source", *source, "--limit", fmt.Sprint(*limit), "--json"}
		if *cursor != "" {
			commandArgs = append(commandArgs, "--cursor", *cursor)
		}
		scope = "home Run Dispatch"
	case "worker-list":
		if *runID == "" {
			return errors.New("worker-list requires -run to scope the query")
		}
		commandArgs, scope = []string{"orchestration", "worker-list", "--run", *runID, "--include-remote", "--limit", "100", "--json"}, "home Run"
	case "request-show":
		if *requestID == "" {
			return errors.New("request-show requires -request")
		}
		commandArgs, scope = []string{"orchestration", "request-show", "--request", *requestID, "--json"}, "home mutation request"
	case "check-peek":
		if *runID == "" || *terminal == "" {
			return errors.New("check-peek requires exact -run and -terminal")
		}
		commandArgs, scope = []string{"orchestration", "check", "--terminal", *terminal, "--run", *runID, "--peek", "--json"}, "home Run inbox (non-consuming peek)"
	case "check-all":
		if *runID == "" || *terminal == "" {
			return errors.New("check-all requires exact -run and -terminal")
		}
		commandArgs, scope = []string{"orchestration", "check", "--terminal", *terminal, "--run", *runID, "--all", "--json"}, "home Run inbox history (non-consuming all)"
	default:
		return fmt.Errorf("unsupported read-only operation %q", *operation)
	}

	result := commandRunner{executable: *orca, timeout: 45 * time.Second}.run(context.Background(), commandArgs...)
	record := map[string]any{
		"schema_version": "steerline.orca-probe-sample.v1",
		"captured_at":    time.Now().UTC().Format(time.RFC3339),
		"evidence_class": "captured_cli_response",
		"operation":      *operation,
		"scope":          scope,
		"exit_code":      exitCode(result.err),
		"response":       sanitizedResponse(result.stdout),
	}
	if result.err != nil {
		record["error"] = "Command failed; raw stderr omitted for privacy."
	}
	return writeJSON(*out, record)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func sanitizedResponse(data []byte) any {
	var decoded any
	if json.Unmarshal(data, &decoded) != nil {
		return "[non-JSON response omitted]"
	}
	return sanitizeValue(decoded, map[string]string{}, 0, "")
}

var (
	sensitiveKey = regexp.MustCompile(`(?i)(token|secret|password|authorization|cookie|credential|capability|pairing.?code|private.?key|api.?key|access.?key)`)
	textKey      = regexp.MustCompile(`(?i)^(body|content|prompt|question|text|transcript|raw|preview|spec|subject|comment|summary|reportpath|error|message|name|title|displayname)$`)
	idKey        = regexp.MustCompile(`(?i)(^id$|ids?$|handle$|cursor$|environment$|repository$|repo$|hostname$|runtimeid$|dispatch$|task$|worktree$|identity$|branch$|revision$|commit$|head$|base$)`)
	pathKey      = regexp.MustCompile(`(?i)(^path$|path$|directory$|cwd$|filepath$)`)
	safeValueKey = regexp.MustCompile(`(?i)^(ok|status|state|kind|type|outcome|phase|stage|source|connectionstate|version|runtimeversion|appversion|platform|protocolversion|schemaversion|exitcode|code|resultcode)$`)
	safeAtom     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/+-]{0,127}$`)
	absolutePath = regexp.MustCompile(`(?i)([a-z]:\\[^\s"']+|/(?:Users|home|private|tmp)/[^\s"']+)`)
	urlPattern   = regexp.MustCompile(`(?i)https?://[^\s"']+|orca://[^\s"']+`)
)

func sanitizeValue(value any, ids map[string]string, depth int, key string) any {
	if depth > 40 {
		return "[depth-limited]"
	}
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := v[key]
			switch {
			case sensitiveKey.MatchString(key):
				result[key] = "[REDACTED]"
			case strings.EqualFold(key, "payload"):
				if raw, ok := child.(string); ok {
					var payload any
					if json.Unmarshal([]byte(raw), &payload) == nil {
						clean := sanitizeValue(payload, ids, depth+1, key)
						if encoded, err := json.Marshal(clean); err == nil {
							result[key] = string(encoded)
							break
						}
					}
					result[key] = "[NON-JSON PAYLOAD REDACTED]"
				} else {
					result[key] = sanitizeValue(child, ids, depth+1, key)
				}
			case pathKey.MatchString(key):
				result[key] = pseudonym(child, "PATH", ids)
			case textKey.MatchString(key):
				result[key] = "[TEXT REDACTED]"
			case idKey.MatchString(key):
				switch child.(type) {
				case []any, map[string]any:
					result[key] = sanitizeValue(child, ids, depth+1, key)
				default:
					result[key] = pseudonym(child, "ID", ids)
				}
			default:
				result[key] = sanitizeValue(child, ids, depth+1, key)
			}
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i := range v {
			if idKey.MatchString(key) {
				result[i] = pseudonym(v[i], "ID", ids)
			} else {
				result[i] = sanitizeValue(v[i], ids, depth+1, key)
			}
		}
		return result
	case string:
		clean := urlPattern.ReplaceAllString(v, "[URL REDACTED]")
		clean = absolutePath.ReplaceAllString(clean, "[PATH REDACTED]")
		if safeValueKey.MatchString(key) || (key == "capabilities" || key == "flags" || key == "sentinel") && safeAtom.MatchString(clean) {
			return clean
		}
		lowerKey := strings.ToLower(key)
		if timestamp, err := time.Parse(time.RFC3339Nano, clean); err == nil && (strings.Contains(lowerKey, "time") || strings.HasSuffix(lowerKey, "at")) {
			return timestamp.UTC().Format(time.RFC3339Nano)
		}
		return "[TEXT REDACTED]"
	default:
		return value
	}
}

func pseudonym(value any, prefix string, ids map[string]string) any {
	s := fmt.Sprint(value)
	if s == "" || s == "<nil>" || s == "null" {
		return value
	}
	for _, address := range []string{"dispatch:", "run:"} {
		if strings.HasPrefix(s, address) {
			return address + fmt.Sprint(pseudonym(strings.TrimPrefix(s, address), prefix, ids))
		}
	}
	if mapped, ok := ids[s]; ok {
		return mapped
	}
	mapped := fmt.Sprintf("<%s-%03d>", prefix, len(ids)+1)
	ids[s] = mapped
	return mapped
}

func sanitizeFile(args []string) error {
	fs := flag.NewFlagSet("sanitize", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	in := fs.String("in", "", "input JSON evidence file")
	out := fs.String("out", "", "write sanitized JSON here; stdout when omitted")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *in == "" {
		return errors.New("sanitize requires -in and accepts no positional arguments")
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	var input any
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("input is not valid JSON: %w", err)
	}
	return writeJSON(*out, sanitizeValue(input, map[string]string{}, 0, ""))
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	tmp := abs + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
