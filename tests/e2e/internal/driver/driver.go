// Package driver uses product command and read contracts, never SQL mutations.
package driver

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/google/uuid"
)

type Client struct {
	Binary, Config, Home, StatePath, LogDir, WorkingDirectory string
	Profile, NativeHome                                       string
	sequence                                                  int
}

// Environment replaces project-owned configuration and disables inherited test switches.
func Environment(base []string, home, state string) []string {
	result := make([]string, 0, len(base)+6)
	var pairing struct{ Pairing string }
	if raw, err := os.ReadFile(filepath.Join(home, "orca-state-fixture.json")); err == nil {
		_ = json.Unmarshal(raw, &pairing)
	}
	for _, v := range base {
		key := strings.ToUpper(strings.SplitN(v, "=", 2)[0])
		if pairing.Pairing != "" && (key == "ORCA_PAIRING_CODE" || key == "ORCA_REMOTE_PAIRING" || key == "ORCA_USER_DATA_PATH") {
			continue
		}
		if strings.HasPrefix(key, "STEERLINE_") || key == "TEMP" || key == "TMP" || key == "CODEX_HOME" || strings.HasPrefix(key, "GIT_") || key == "GOFLAGS" || key == "HTTP_PROXY" || key == "HTTPS_PROXY" || key == "ALL_PROXY" || key == "NO_PROXY" {
			continue
		}
		result = append(result, v)
	}
	if pairing.Pairing != "" {
		result = append(result, "ORCA_PAIRING_CODE="+pairing.Pairing, "ORCA_USER_DATA_PATH="+home)
	}
	temp := filepath.Join(home, "temp")

	// The independent Runner invokes real Git. Redirect only this controlled
	// fixture's public repository identity to its owned loopback Git server.
	var fixture struct {
		GitURL     string `json:"git_url"`
		Repository string `json:"repository"`
	}
	if raw, err := os.ReadFile(state); err == nil && json.Unmarshal(raw, &fixture) == nil && strings.HasPrefix(fixture.GitURL, "http://127.0.0.1:") {
		repository := fixture.Repository
		if repository == "" {
			if i := strings.Index(fixture.GitURL[7:], "/"); i >= 0 {
				repository = strings.TrimSuffix(fixture.GitURL[7+i+1:], ".git")
			}
		}
		if len(strings.Split(repository, "/")) == 2 {
			result = append(result, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=url."+fixture.GitURL+".insteadOf", "GIT_CONFIG_VALUE_0=https://github.com/"+repository+".git")
		}
	}
	return append(result, "STEERLINE_HOME="+home, "STEERLINE_E2E_CONTROLLED_STATE="+state, "CODEX_HOME="+filepath.Join(home, "codex"), "TEMP="+temp, "TMP="+temp, "GOFLAGS=", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0", "NO_PROXY=127.0.0.1,localhost")
}

func (c *Client) Command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	cmd.Env = c.Environment(os.Environ())
	cmd.Dir = c.WorkingDirectory
	return cmd
}

func (c *Client) Run(ctx context.Context, args ...string) ([]byte, error) {
	return c.run(ctx, c.Binary, args...)
}

// RunHelper executes a native fixture control with the same isolated environment.
func (c *Client) RunHelper(ctx context.Context, helper string, args ...string) ([]byte, error) {
	return c.run(ctx, helper, args...)
}

func (c *Client) run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	timeout := 135 * time.Second
	if len(args) > 0 && args[0] == "start" {
		timeout = 190 * time.Second
	}
	if len(args) >= 2 && args[0] == "task" && (args[1] == "validate" || args[1] == "create") {
		// These commands use the ready project actor and validate real Git
		// contracts. Leave room for the existing product command budget.
		timeout = 2 * time.Minute
	}
	return c.runWithTimeout(ctx, timeout, binary, args...)
}

func (c *Client) runWithTimeout(ctx context.Context, timeout time.Duration, binary string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.Command(binary, args...)
	cmd.Env = c.Environment(os.Environ())
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

// MainControl invokes the same application client used by the CLI.
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
	if *expectedStatus != "applied" && !(*action == "start_task" && (*expectedStatus == "failed" || *expectedStatus == "indeterminate")) {
		return fmt.Errorf("unsupported E2E control status expectation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	product, err := steerlineapp.OpenApplicationProject(ctx, "", *config)
	if err != nil {
		return err
	}
	defer product.Close()
	var payload productapi.ControlPayload
	if *action == "start_task" {
		payload, err = product.StartTaskPayload(ctx, *taskID, *profileID)
		if err != nil {
			return err
		}
	}
	request, err := product.SubmitControl(ctx, productapi.ControlInput{RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""), Action: productapi.ControlAction(*action), Payload: payload, RequestedBy: "e2e", InstanceID: *expectedInstance})
	if err != nil {
		return err
	}
	// Request expiration guards admission in the product. A verified applying
	// reservation is already durable; completion can arrive after that deadline.
	receipt, err := product.WaitControl(ctx, request.RequestID, 100*time.Millisecond)
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

func ReadWorkflow(ctx context.Context, database, project string) (storageports.RunnerSnapshot, error) {
	reader, err := sqlite.OpenSQLiteReader(database)
	if err != nil {
		return storageports.RunnerSnapshot{}, err
	}
	defer reader.Close()
	return reader.ReadRunnerSnapshot(ctx, project)
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

// HostStatus observes only the product's home-scoped CLI status contract.
type HostStatus struct {
	Backend struct {
		State    string `json:"state"`
		Identity struct {
			InstanceID string `json:"instance_id"`
			Process    struct {
				PID int `json:"pid"`
			} `json:"process"`
		} `json:"identity"`
	} `json:"backend"`
	Projects []struct {
		ConfigID     string `json:"config_id"`
		ProjectID    string `json:"project_id"`
		RuntimeState string `json:"runtime_state"`
		Online       bool   `json:"online"`
		Diagnostic   string `json:"diagnostic"`
	} `json:"projects"`
}

func (c *Client) HostStatus(ctx context.Context) (status HostStatus, err error) {
	raw, err := c.Run(ctx, "status", "--json")
	if err != nil {
		return status, err
	}
	err = json.Unmarshal(raw, &status)
	return
}
