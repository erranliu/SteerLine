package workflow

import (
	"fmt"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
)

// Transition declares business changes. It never sends input or saves records.
type Transition struct {
	Next                                                    *StageInput
	Status, Reason, Outcome, PRCandidateURL                 string
	Report                                                  *Report
	ReportRetry                                             uint64
	AcceptResult, RecoverOriginal, Publish, ExpireQuestions bool
}

func PublicationSourceSucceeded(w Snapshot, input StageInput) bool {
	if input.StepID != StepPublishReport {
		return input.StepID == StepPublish
	}
	source, original := Result(w, input.SourceCommandID), Input(w, input.SourceCommandID)
	return source != nil && original != nil && source.StepID == StepPublish && original.StepID == StepPublish && source.TurnID == input.SourceTurnID && source.Succeeded && source.Failure == "" && source.FinalVerified
}

// Route is the single owner of development/v3 result interpretation and retry.
// ReportRetry is a business format correction, never a transport retry.
func Route(w Snapshot, input StageInput, result reliable.Result, reportPolicy string) Transition {
	t := Transition{ReportRetry: w.ReportRetry, AcceptResult: true}
	failedCore := !result.Succeeded && (input.StepID == StepImplement || input.StepID == StepReport)
	if !failedCore && input.StepID != StepImplement && !result.ContentVerified {
		t.AcceptResult = false
		t.Status = "reconciling"
		t.Reason = "original stopped turn lacks verified final provenance: " + result.Diagnostic
		return t
	}
	if failedCore {
		t.RecoverOriginal = true
		t.Outcome = "blocked"
		t.Reason = "Original turn failed or was interrupted; continue the original session to verify current work: " + result.Failure
		return t
	}
	if reportPolicy == "" {
		reportPolicy = "frozen_native_permissions; business_read_only_is_prompt_policy"
	}
	if input.StepID == StepImplement {
		t.Next = &StageInput{StepID: StepReport, SourceCommandID: input.CommandID, SourceTurnID: result.TurnID, ReportToolPolicy: reportPolicy}
		return t
	}
	report, err := DecodeReport(result.Content, input.StepID)
	if err == nil && PublicationStep(input.StepID) {
		t.PRCandidateURL = report.Outputs.PRURL
	}
	if err == nil && report.Outcome == "blocked" && report.Blocker.Category == "unknown_external" {
		t.Report = &report
		t.Outcome = report.Outcome
		t.Status = "reconciling"
		t.Reason = blockerReason(report.Blocker)
		return t
	}
	if err != nil || !result.Succeeded && report.Outcome == "done" {
		t.Reason = "result_invalid: " + result.Failure
		if err != nil {
			t.Reason += "; " + err.Error()
		}
		if err != nil && input.StepID == StepPublish {
			t.Next = &StageInput{StepID: StepPublishReport, SourceCommandID: input.CommandID, SourceTurnID: result.TurnID, ReportToolPolicy: reportPolicy}
			return t
		}
		if input.StepID == StepReport && w.ReportRetry == 0 {
			t.ReportRetry = 1
			t.Next = &StageInput{StepID: input.StepID, SourceCommandID: input.SourceCommandID, SourceTurnID: input.SourceTurnID, ReportToolPolicy: input.ReportToolPolicy}
			return t
		}
		t.Status = "blocked"
		return t
	}
	t.Report = &report
	t.Outcome = report.Outcome
	switch report.Outcome {
	case "needs_input":
		t.Status = "waiting_input"
		t.Reason = report.Question
		return t
	case "blocked":
		t.Status = "blocked"
		t.Reason = blockerReason(report.Blocker)
		return t
	}
	if input.StepID == StepReport {
		source := Result(w, input.SourceCommandID)
		if source == nil || source.TurnID != input.SourceTurnID || !source.Succeeded || source.Failure != "" {
			t.Status = "blocked"
			t.Reason = "source execution failed; report generated, original failure retained"
			return t
		}
		t.ExpireQuestions = true
		t.Next = &StageInput{StepID: StepPublish}
		return t
	}
	if input.StepID == StepPublishReport && !PublicationSourceSucceeded(w, input) {
		t.Status = "reconciling"
		t.Reason = "source publication failed; report generated, original failure retained"
		return t
	}
	t.Publish = true
	return t
}
func blockerReason(b *Blocker) string {
	return fmt.Sprintf("%s: %s; %s; known=%s; unknown=%s", b.Category, b.Action, b.Error, b.Known, b.Unknown)
}
