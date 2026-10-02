// Package driver uses product command and read contracts, never SQL mutations.
package driver

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
	"github.com/erranliu/SteerLine/internal/testprocess"
)

type Client struct {
	Binary, Config, Home, StatePath, LogDir string
	Profile                                 string
	sequence                                int
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
	profile := c.Profile
	if profile == "" {
		profile = "controlled"
	}
	// ResolveIdentity is bounded at 25s; the product control request has a 2m
	// lifetime. Native preparation must get that same receipt window, without
	// submitting another request when a terminal is still starting.
	_, err := c.runWithTimeout(ctx, 150*time.Second, helper, "control", "-config", c.Config, "-action", action, "-task-id", taskID, "-profile", profile)
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *action != "resume_dispatch" && *action != "start_task" {
		return fmt.Errorf("unsupported E2E control action")
	}
	c, err := steerlineapp.LoadConfig(*config)
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
		payload = steerline.ControlPayload{TaskID: *taskID, ExecutionProfileID: profile.ID, ExecutionProfileDigest: profile.Digest()}
	}
	request, err := steerline.SubmitControl(c, *action, payload, "e2e")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(context.Background(), request.ExpiresAt)
	defer cancel()
	receipt, err := steerline.WaitControlReceipt(ctx, c, request.RequestID, 100*time.Millisecond)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(receipt); err != nil {
		return err
	}
	if receipt.Status != "applied" {
		return fmt.Errorf("control %s: %s: %s", receipt.Status, receipt.Code, receipt.Message)
	}
	return nil
}

func (c *Client) Tasks(ctx context.Context) (contracts.NativeTaskPage, error) {
	var page contracts.NativeTaskPage
	raw, err := c.Run(ctx, "task", "list", "-config", c.Config)
	if err == nil {
		err = json.Unmarshal(raw, &page)
	}
	return page, err
}

func ReadWorkflow(ctx context.Context, database, project string) (contracts.DomainSnapshot, error) {
	reader, err := steerline.OpenSQLiteReader(database)
	if err != nil {
		return contracts.DomainSnapshot{}, err
	}
	defer reader.Close()
	return steerline.NewQueryService(reader, project).Workflow(ctx, "")
}

// ReadProjectView uses the product reader's public transaction contract.
func ReadProjectView(ctx context.Context, database, project string) (contracts.ProjectView, error) {
	reader, err := steerline.OpenSQLiteReader(database)
	if err != nil {
		return contracts.ProjectView{}, err
	}
	defer reader.Close()
	return steerline.NewQueryService(reader, project).ReadProjectView(ctx, "", 100)
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
	reader, err := steerline.OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return err
	}
	defer reader.Close()
	view, err := steerline.NewQueryService(reader, c.Identity()).ReadProjectView(ctx, "", 100)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(view)
}
