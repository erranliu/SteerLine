package driver

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	app "github.com/erranliu/SteerLine/internal/app"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

// CleanupRunner first fences the original service (including any in-flight
// submit), then explicitly opens a cleanup epoch when original Runs need stop.
// It never resumes business input, reads private storage or guesses a PID.
func CleanupRunner(ctx context.Context, executionConfig, stateDir string, candidates []runnerstate.Candidate) error {
	var config struct {
		Runner runnerclient.Selection `json:"runner"`
	}
	raw, err := os.ReadFile(executionConfig)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		return err
	}
	cli, version, err := config.Runner.Open(ctx, stateDir, 10*time.Second)
	if err != nil {
		return err
	}
	status, err := cli.ServiceStatus(ctx)
	if err != nil {
		return err
	}
	if status.Status == "ready" {
		if err = cli.StopService(ctx, status.InstanceID, "fixture-fence-"+status.InstanceID); err != nil {
			return err
		}
	} else if status.Status != "not_running" {
		return errors.New("original Runner service absence is unverified")
	}
	needsInspection := false
	for _, c := range candidates {
		needsInspection = needsInspection || c.RunID != "" && (c.Projection == nil || c.Projection.StopStatus != "stopped") || c.SubmitState == "unknown"
	}
	if !needsInspection {
		return nil
	}
	service, err := cli.StartService(ctx, version)
	if err != nil {
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = cli.StopService(stopCtx, service.InstanceID, "fixture-close-"+service.InstanceID)
	}()
	for _, c := range candidates {
		if c.RunID == "" && c.SubmitState != "unknown" {
			continue
		}
		selector, id := "run-id", c.RunID
		if id == "" {
			selector, id = "request-id", c.Submit.RequestID
		}
		original, err := cli.Inspect(ctx, selector, id, false)
		var failure *runnerclient.Failure
		if c.RunID == "" && errors.As(err, &failure) && failure.Code == "request_not_found" {
			continue
		}
		if err != nil {
			return err
		}
		run := original.Run
		if run == nil || c.RunID != "" && run.RunID != c.RunID {
			return errors.New("cleanup lost the original Run binding")
		}
		if run.StopStatus == "stopped" {
			continue
		}
		accepted := false
		for retry := 0; retry < 5; retry++ {
			body, _ := json.Marshal(map[string]uint64{"expected_revision": run.Revision})
			request := runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "fixture-stop-" + run.RunID + "-" + strconv.FormatUint(run.Revision, 10), Input: body}
			if _, err = cli.Call(ctx, "stop", []string{"--run-id", run.RunID}, &request); err == nil {
				accepted = true
				break
			}
			// This exact structured rejection proves zero acceptance; refresh
			// only the same original stop target. Unknown outcomes are returned.
			var rejected *runnerclient.Failure
			if !errors.As(err, &rejected) || rejected.Code != "stale_revision" {
				return err
			}
			fresh, inspectErr := cli.Inspect(ctx, "run-id", run.RunID, false)
			if inspectErr != nil {
				return inspectErr
			}
			if fresh.Run == nil || fresh.Run.RunID != run.RunID {
				return errors.New("cleanup refresh changed original Run")
			}
			run = fresh.Run
			if run.StopStatus == "stopped" {
				accepted = true
				break
			}
		}
		if !accepted {
			return fmt.Errorf("original Run stop stayed stale after bounded refresh: %w", err)
		}
		for {
			view, err := cli.Inspect(ctx, "run-id", run.RunID, false)
			if err != nil {
				return err
			}
			if view.Run.StopStatus == "stopped" {
				break
			}
			if view.Run.StopStatus == "unknown" || view.Run.StopStatus == "unsupported" {
				return fmt.Errorf("original Run stop is %s: %s", view.Run.StopStatus, view.Run.Diagnostic)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	return cli.StopService(ctx, service.InstanceID, "fixture-close-"+service.InstanceID)
}

// CleanupOwnedRunner runs in the same environment as the original project.
// The helper inherits external tool references without exporting credentials.
func (c *Client) CleanupOwnedRunner(ctx context.Context, helper string) error {
	_, err := c.runWithTimeout(ctx, 110*time.Second, helper, "runner-cleanup", "-config", c.Config)
	return err
}

func MainRunnerCleanup(args []string) error {
	fs := flag.NewFlagSet("runner-cleanup", flag.ContinueOnError)
	path := fs.String("config", "", "owned project config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := app.LoadConfig(*path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	if err := c.ResolveIdentity(ctx); err != nil {
		return err
	}
	hc, err := app.BuildHostConfig(ctx, c)
	if err != nil {
		return err
	}
	candidates, err := ReadOriginalOperations(ctx, filepath.Join(hc.RunDir, "steerline.db"), c.Identity())
	if err != nil {
		return err
	}
	err = CleanupRunner(ctx, app.GlobalExecutionConfigPath(), filepath.Join(hc.RunDir, "runner"), candidates)
	if encodeErr := json.NewEncoder(os.Stdout).Encode(map[string]any{"confirmed": err == nil, "error": fmt.Sprint(err)}); encodeErr != nil {
		return encodeErr
	}
	return err
}
