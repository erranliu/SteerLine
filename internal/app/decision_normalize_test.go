package app

import (
	"reflect"
	"testing"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	model "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestLocalAPIDecisionNormalizationUsesOriginalQuestions(t *testing.T) {
	question := model.DecisionRequest{Question: "Continue?", Options: []backend.DecisionOption{{ID: "yes", Label: "Continue"}}, AllowFreeText: true}
	for _, native := range []bool{false, true} {
		if native {
			question.QuestionRoute = &backend.ImplementationDataEnvelope{}
			question.Questions = []backend.Question{{ID: "question-original", Options: question.Options, AllowFreeText: true}}
		}
		input := api.DecisionInput{CommandID: "command-original", RequestedBy: "operator", Answer: api.DecisionAnswer{RunID: "run-original", DecisionID: "decision-original", SourceAttemptID: "attempt-original", ExpectedDecisionVersion: 3, ExpectedRunRevision: 7, ExpectedRecoveryVersion: 2, ServiceInstanceID: "instance-original", OptionID: " yes ", Note: " original remarks ", AnsweredBy: "operator"}}
		mapped, err := normalizeDecisionInput(input, &question)
		if err != nil {
			t.Fatal(err)
		}
		if mapped.Answer.OptionID != "yes" || mapped.Answer.Note != "original remarks" || mapped.Answer.RunID != input.Answer.RunID || mapped.Answer.SourceAttemptID != input.Answer.SourceAttemptID || mapped.Answer.ExpectedRecoveryVersion != 2 || mapped.Answer.ServiceInstanceID != input.Answer.ServiceInstanceID {
			t.Fatalf("mapped answer lost semantics: %+v", mapped)
		}
		if native && !reflect.DeepEqual(mapped.Answer.Answers, map[string][]string{"question-original": {"Continue", "original remarks"}}) {
			t.Fatalf("native choice/text/remarks did not use original question: %+v", mapped.Answer)
		}
		if input.Answer.OptionID != " yes " || input.Answer.Note != " original remarks " || input.Answer.Answers != nil {
			t.Fatal("mapping changed the original input")
		}
	}
}
