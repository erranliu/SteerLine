package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	mv "github.com/erranliu/SteerLine/internal/manualverification"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func checkManualVerificationProof(path string) error {
	var proof struct {
		Draft    bool      `json:"initial_draft"`
		PR       string    `json:"pr"`
		First    string    `json:"first_head"`
		Second   string    `json:"second_head"`
		Final    mv.Record `json:"final"`
		Delivery int       `json:"delivery_count"`
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if json.Unmarshal(raw, &proof) != nil || !proof.Draft || !strings.HasPrefix(proof.PR, "https://github.com/erranliu/SteerlineTest/pull/") || !taskmodel.ValidCommit(proof.First) || !taskmodel.ValidCommit(proof.Second) || proof.First == proof.Second || proof.Delivery != 1 || !proof.Final.Merged || proof.Final.Archived || proof.Final.Closure == nil || proof.Final.PR != proof.PR || proof.Final.Head != proof.Second || len(proof.Final.Results) != 3 || len(proof.Final.Attempts) != 4 {
		return errors.New("manual verification live proof is incomplete")
	}
	for i, head := range []string{proof.First, proof.Second} {
		result, attempt := proof.Final.Results[i], proof.Final.Attempts[i]
		if result.Source != mv.SourcePR || result.Head != head || attempt.Source != mv.SourcePR || attempt.Head != head || result.AttemptID != attempt.ID || result.Outcome != []string{"failed", "passed"}[i] {
			return errors.New("manual verification evidence differs from its original commit or attempt")
		}
	}
	for _, attempt := range proof.Final.Attempts {
		if !attempt.Started || !attempt.Removed || attempt.Status != "stopped" {
			return errors.New("manual verification attempt was not started and cleaned")
		}
	}
	pr, remote := proof.Final.Attempts[2], proof.Final.Attempts[3]
	closure, result := proof.Final.Closure, proof.Final.Results[2]
	if pr.Source != mv.SourcePR || pr.Head != proof.Second || remote.Source != mv.SourceRemote || !taskmodel.ValidCommit(remote.Head) ||
		result.Source != remote.Source || result.Head != remote.Head || result.AttemptID != remote.ID || result.Outcome != "failed" ||
		closure.Source != remote.Source || closure.Head != remote.Head || closure.Actor == "" || closure.At.IsZero() {
		return errors.New("post-merge manual verification proof is incomplete")
	}
	return nil
}
