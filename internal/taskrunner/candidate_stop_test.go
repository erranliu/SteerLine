package taskrunner

import (
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"testing"
	"time"
)

func TestRunnerCandidateStopProjection(t *testing.T) {
	at := time.Now().UTC()
	c := runnerbridge.Candidate{ID: "candidate", Participation: "abandoned", Withdrawal: &runnerstate.Withdrawal{RequestID: "stop", RequestedBy: "operator", At: at}, Projection: &runnerclient.Run{Status: "running", Revision: 1, ResumeRequired: true}}
	v := projectCandidate(c)
	if v.Participation != "abandoned" || v.ExecutionStatus != "running" || v.CandidateStopReason != "manual" || v.CandidateStoppedBy != "operator" || v.CandidateEndedAt == nil || !v.CandidateEndedAt.Equal(at) || v.ResourceStatus != "retained" || v.Decision != nil {
		t.Fatal("withdrawal lost facts or invented resource stop", v)
	}
	c.StopRequested = true
	v = projectCandidate(c)
	if v.ResourceStatus != "stop_pending" {
		t.Fatal("Task stop obligation was hidden")
	}
}
