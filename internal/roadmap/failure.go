package roadmap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func (s *Service) contentFailure(ctx context.Context, run *runnercontract.Run) string {
	result := run.ContentResult
	if result == nil {
		result = &runnercontract.ContentResult{}
		if run.Final != nil {
			result.Source = *run.Final
		}
	}
	parts := []string{"未取得经核实的完整生成结果"}
	add := func(value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		for _, previous := range parts {
			if previous == value {
				return
			}
		}
		parts = append(parts, value)
	}
	add(result.Diagnostic)
	add(run.Diagnostic)
	add(result.Source.Diagnostic)
	if run.Final != nil {
		add(run.Final.Diagnostic)
	}
	if result.Diagnostic == "human_input_required" || run.Diagnostic == "human_input_required" {
		questions, err := s.failureQuestions(ctx, run, result.Source)
		if err != nil {
			add("原问题详情读取失败：" + err.Error())
		} else {
			for _, question := range questions {
				add(question)
			}
		}
	}
	return strings.Join(parts, "；")
}

// Read only Runner's public, paged history, bound to the failed original turn.
// A historical question never creates a reply or resume opportunity.
func (s *Service) failureQuestions(ctx context.Context, run *runnercontract.Run, source runnercontract.Final) ([]string, error) {
	if source.CommandID == "" || source.SessionID == "" || source.TurnID == "" || source.AttemptID != run.Execution.AttemptID || source.SessionID != run.Execution.SessionID || source.TurnID != run.Execution.TurnID {
		return nil, fmt.Errorf("原问题执行身份不完整或不一致")
	}
	after := ""
	seen := map[string]bool{}
	questions := []string{}
	for page := 0; page < 100; page++ {
		args := []string{"--run-id", run.RunID, "--limit", "500"}
		if after != "" {
			args = append(args, "--after", after)
		}
		response, err := s.Runner.Call(ctx, "history", args, nil)
		if err != nil {
			return nil, err
		}
		var history struct {
			RunID   string             `json:"run_id"`
			Run     runnercontract.Run `json:"run"`
			Entries []struct {
				Role      string                   `json:"role"`
				CommandID string                   `json:"command_id"`
				SessionID string                   `json:"session_id"`
				TurnID    string                   `json:"turn_id"`
				Question  *runnercontract.Question `json:"question"`
			} `json:"entries"`
			Next string `json:"next_cursor"`
			More bool   `json:"has_more"`
		}
		if err = json.Unmarshal(response.Data, &history); err != nil {
			return nil, err
		}
		if history.RunID != run.RunID || history.Run.RunID != run.RunID || history.Run.InputDigest != run.InputDigest || history.Run.Revision < run.Revision || history.Entries == nil || len(history.Entries) > 500 || history.Next == "" || seen[history.Next] || history.More && len(history.Entries) == 0 {
			return nil, fmt.Errorf("原问题历史身份或分页未核实")
		}
		for _, entry := range history.Entries {
			if entry.Role != "question" || entry.Question == nil || entry.CommandID != source.CommandID || entry.SessionID != source.SessionID || entry.TurnID != source.TurnID {
				continue
			}
			if entry.Question.Text != "" {
				questions = append(questions, entry.Question.Text)
			}
			for _, q := range entry.Question.NativeQuestions {
				if q.Text != "" {
					questions = append(questions, q.Text)
				}
			}
		}
		if !history.More {
			if len(questions) == 0 {
				return nil, fmt.Errorf("原执行的问题正文不可用")
			}
			return questions, nil
		}
		seen[history.Next], after = true, history.Next
	}
	return nil, fmt.Errorf("原问题历史超过分页上限")
}
