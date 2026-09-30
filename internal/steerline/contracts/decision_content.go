package contracts

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
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
