// Package driver uses product command and read contracts, never SQL mutations.
package driver

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	executionreconstruction "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/google/uuid"
)

type Client struct {
	Binary, Config, Home, StatePath, LogDir, WorkingDirectory string
	Profile                                                   string
	sequence                                                  int
}

// Environment replaces project-owned configuration and disables inherited test switches.
func Environment(base []string, home, state string) []string {
	result := make([]string, 0, len(base)+6)
	for _, v := range base {
		key := strings.ToUpper(strings.SplitN(v, "=", 2)[0])
		if strings.HasPrefix(key, "STEERLINE_") || key == "TEMP" || key == "TMP" || key == "CODEX_HOME" || strings.HasPrefix(key, "GIT_") || key == "GOFLAGS" || key == "HTTP_PROXY" || key == "HTTPS_PROXY" || key == "ALL_PROXY" || key == "NO_PROXY" {
			continue
		}
		result = append(result, v)
	}
	temp := filepath.Join(home, "temp")
	if token, err := os.ReadFile(filepath.Join(home, "github-token")); err == nil {
		// Project credential contents never enter command/evidence records.
		result = append(result, "GH_TOKEN="+strings.TrimSpace(string(token)))
	}
	return append(result, "STEERLINE_HOME="+home, "STEERLINE_E2E_CONTROLLED_STATE="+state, "CODEX_HOME="+filepath.Join(home, "codex"), "TEMP="+temp, "TMP="+temp, "GOFLAGS=", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0", "NO_PROXY=127.0.0.1,localhost")
}

func (c *Client) Command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	cmd.Env = Environment(os.Environ(), c.Home, c.StatePath)
	cmd.Dir = c.WorkingDirectory
	return cmd
}

func (c *Client) Run(ctx context.Context, args ...string) ([]byte, error) {
	return c.run(ctx, c.Binary, args...)
}

func (c *Client) run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	return c.runWithTimeout(ctx, 30*time.Second, binary, args...)
}

func (c *Client) runWithTimeout(ctx context.Context, timeout time.Duration, binary string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.Command(binary, args...)
	cmd.Env = Environment(os.Environ(), c.Home, c.StatePath)
	cmd.Dir = c.WorkingDirectory
	var out, stderr strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := testprocess.Run(ctx, cmd)
	c.sequence++
	if c.LogDir != "" {
		record := map[string]any{"command": append([]string{binary}, args...), "stdout": out.String(), "stderr": stderr.String(), "error": fmt.Sprint(err)}
		raw, encodeErr := json.MarshalIndent(record, "", "  ")
		if encodeErr != nil {
			return nil, encodeErr
		}
		if logErr := os.WriteFile(filepath.Join(c.LogDir, fmt.Sprintf("command-%04d.json", c.sequence)), raw, 0600); logErr != nil {
			return nil, logErr
		}
	}
	if err != nil {
		return []byte(out.String()), fmt.Errorf("CLI %v: %w: %s%s", args, err, out.String(), stderr.String())
	}
	return []byte(out.String()), nil
}

func (c *Client) Control(ctx context.Context, helper, action, taskID string) error {
	return c.ControlExpectedStatus(ctx, helper, action, taskID, "applied")
}

func (c *Client) ControlExpectedStatus(ctx context.Context, helper, action, taskID, expectedStatus string) error {
	profile := c.Profile
	if profile == "" {
		profile = "controlled"
	}
	// Admission expires on the original product request. An admitted claim can
	// still be applying native preparation; await that same receipt without resend.
	_, err := c.runWithTimeout(ctx, 390*time.Second, helper, "control", "-config", c.Config, "-action", action, "-task-id", taskID, "-profile", profile, "-expected-status", expectedStatus)
	return err
}

func (c *Client) ControlTargeted(ctx context.Context, helper, action, taskID, expectedInstance string) error {
	profile := c.Profile
	if profile == "" {
		profile = "controlled"
	}
	_, err := c.runWithTimeout(ctx, 390*time.Second, helper, "control", "-config", c.Config, "-action", action, "-task-id", taskID,
		"-profile", profile, "-expected-instance", expectedInstance)
	return err
}

