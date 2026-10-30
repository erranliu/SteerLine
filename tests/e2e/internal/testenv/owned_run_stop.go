//go:build e2e

package testenv

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// stopOriginalRun settles this acceptance's native execution without withdrawing
// its candidate from the task. Only a proven zero-acceptance CAS rejection may
// refresh the same original target; unknown requests never cause another write.
func (s *Scenario) stopOriginalRun(runner runnerclient.Client, runID, digest, prefix string) {
	t := s.t
	if runID == "" || digest == "" {
		t.Fatal("original Run binding required before stop")
	}
	accepted := false
	for attempt := 0; attempt < 5; attempt++ {
		observed, err := runner.Inspect(s.Context, "run-id", runID, false)
		s.Must(err)
		if observed.Run == nil || observed.Run.RunID != runID || observed.Run.InputDigest != digest {
			t.Fatal("original Run identity lost before stop")
		}
		if observed.Run.StopStatus == "stopped" {
			accepted = true
			break
		}
		revision := observed.Run.Revision
		body, err := json.Marshal(map[string]uint64{"expected_revision": revision})
		s.Must(err)
		request := runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "progress-stop-" + runID + "-" + strconv.FormatUint(revision, 10), Input: body}
		s.Must(controlled.Write(filepath.Join(s.Output, fmt.Sprintf(prefix+"stop-request-%d.json", attempt)), request))
		response, err := runner.Call(s.Context, "stop", []string{"--run-id", runID}, &request)
		s.Must(controlled.Write(filepath.Join(s.Output, fmt.Sprintf(prefix+"stop-response-%d.json", attempt)), map[string]any{"response": response, "error": fmt.Sprint(err)}))
		if err == nil {
			accepted = true
			break
		}
		// Refresh only after a structured zero-acceptance CAS rejection.
		var rejected *runnerclient.Failure
		if !errors.As(err, &rejected) || rejected.Code != "stale_revision" || rejected.Unknown {
			s.Must(err)
		}
	}
	if !accepted {
		t.Fatal("original Run stop remained stale")
	}
	s.Wait("original public Run native stop settlement", func() (bool, error) {
		observed, err := runner.Inspect(s.Context, "run-id", runID, false)
		if err != nil || observed.Run == nil {
			return false, err
		}
		s.Must(controlled.Write(filepath.Join(s.Output, prefix+"stopped-run.json"), observed.Run))
		if observed.Run.RunID != runID || observed.Run.InputDigest != digest {
			t.Fatal("original Run identity changed during stop")
		}
		if observed.Run.StopStatus == "unknown" || observed.Run.StopStatus == "unsupported" {
			t.Fatal("original native stop remains unverified", observed.Run.StopStatus)
		}
		return observed.Run.StopStatus == "stopped", nil
	})
}
