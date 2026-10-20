// Package runnerclient consumes only the steerrunner-cli/v1 process contract.
// It never imports Runner Go packages, opens its database or uses its IPC.
package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const Protocol = "steerrunner-cli/v1"
const MaxRequestBytes = 8 << 20
const maxResponseBytes = 32 << 20

type Request = runnercontract.Request

type RemoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Response struct {
	Protocol  string          `json:"protocol"`
	OK        bool            `json:"ok"`
	RequestID *string         `json:"request_id"`
	Data      json.RawMessage `json:"data"`
	Error     *RemoteError    `json:"error"`
}
type Failure struct {
	Code    string
	Message string
	Exit    int
	Unknown bool
}

func (f *Failure) Error() string { return f.Code + ": " + f.Message }

type Client struct {
	Takeover              *runnercontract.TakeoverReport
	InstanceID            string
	BuildVersion          string
	ResourceDigest        string
	NativeSessionAdapters []string
	Executable            string
	StateDir              string
	Timeout               time.Duration
	// SHA256 binds every invocation to the selected release, including queries.
	SHA256 string
}
type bounded struct {
	bytes.Buffer
	max int
}

func (b *bounded) Write(value []byte) (int, error) {
	if b.Len()+len(value) > b.max {
		return 0, errors.New("CLI output limit exceeded")
	}
	return b.Buffer.Write(value)
}
func mutation(command string) bool {
	switch command {
	case "submit", "reply", "resume", "stop", "service stop", "environment-check start", "environment-check stop", "artifacts collect":
		return true
	}
	return false
}
func (c Client) Call(ctx context.Context, command string, args []string, input *Request) (Response, error) {
	return c.call(ctx, command, args, input, nil)
}

