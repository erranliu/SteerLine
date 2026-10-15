package workflow

import "errors"

// FeedbackIntent carries business choices and opaque source association.
// Unsent is a reliable notification, not an inspection of native operations.
type FeedbackIntent struct {
	Option                    string
	Continue                  bool
	Async                     bool
	SourceTurn, SourceAttempt string
	UnsentReportCommand       string
}

func FeedbackStep(w Snapshot, intent FeedbackIntent) (string, uint64, error) {
	step, retry := w.StepID, w.ReportRetry
	if intent.Continue && step == StepReport {
		valid := false
		for i := len(w.StageInputs) - 1; i >= 0; i-- {
			previous := w.StageInputs[i]
			if previous.StepID != StepReport {
				continue
			}
			source := Result(w, previous.SourceCommandID)
			valid = source != nil && source.Succeeded && source.Failure == "" && source.TurnID == previous.SourceTurnID && previous.CommandID == intent.UnsentReportCommand
			break
		}
		if !valid {
			step, retry = StepImplement, 0
		}
	}
	if intent.Async {
		found := false
		for _, result := range w.StageResults {
			if result.TurnID == intent.SourceTurn && result.AttemptID == intent.SourceAttempt {
				step = result.StepID
				found = true
				break
			}
		}
		if !found {
			return "", retry, errors.New("async continuation has no verified originating stage result")
		}
	}
	if step == StepAwaitMerge || intent.Option == "implement" {
		if intent.Option == "publish" {
			step = StepPublish
		} else {
			step, retry = StepImplement, 0
		}
	}
	return step, retry, nil
}
func FeedbackInput(w Snapshot) StageInput {
	input := StageInput{StepID: w.StepID, TaskFacts: TaskFacts(w)}
	if ReportStep(w.StepID) {
		for i := len(w.StageInputs) - 1; i >= 0; i-- {
			previous := w.StageInputs[i]
			if previous.StepID == w.StepID {
				input.SourceCommandID, input.SourceTurnID, input.ReportToolPolicy = previous.SourceCommandID, previous.SourceTurnID, previous.ReportToolPolicy
				break
			}
		}
	}
	return input
}
