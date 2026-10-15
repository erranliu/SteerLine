package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
		body, _ := json.Marshal(map[string]uint64{"expected_revision": run.Revision})
		request := runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "fixture-stop-" + run.RunID + "-" + strconv.FormatUint(run.Revision, 10), Input: body}
		if _, err = cli.Call(ctx, "stop", []string{"--run-id", run.RunID}, &request); err != nil {
			return err
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
