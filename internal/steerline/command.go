package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ServiceCommandPayload struct {
	ServiceInstanceID string          `json:"service_instance_id"`
	ExpectedRevision  uint64          `json:"expected_revision"`
	RunAttemptID      string          `json:"run_attempt_id,omitempty"`
	Body              json.RawMessage `json:"body"`
}

// SubmitServiceCommand targets the ready service's SQLite command queue. It
// binds the command to the displayed service instance and run/control revision.
func SubmitServiceCommand(ctx context.Context, c Config, kind, targetID, requestedBy, commandID string, expectedRevision uint64, body any) (CommandRecord, bool, error) {
	if !validServiceCommandKind(kind) || targetID == "" || requestedBy == "" || !controlIDPattern.MatchString(commandID) || expectedRevision == 0 {
		return CommandRecord{}, false, errors.New("service command identity, target, requester and expected revision are required")
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return CommandRecord{}, false, err
	}
	connection := ConnectService(ctx, c)
	if !connection.CanControl || connection.Snapshot.Runtime == nil {
		diagnostic := connection.Diagnostic
		if diagnostic == "" {
			diagnostic = string(connection.State)
		}
		return CommandRecord{}, false, fmt.Errorf("service is offline or not ready for commands: %s", diagnostic)
	}
	connected := connection.Snapshot.Runtime
	runtimeState, err := LoadDatabaseRuntime(c)
	if err != nil {
		return CommandRecord{}, false, err
	}
	if runtimeState == nil || runtimeState.Service.Status != "running" || !runtimeState.Service.Ready || runtimeState.Service.InstanceID != connected.Service.InstanceID {
		return CommandRecord{}, false, errors.New("stale_instance: SQLite projection does not match the ready service")
	}
	if !serviceHasCapability(runtimeState, kind) {
		return CommandRecord{}, false, fmt.Errorf("service does not advertise %s capability", kind)
	}
	targetAttemptID := ""
	if kind == "resolve_command" {
		if runtimeState.Control.Revision != expectedRevision {
			return CommandRecord{}, false, errors.New("stale_revision: control blockers changed; refresh before resolving")
		}
		found := false
		for _, blocker := range runtimeState.Control.Blockers {
			if blocker.RequestID == targetID {
				found = true
				break
			}
		}
		if !found {
			return CommandRecord{}, false, errors.New("control blocker is no longer present")
		}
	} else {
		var run *Run
		for _, candidate := range runtimeState.Runs {
			if candidate != nil && candidate.RunID == targetID {
				run = candidate
				break
			}
		}
		if run == nil {
			return CommandRecord{}, false, errors.New("target run was not found")
		}
		if run.ControlRevision != expectedRevision {
			return CommandRecord{}, false, errors.New("stale_revision: run changed; refresh before issuing the recovery command")
		}
		targetAttemptID = run.AttemptID
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return CommandRecord{}, false, err
	}
	payload, err := json.Marshal(ServiceCommandPayload{ServiceInstanceID: runtimeState.Service.InstanceID, ExpectedRevision: expectedRevision, RunAttemptID: targetAttemptID, Body: bodyBytes})
	if err != nil {
		return CommandRecord{}, false, err
	}
	if len(payload) > maxControlRequestBytes {
		return CommandRecord{}, false, errors.New("service command exceeds the 3 MiB protocol limit")
	}
	db, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return CommandRecord{}, false, err
	}
	defer db.Close()
	return (CommandService{Store: db, Project: c.Identity()}).Submit(ctx, CommandRecord{CommandID: commandID, Kind: kind, TargetID: targetID, ExpectedVersion: expectedRevision, IdempotencyKey: commandID, RequestedBy: requestedBy, Payload: payload, Status: "pending", CreatedAt: now()})
}

func validServiceCommandKind(kind string) bool {
	switch kind {
	case "retry_run", "mark_stopped", "resolve_command":
		return true
	default:
		return false
	}
}

type Runner struct {
	Dir     string
	Timeout time.Duration
	Env     map[string]string
	Input   []byte
}
type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, fmt.Errorf("command output exceeds %d bytes", b.max)
	}
	return b.Buffer.Write(p)
}
func (r Runner) Output(ctx context.Context, command Command, args ...string) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.Command(command.Executable, append(append([]string{}, command.Args...), args...)...)
	cmd.Dir = r.Dir
	if r.Input != nil {
		cmd.Stdin = bytes.NewReader(r.Input)
	}
	if len(r.Env) > 0 {
		cmd.Env = overrideEnv(cmd.Environ(), r.Env)
	}
	var stdout = limitedBuffer{max: 16 << 20}
	var stderr = limitedBuffer{max: 1 << 20}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	p, e := startProcess(cmd)
	if e != nil {
		return nil, e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case e = <-done:
	case <-ctx.Done():
		p.Kill()
		<-done
		e = ctx.Err()
	}
	p.Close()
	if e != nil {
		return stdout.Bytes(), fmt.Errorf("%s failed: %w (%s)", command.Executable, e, clip(stderr.String(), 1000))
	}
	return stdout.Bytes(), nil
}

func overrideEnv(base []string, values map[string]string) []string {
	out := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		replace := false
		for name := range values {
			if strings.EqualFold(key, name) {
				replace = true
				break
			}
		}
		if !replace {
			out = append(out, entry)
		}
	}
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func unsetEnv(base []string, names []string) []string {
	if len(names) == 0 {
		return base
	}
	blocked := make(map[string]bool, len(names))
	prefixes := []string{}
	for _, name := range names {
		upper := strings.ToUpper(name)
		if strings.HasSuffix(upper, "*") {
			prefixes = append(prefixes, strings.TrimSuffix(upper, "*"))
		} else {
			blocked[upper] = true
		}
	}
	out := make([]string, 0, len(base))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		upperKey := strings.ToUpper(key)
		remove := blocked[upperKey]
		for _, prefix := range prefixes {
			if strings.HasPrefix(upperKey, prefix) {
				remove = true
				break
			}
		}
		if !remove {
			out = append(out, entry)
		}
	}
	return out
}
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