// MainControl invokes the same exported control client used by the console.
// An isolated helper process scopes LoadConfig's process environment.
func MainControl(args []string) error {
	fs := flag.NewFlagSet("E2E control", flag.ContinueOnError)
	config := fs.String("config", "", "project config")
	action := fs.String("action", "", "control action")
	taskID := fs.String("task-id", "", "task ID")
	profileID := fs.String("profile", "controlled", "execution profile ID")
	expectedInstance := fs.String("expected-instance", "", "displayed service instance ID; rejects stale targets")
	expectedStatus := fs.String("expected-status", "applied", "required product receipt status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *action != "resume_dispatch" && *action != "start_task" {
		return fmt.Errorf("unsupported E2E control action")
	}
	if *expectedStatus != "applied" && !(*action == "start_task" && *expectedStatus == "failed") {
		return fmt.Errorf("unsupported E2E control status expectation")
	}
	c, err := steerlineapp.LoadConfig(*config)
	if err != nil {
		return err
	}
	hostConfig, err := steerlineapp.BuildHostConfig(context.Background(), c)
	if err != nil {
		return err
	}
	identityCtx, identityCancel := context.WithTimeout(context.Background(), 25*time.Second)
	err = c.ResolveIdentity(identityCtx)
	identityCancel()
	if err != nil {
		return err
	}
	var payload any
	if *action == "start_task" {
		_, profile, err := c.ResolveExecutionProfile(*profileID, "")
		if err != nil {
			return err
		}
		payload = productapi.ControlPayload{TaskID: *taskID, ExecutionProfileID: profile.ID, ExecutionProfileDigest: profile.Digest()}
	}
	var request productapi.ControlRequest
	if *expectedInstance == "" {
		request, err = steerline.SubmitControl(hostConfig, *action, payload, "e2e")
	} else {
		requestID := strings.ReplaceAll(uuid.NewString(), "-", "")
		request, err = steerline.SubmitControlTargeted(hostConfig, *action, payload, "e2e", requestID, *expectedInstance, nil)
	}
	if err != nil {
		return err
	}
	// Request expiration guards admission in the product. A verified applying
	// reservation is already durable; completion can arrive after that deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	receipt, err := steerline.WaitControlReceipt(ctx, hostConfig, request.RequestID, 100*time.Millisecond)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(receipt); err != nil {
		return err
	}
	if receipt.Status != *expectedStatus {
		return fmt.Errorf("control %s: %s: %s", receipt.Status, receipt.Code, receipt.Message)
	}
	return nil
}

func (c *Client) Tasks(ctx context.Context) (storageports.NativeTaskPage, error) {
	var page storageports.NativeTaskPage
	raw, err := c.Run(ctx, "task", "list", "-config", c.Config)
	if err == nil {
		err = json.Unmarshal(raw, &page)
	}
	return page, err
}

func ReadWorkflow(ctx context.Context, database, project string) (executionreconstruction.Snapshot, error) {
	reader, err := sqlite.OpenSQLiteReader(database)
	if err != nil {
		return executionreconstruction.Snapshot{}, err
	}
	defer reader.Close()
	snapshot, err := reader.ReadExecutionStateSnapshot(ctx, project)
	if err != nil {
		return executionreconstruction.Snapshot{}, err
	}
	return executionreconstruction.SnapshotFromStorage(snapshot), nil
}

// ReadProjectView uses the product reader's public transaction contract.
func ReadProjectView(ctx context.Context, database, project string) (storageports.ProjectView, error) {
	reader, err := sqlite.OpenSQLiteReader(database)
	if err != nil {
		return storageports.ProjectView{}, err
	}
	defer reader.Close()
	return productquery.NewService(reader, project).ReadProjectView(ctx, "", 100)
}

// ReadApplicationProjectSnapshot obtains the product view and execution facts
// used by SnapshotContext from one SQLite read transaction.
func ReadApplicationProjectSnapshot(ctx context.Context, database, project, selected string, limit int) (storageports.ApplicationProjectSnapshot, error) {
	reader, err := sqlite.OpenSQLiteReader(database)
	if err != nil {
		return storageports.ApplicationProjectSnapshot{}, err
	}
	defer reader.Close()
	return reader.ReadApplicationProjectSnapshot(ctx, project, selected, limit)
}

// MainView exposes the same public read transaction for fixture diagnostics.
func MainView(args []string) error {
	fs := flag.NewFlagSet("E2E view", flag.ContinueOnError)
	config := fs.String("config", "", "project config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := steerlineapp.LoadConfig(*config)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := c.ResolveIdentity(ctx); err != nil {
		return err
	}
	reader, err := steerlineapp.OpenNativeTaskReader(ctx, &c)
	if err != nil {
		return err
	}
	defer reader.Close()
	view, err := productquery.NewService(reader, c.Identity()).ReadProjectView(ctx, "", 100)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(view)
}
