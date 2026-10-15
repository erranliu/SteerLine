package taskrunner

import (
	"encoding/json"
	"strings"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	model "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// Decision is a UI projection of public Runner controls, never a persisted
// native question/route or a second execution state machine.
func Decision(run *runnerclient.Run) *model.DecisionState {
	if run == nil {
		return nil
	}
	d := &model.DecisionState{AttemptID: run.Execution.AttemptID, Version: run.Revision, Status: "pending"}
	d.Request = model.DecisionRequest{FormatVersion: 2, BlocksProgress: true}
	if run.Controls.CanResume {
		d.ID = digest("resume:" + run.RunID)[:32]
		d.Request.InteractionKind = "runner_resume"
		d.Request.Question = "继续原任务"
		d.Request.Options = []b.DecisionOption{{ID: "resume", Label: "继续原任务"}}
		return d
	}
	if run.Question != nil {
		q := run.Question
		d.ID = q.ID
		d.Request.InteractionKind = "runner_question"
		d.Request.Question = q.Text
		d.Request.AllowFreeText = q.AllowFreeText
		if !run.Controls.CanReply {
			d.Status = "unavailable"
			d.Request.Context = run.Controls.Reason
		}
		for _, o := range q.Options {
			d.Request.Options = append(d.Request.Options, b.DecisionOption{ID: o.ID, Label: o.Label, Detail: o.Detail})
		}
		for _, n := range q.NativeQuestions {
			question := b.Question{ID: n.ID, Question: n.Text, AllowFreeText: n.AllowFreeText}
			for _, o := range n.Options {
				question.Options = append(question.Options, b.DecisionOption{ID: o.ID, Label: o.Label, Detail: o.Detail})
			}
			d.Request.Questions = append(d.Request.Questions, question)
		}
		if len(q.NativeQuestions) == 1 {
			n := d.Request.Questions[0]
			d.Request.Question = n.Question
			d.Request.Options = n.Options
			d.Request.AllowFreeText = n.AllowFreeText
		} else if len(q.NativeQuestions) > 1 {
			var context strings.Builder
			context.WriteString("用 JSON 回答全部问题，键为问题 ID，值为答案数组。\n")
			for _, n := range d.Request.Questions {
				context.WriteString(n.ID + ": " + n.Question + "\n")
				for _, o := range n.Options {
					context.WriteString("  " + o.ID + ": " + o.Label + "\n")
				}
			}
			d.Request.Context = context.String()
			d.Request.AllowFreeText = true
		}
		return d
	}
	if run.Status == "pr_ready" && run.Controls.CanReply {
		d.ID = digest("feedback:" + run.RunID)[:32]
		d.Request.InteractionKind = "business_feedback"
		d.Request.Question = "向原任务提交业务反馈"
		d.Request.AllowFreeText = true
		return d
	}
	return nil
}

func InputForAnswer(candidate runnerbridge.Candidate, answer api.DecisionAnswer) (string, json.RawMessage, error) {
	run := candidate.Projection
	decision := Decision(run)
	if candidate.StopRequested || candidate.Participation != "active" || run == nil || decision == nil || answer.RunID != candidate.ID || answer.DecisionID != decision.ID || answer.ExpectedRunRevision != candidate.Revision || answer.ExpectedDecisionVersion != run.Revision {
		return "", nil, failure("stale_version", "question or candidate changed; refresh before answering")
	}
	if answer.SourceAttemptID != "" && (answer.SourceAttemptID != run.Execution.AttemptID || answer.ExpectedRecoveryVersion != run.Revision) {
		return "", nil, failure("stale_recovery", "original Attempt or recovery revision changed")
	}
	if decision.Request.InteractionKind == "runner_resume" {
		if answer.OptionID != "resume" || strings.TrimSpace(answer.Note) != "" || len(answer.Answers) > 0 {
			return "", nil, failure("invalid_request", "resume requires its explicit option without business feedback")
		}
		body, _ := json.Marshal(map[string]uint64{"expected_revision": run.Revision})
		return "resume", body, nil
	}
	if !run.Controls.CanReply {
		return "", nil, failure("candidate_inactive", "original Runner Run does not permit reply")
	}
	text := answer.Note
	if len(answer.Answers) > 0 && (answer.OptionID != "" || strings.TrimSpace(answer.Note) != "") {
		return "", nil, failure("invalid_request", "structured answers cannot also contain an option or note")
	}
	kind := "feedback"
	questionID := ""
	if run.Question != nil {
		kind = "answer"
		q := run.Question
		questionID = q.ID
		if len(q.NativeQuestions) > 1 {
			values := map[string][]string{}
			for key, parts := range answer.Answers {
				values[key] = append([]string(nil), parts...)
			}
			if len(values) == 0 {
				canonical, err := taskmodel.CanonicalJSON([]byte(answer.Note))
				if err != nil {
					return "", nil, failure("invalid_request", "answer JSON must not contain duplicate or ambiguous keys")
				}
				if err := json.Unmarshal(canonical, &values); err != nil {
					return "", nil, failure("invalid_request", "answer all native questions with JSON keyed by question ID")
				}
			}
			if len(values) != len(q.NativeQuestions) || answer.OptionID != "" {
				return "", nil, failure("invalid_request", "answer every original native question exactly once")
			}
			for _, question := range q.NativeQuestions {
				parts, ok := values[question.ID]
				if !ok || len(parts) == 0 || len(parts) > 2 || len(parts) > 1 && !question.AllowFreeText {
					return "", nil, failure("invalid_request", "structured answer differs from the native question")
				}
				for i, part := range parts {
					if strings.TrimSpace(part) == "" {
						return "", nil, failure("invalid_request", "native answer cannot be empty")
					}
					offered := false
					for _, option := range question.Options {
						if part == option.ID || part == option.Label {
							parts[i], offered = option.Label, true
							break
						}
					}
					if !offered && !question.AllowFreeText {
						return "", nil, failure("invalid_request", "native answer option is not offered")
					}
				}
			}
			body, _ := json.Marshal(values)
			text = string(body)
		} else {
			options := q.Options
			allowText := q.AllowFreeText
			id := q.ID
			if len(q.NativeQuestions) == 1 {
				options = q.NativeQuestions[0].Options
				allowText = q.NativeQuestions[0].AllowFreeText
				id = q.NativeQuestions[0].ID
			}
			if len(answer.Answers) > 0 {
				values := append([]string(nil), answer.Answers[id]...)
				if len(answer.Answers) != 1 || len(values) == 0 || len(values) > 2 || len(values) > 1 && !allowText {
					return "", nil, failure("invalid_request", "answer differs from the original native question")
				}
				for i, part := range values {
					if strings.TrimSpace(part) == "" {
						return "", nil, failure("invalid_request", "native answer cannot be empty")
					}
					offered := false
					for _, option := range options {
						if part == option.ID || part == option.Label {
							values[i], offered = option.Label, true
							break
						}
					}
					if !offered && !allowText {
						return "", nil, failure("invalid_request", "native answer option is not offered")
					}
				}
				text = strings.Join(values, "\n")
			} else if answer.OptionID != "" {
				text = ""
				for _, option := range options {
					if option.ID == answer.OptionID || option.Label == answer.OptionID {
						text = option.Label
						break
					}
				}
				if text == "" {
					return "", nil, failure("invalid_request", "answer option is not offered")
				}
				if strings.TrimSpace(answer.Note) != "" {
					if !allowText {
						return "", nil, failure("invalid_request", "this native question does not accept an additional note")
					}
					text += "\n" + answer.Note
				}
			} else if !allowText {
				return "", nil, failure("invalid_request", "this native question requires an offered option")
			}
		}
	} else if answer.OptionID != "" || len(answer.Answers) > 0 {
		return "", nil, failure("invalid_request", "business feedback requires its original text")
	}
	if strings.TrimSpace(text) == "" {
		return "", nil, failure("invalid_request", "reply text cannot be empty")
	}
	body, _ := json.Marshal(struct {
		Revision   uint64 `json:"expected_revision"`
		Kind       string `json:"kind"`
		QuestionID string `json:"question_id,omitempty"`
		Text       string `json:"text"`
	}{run.Revision, kind, questionID, text})
	return "reply", body, nil
}
