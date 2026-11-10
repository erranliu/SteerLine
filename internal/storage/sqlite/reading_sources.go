package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/steeragent/suggestion/contract"
)

func indexRoadmapReading(ctx context.Context, tx readingWriter, r roadmap.Record) error {
	f := readingField{source: "roadmap", sourceID: r.ID, origin: "roadmap", originID: r.ID}
	put := func(id, path, role, kind, label, text string) error {
		v := f
		v.id = id
		v.field = path
		v.role = role
		v.kind = kind
		v.label = label
		v.text = text
		if kind == "draft" {
			v.at = r.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		}
		return putReadingField(ctx, tx, r.Project, v)
	}
	if err := put("draft", "$.source.body", "user", "draft", r.Source.Path, r.Source.Body); err != nil {
		return err
	}
	if r.Analysis != nil {
		body, _ := json.Marshal(r.Analysis)
		if err := put("analysis", "$.analysis", "assistant", "generated_result", "", string(body)); err != nil {
			return err
		}
	}
	if err := put("design", "$.design", "assistant", "generated_result", "", r.Design); err != nil {
		return err
	}
	for i, d := range r.Documents {
		parts := []struct{ path, label, body string }{{"spec", "Spec", d.Spec.Body}, {"plan", "Plan", d.Plan.Body}}
		for _, p := range parts {
			if err := put(fmt.Sprintf("document:%d:%s", i, p.path), fmt.Sprintf("$.documents[%d].%s.body", i, p.path), "assistant", "generated_result", p.label+" / "+d.Key, p.body); err != nil {
				return err
			}
		}
		for j, a := range d.Acceptance {
			if err := put(fmt.Sprintf("acceptance:%d:%d", i, j), fmt.Sprintf("$.documents[%d].acceptance[%d].body", i, j), "assistant", "generated_result", a.Path, a.Body); err != nil {
				return err
			}
		}
	}
	for i, q := range r.Questions {
		if err := put(fmt.Sprintf("question:%d", i), fmt.Sprintf("$.document_questions[%d]", i), "assistant", "question_context", "", q); err != nil {
			return err
		}
	}
	return nil
}

func indexEnvironmentReading(ctx context.Context, tx readingWriter, r env.Record) error {
	for i, round := range r.Rounds {
		if round.Run == nil || round.Run.Final == nil {
			continue
		}
		final := round.Run.Final
		if !final.Verified || final.Text == "" {
			continue
		}
		if err := putReadingField(ctx, tx, r.Project, readingField{source: "environment_detection", sourceID: r.ID, id: fmt.Sprintf("round:%d:final", i), origin: "environment", originID: r.ID, field: fmt.Sprintf("$.rounds[%d].run.final.text", i), role: "assistant", kind: "verified_final", text: final.Text, run: round.RunID, attempt: final.AttemptID, session: final.SessionID, turn: final.TurnID, group: fmt.Sprint(round.Number)}); err != nil {
			return err
		}
	}
	return nil
}

func indexAdvisoryReading(ctx context.Context, tx readingWriter, project, id string, raw []byte) error {
	var r struct {
		Suggestion *contract.Suggestion `json:"suggestion"`
		Source     struct {
			CandidateID string              `json:"candidate_id"`
			Occurrence  string              `json:"occurrence"`
			Facts       []contract.Material `json:"facts"`
		} `json:"source"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if r.Suggestion != nil {
		if err := putReadingField(ctx, tx, project, readingField{source: "main_agent_advisory", sourceID: id, id: "suggestion", origin: "advisory", originID: id, field: "$.suggestion.prompt", role: "assistant", kind: "suggestion", text: r.Suggestion.Prompt}); err != nil {
			return err
		}
	}
	for i, m := range r.Source.Facts {
		if m.ID != "question" || m.Source != r.Source.CandidateID || m.Revision != r.Source.Occurrence {
			continue
		}
		attempt, question, ok := strings.Cut(r.Source.Occurrence, ":")
		if !ok || attempt == "" || question == "" {
			continue
		}
		var q struct{ ID, Text string }
		if json.Unmarshal([]byte(m.Body), &q) != nil || q.ID != question {
			continue
		}
		// Body is the original saved question JSON, including structured questions.
		f := readingField{source: "main_agent_advisory", sourceID: id, id: "question:" + r.Source.Occurrence, origin: "advisory", originID: id, field: fmt.Sprintf("$.source.facts[%d].body", i), role: "assistant", kind: "question_context", text: m.Body, attempt: attempt}
		if err := putReadingField(ctx, tx, project, f); err != nil {
			return err
		}
		f.source = "task_candidate"
		f.sourceID = r.Source.CandidateID
		if err := putReadingField(ctx, tx, project, f); err != nil {
			return err
		}
	}
	return nil
}
