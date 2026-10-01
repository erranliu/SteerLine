package contracts

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// NormalizeDecisionContent checks application display limits, without native binding or authorization.
func NormalizeDecisionText(req *DecisionRequest) error {
	if req == nil {
		return errors.New("decision object is missing")
	}
	req.Question = strings.TrimSpace(req.Question)
	req.Context = strings.TrimSpace(req.Context)
	if !utf8.ValidString(req.Question) || !utf8.ValidString(req.Context) {
		return errors.New("decision question and context must be valid UTF-8")
	}
	if req.Question == "" {
		return errors.New("decision question is empty")
	}
	return nil
}

func NormalizeDecisionContent(req *DecisionRequest) error {
	if err := NormalizeDecisionText(req); err != nil {
		return err
	}
	if len([]rune(req.Question)) > 2000 || len([]rune(req.Context)) > 8000 {
		return errors.New("decision question or context exceeds its protocol length limit")
	}
	if len(req.Options) == 0 && !req.AllowFreeText {
		return errors.New("decision needs options or allow_free_text")
	}
	if len(req.Options) > 12 {
		return errors.New("decision has too many options")
	}
	seen := map[string]bool{}
	for i := range req.Options {
		o := &req.Options[i]
		o.ID = strings.TrimSpace(o.ID)
		o.Label = strings.TrimSpace(o.Label)
		o.Detail = strings.TrimSpace(o.Detail)
		if !utf8.ValidString(o.ID) || !utf8.ValidString(o.Label) || !utf8.ValidString(o.Detail) {
			return fmt.Errorf("decision option %d contains invalid UTF-8", i+1)
		}
		switch {
		case o.ID == "":
			return errors.New("decision option id is empty")
		case len([]rune(o.ID)) > 128:
			return fmt.Errorf("decision option %q id exceeds its protocol length limit", o.ID)
		case o.Label == "":
			return fmt.Errorf("decision option %q has no label", o.ID)
		case len([]rune(o.Label)) > 300:
			return fmt.Errorf("decision option %q label exceeds its protocol length limit", o.ID)
		case len([]rune(o.Detail)) > 4000:
			return fmt.Errorf("decision option %q detail exceeds its protocol length limit", o.ID)
		case seen[o.ID]:
			return fmt.Errorf("duplicate decision option id %q", o.ID)
		}
		seen[o.ID] = true
	}
	if req.Recommended != "" && !seen[req.Recommended] {
		return errors.New("recommended option is not offered")
	}
	return nil
}

func ValidateDecisionRequest(req *DecisionRequest) error {
	if err := NormalizeDecisionText(req); err != nil {
		return err
	}
	if req.FormatVersion != DecisionRequestFormatVersion {
		return errors.New("decision request has an unsupported format version")
	}
	if req.QuestionRoute == nil {
		if len(req.Questions) != 0 {
			return errors.New("decision questions require an implementation route")
		}
	} else {
		if err := req.QuestionRoute.Validate(); err != nil {
			return err
		}
		if len(req.Questions) == 0 || len(req.Questions) > 3 {
			return errors.New("routed decision has no questions or exceeds the question limit")
		}
		seen := map[string]bool{}
		for i := range req.Questions {
			q := &req.Questions[i]
			if q.ID == "" || seen[q.ID] || q.ExecutionID != req.QuestionRoute.ExecutionID || q.AttemptID != req.QuestionRoute.AttemptID || q.Generation != req.QuestionRoute.Generation || q.TurnID != req.QuestionRoute.TurnID {
				return errors.New("decision question differs from its fixed execution, generation or turn")
			}
			seen[q.ID] = true
			content := DecisionRequest{FormatVersion: DecisionRequestFormatVersion, Question: q.Question, Options: q.Options, AllowFreeText: q.AllowFreeText}
			if err := NormalizeDecisionContent(&content); err != nil {
				return err
			}
			for _, option := range content.Options {
				if option.Receiver != "" || option.Action != "" {
					return errors.New("routed question options cannot carry center action authority")
				}
			}
			q.Question, q.Options = content.Question, content.Options
		}
	}
	return NormalizeDecisionContent(req)
}

// ValidateDecisionAnswer normalizes and validates saved answer content, without authorization or I/O.
func ValidateDecisionAnswer(req *DecisionRequest, a *DecisionAnswer) error {
	a.OptionID = strings.TrimSpace(a.OptionID)
	if req.HostTarget == nil {
		a.Note = strings.TrimSpace(a.Note)
	}
	a.AnsweredBy = strings.TrimSpace(a.AnsweredBy)
	if a.AnsweredBy == "" {
		return errors.New("answer requires answered_by")
	}
	if !utf8.ValidString(a.OptionID) || !utf8.ValidString(a.Note) || !utf8.ValidString(a.AnsweredBy) {
		return errors.New("answer fields must be valid UTF-8")
	}
	if len([]rune(a.Note)) > 4000 {
		return errors.New("answer note exceeds the 4000 character protocol limit")
	}
	if len([]rune(a.AnsweredBy)) > 200 {
		return errors.New("answered_by exceeds the 200 character protocol limit")
	}
	if questions := DecisionQuestions(req); len(questions) > 0 {
		if len(a.Answers) != len(questions) {
			return errors.New("native answer must include every question ID")
		}
		for _, q := range questions {
			values, ok := a.Answers[q.ID]
			if !ok || len(values) == 0 || len(values) > 2 {
				return fmt.Errorf("native answer for %q is missing or too long", q.ID)
			}
			for _, value := range values {
				if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || len([]rune(value)) > 4000 {
					return fmt.Errorf("native answer for %q is invalid", q.ID)
				}
			}
			if len(q.Options) > 0 && !q.AllowFreeText {
				offered := false
				for _, option := range q.Options {
					if values[0] == option.ID || values[0] == option.Label {
						offered = true
						break
					}
				}
				if !offered {
					return fmt.Errorf("native answer for %q is not offered", q.ID)
				}
			}
		}
		return nil
	}
	if len(req.Options) > 0 {
		if a.OptionID == "" {
			if req.AllowFreeText && a.Note != "" {
				return nil
			}
			return errors.New("answer requires an offered option")
		}
		for _, o := range req.Options {
			if o.ID == a.OptionID {
				return nil
			}
		}
		return fmt.Errorf("option %q is not one of the offered options", a.OptionID)
	}
	if a.Note == "" {
		return errors.New("this decision requires a free-text note")
	}
	return nil
}

// DecisionQuestions returns the typed questions already bound to a persisted request.
func DecisionQuestions(req *DecisionRequest) []executionapi.Question {
	if req == nil || req.QuestionRoute == nil {
		return nil
	}
	return req.Questions
}
