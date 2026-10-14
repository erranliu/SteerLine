package workflow

import "encoding/json"

type FeedbackData struct {
	Question     string              `json:"question"`
	Context      string              `json:"context,omitempty"`
	Selected     string              `json:"selected_answer,omitempty"`
	Note         string              `json:"user_text,omitempty"`
	Answers      map[string][]string `json:"answers,omitempty"`
	Diagnostic   string              `json:"blocker,omitempty"`
	QuestionID   string              `json:"question_id,omitempty"`
	SourceTurnID string              `json:"source_turn_id,omitempty"`
}

func RenderFeedbackData(value FeedbackData) ([]byte, error) {
	if value.Diagnostic == value.Context {
		value.Diagnostic = ""
	}
	return json.Marshal(value)
}
func FeedbackText(option, label, note string, asyncFacts json.RawMessage) string {
	if len(asyncFacts) > 0 {
		return "Answer to the original asynchronous question:\n" + string(asyncFacts)
	}
	if option == "" {
		return note
	}
	if label == "" {
		label = option
	}
	text := "Selected option: " + label
	if note != "" {
		text += "\n\nAdditional instructions:\n" + note
	}
	return text
}
func PublicationInstruction(step string, policy json.RawMessage, remote, base string) string {
	facts, _ := json.Marshal(map[string]any{"publication_policy": policy})
	text := "\nFixed execution facts: " + string(facts) + "\n"
	if step == StepPublish {
		target, _ := json.Marshal(struct {
			Remote     string `json:"remote"`
			BaseBranch string `json:"base_branch"`
		}{remote, base})
		text += "\nPR target: " + string(target) + "\n"
	}
	return text
}