func (c Client) call(ctx context.Context, command string, args []string, input *Request, startup *runnercontract.ServiceStartInput) (Response, error) {
	if c.InstanceID != "" && command != "version" && command != "service status" && command != "service stop" && command != "service verify-stopped" {
		if err := c.ValidateInstance(ctx); err != nil {
			return Response{}, err
		}
	}
	if c.Executable == "" {
		return Response{}, &Failure{Code: "runner_unavailable", Message: "SteerRunner executable is not configured", Exit: 4}
	}
	if c.SHA256 != "" {
		if err := VerifyExecutable(c.Executable, c.SHA256); err != nil {
			return Response{}, &Failure{Code: "runner_unavailable", Message: err.Error(), Exit: 4}
		}
	}
	argv := []string{}
	switch command {
	case "artifacts discover", "artifacts collect", "artifacts list", "artifacts read":
		argv = append(argv, "artifacts", command[len("artifacts "):])
	case "environment-check start", "environment-check inspect", "environment-check stop":
		argv = append(argv, "environment-check", command[len("environment-check "):])
	case "service start", "service status", "service stop", "service verify-stopped", "service verify-takeover":
		argv = append(argv, "service", command[len("service "):])
	case "version", "capabilities", "submit", "inspect", "last-message", "open-native-session", "events", "reply", "resume", "stop", "workspace-git":
		argv = append(argv, command)
	default:
		return Response{}, &Failure{Code: "invalid_arguments", Message: "Unsupported Runner CLI command", Exit: 2}
	}
	if c.InstanceID != "" && command != "version" && command != "service start" && command != "service status" && command != "service stop" && command != "service verify-stopped" {
		argv = append(argv, "--expected-instance-id", c.InstanceID)
	}
	argv = append(argv, args...)
	argv = append(argv, "--json", "--protocol", Protocol)
	if command != "version" {
		argv = append(argv, "--state-dir", c.StateDir)
	}
	var body []byte
	if startup != nil {
		if command != "service start" || input != nil {
			return Response{}, errors.New("startup stops require service start")
		}
		if err := startup.Validate(); err != nil {
			return Response{}, err
		}
		body, _ = json.Marshal(startup)
		if len(body) > MaxRequestBytes {
			return Response{}, errors.New("startup stops exceed Runner request bound")
		}
		argv = append(argv, "--input", "-")
	} else if input != nil {
		if !mutation(command) || input.Protocol != Protocol || input.RequestID == "" || !utf8.Valid(input.Input) || !json.Valid(input.Input) {
			return Response{}, &Failure{Code: "invalid_request", Message: "A fixed v1 request envelope is required", Exit: 2}
		}
		var err error
		body, err = json.Marshal(input)
		if err != nil || len(body) > MaxRequestBytes {
			return Response{}, &Failure{Code: "invalid_request", Message: "Runner request exceeds the CLI contract", Exit: 2}
		}
		argv = append(argv, "--input", "-")
	} else if mutation(command) {
		return Response{}, &Failure{Code: "invalid_request", Message: "A durable request ID and input are required", Exit: 2}
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	process := exec.CommandContext(ctx, c.Executable, argv...)
	hide(process)
	process.Stdin = bytes.NewReader(body)
	output, diagnostic := &bounded{max: maxResponseBytes}, &bounded{max: 64 << 10}
	process.Stdout, process.Stderr = output, diagnostic
	process.WaitDelay = time.Second
	if err := process.Start(); err != nil {
		return Response{}, &Failure{Code: "runner_unavailable", Message: "Runner CLI could not be started", Exit: 4}
	}
	err := process.Wait()
	exit := 0
	if failure, ok := err.(*exec.ExitError); ok {
		exit = failure.ExitCode()
	} else if err != nil {
		exit = -1
	}
	unknown := func() (Response, error) {
		if command == "open-native-session" {
			return Response{}, &Failure{Code: "request_outcome_unknown", Message: "Native viewer may have opened; do not replay the operation", Exit: 6, Unknown: true}
		}
		if mutation(command) || startup != nil {
			if command == "environment-check start" || command == "environment-check stop" {
				return Response{}, &Failure{Code: "request_outcome_unknown", Message: "Inspect the original environment-check request ID; do not replay it", Exit: 6, Unknown: true}
			}
			return Response{}, &Failure{Code: "request_outcome_unknown", Message: "Inspect or retry the original request ID and unchanged input", Exit: 6, Unknown: true}
		}
		return Response{}, &Failure{Code: "runner_unavailable", Message: "Runner query response is unavailable", Exit: 4}
	}
	if ctx.Err() != nil || !utf8.Valid(output.Bytes()) || exit < 0 {
		return unknown()
	}
	var response Response
	if err := decodeEnvelope(output.Bytes(), &response); err != nil || response.Protocol != Protocol || response.OK == (response.Error != nil) || response.OK != (exit == 0) {
		return unknown()
	}
	startupFailureID := false
	if startup != nil && !response.OK && response.RequestID != nil {
		for _, stop := range startup.PendingStops {
			if stop.Request.RequestID == *response.RequestID {
				startupFailureID = true
			}
		}
	}
	if input == nil && response.RequestID != nil && !startupFailureID || input != nil && (response.RequestID == nil || *response.RequestID != input.RequestID) {
		return unknown()
	}
	if !response.OK {
		expected := exitCode(response.Error.Code)
		if expected != exit {
			return unknown()
		}
		return response, &Failure{Code: response.Error.Code, Message: response.Error.Message, Exit: exit, Unknown: exit == 6}
	}
	return response, nil
}
func decodeEnvelope(raw []byte, target *Response) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("one envelope required")
	}
	fields := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || fields[name] {
			return errors.New("ambiguous envelope")
		}
		switch name {
		case "protocol", "ok", "request_id", "data", "error":
		default:
			return errors.New("unknown envelope field")
		}
		fields[name] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		switch name {
		case "protocol":
			var v string
			if bytes.Equal(value, []byte("null")) || json.Unmarshal(value, &v) != nil {
				return errors.New("invalid protocol")
			}
		case "ok":
			if !bytes.Equal(value, []byte("true")) && !bytes.Equal(value, []byte("false")) {
				return errors.New("invalid acceptance flag")
			}
		case "request_id":
			var v *string
			if json.Unmarshal(value, &v) != nil {
				return errors.New("invalid request identity")
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing CLI output")
	}
	if len(fields) != 5 {
		return errors.New("incomplete envelope")
	}
	return json.Unmarshal(raw, target)
}
func exitCode(code string) int {
	switch code {
	case "artifact_path_invalid", "artifact_limit_exceeded":
		return 2
	case "invalid_arguments", "invalid_request", "invalid_input", "invalid_cursor":
		return 2
	case "unsupported_protocol", "unsupported_schema", "unsupported_capability", "unsupported_platform":
		return 3
	case "workspace_unavailable", "artifact_unavailable", "service_unavailable", "service_start_failed", "detach_unsupported", "startup_unverified", "environment_unavailable":
		return 4
	case "request_conflict", "request_not_found", "command_not_found", "run_not_found", "check_not_found", "stale_instance", "directory_conflict", "stale_revision", "invalid_state", "stale_question", "resume_required", "operation_unverified", "answer_pending", "repository_conflict":
		return 5
	case "artifact_not_found", "artifact_changed", "artifact_integrity_failed", "workspace_source_unknown":
		return 5
	case "request_outcome_unknown":
		return 6
	}
	return 1
}
func (c Client) Version(ctx context.Context) (Version, error) {
	response, err := c.Call(ctx, "version", nil, nil)
	if err != nil {
		return Version{}, err
	}
	var version Version
	if json.Unmarshal(response.Data, &version) != nil {
		return version, fmt.Errorf("invalid Runner version response")
	}
	for _, protocol := range version.Protocols {
		if protocol == Protocol {
			return version, nil
		}
	}
	return version, &Failure{Code: "unsupported_protocol", Message: "Runner does not support the required CLI protocol", Exit: 3}
}

type Version struct {
	Version         string   `json:"version"`
	SourceCommit    string   `json:"source_commit"`
	Protocols       []string `json:"protocols"`
	ResourceVersion int      `json:"resource_version"`
	ResourceDigest  string   `json:"resource_digest"`
	Platform        string   `json:"platform"`
}
type Receipt struct {
	CommandID     string `json:"command_id"`
	RunID         string `json:"run_id"`
	ReceiptStatus string `json:"receipt_status"`
	ActionStatus  string `json:"action_status"`
}

func (c Client) Submit(ctx context.Context, request Request) (Receipt, error) {
	response, err := c.Call(ctx, "submit", nil, &request)
	if err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	if json.Unmarshal(response.Data, &receipt) != nil || receipt.RunID == "" || receipt.CommandID == "" || receipt.ReceiptStatus != "accepted" {
		return receipt, &Failure{Code: "request_outcome_unknown", Message: "Submission receipt could not be verified; inspect original request ID", Exit: 6, Unknown: true}
	}
	return receipt, nil
}
