package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// The managed native launcher already freezes the actual Codex session/home.
// Read only that original binding through the existing public history port.
// This never starts/resumes a model turn or selects a session by directory scan.
func (l *ExecutionLifecycle) readBoundHistoryFinal(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, turn *NativeTurnState, state *NativeExecutionState) (bool, error) {
	path := filepath.Join(l.setup.RunDirectory, "runs", snapshot.Session.WorkflowRunID, "turns", snapshot.Session.ExecutionID, "codex-history.v1.json")
	b, err := codexworker.ReadHistoryBinding(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if b.ProjectID != snapshot.Session.ProjectID || b.TaskID != snapshot.Session.TaskID || b.WorkflowID != snapshot.Session.WorkflowRunID || b.ExecutionID != snapshot.Session.ExecutionID || b.AttemptID != snapshot.Session.AttemptID || b.Backend != snapshot.Ref.Backend || b.Protocol != snapshot.Ref.Protocol || b.TargetID != snapshot.Ref.Target.ID || !strings.EqualFold(filepath.Clean(b.Workspace), filepath.Clean(snapshot.Ref.WorkspacePath)) || b.Command.Executable != l.codex.Executable {
		return true, errors.New("original native history binding changed execution/target/workspace")
	}
	if state.ProviderSessionID != "" && state.ProviderSessionID != b.SessionID {
		return true, errors.New("original provider session changed")
	}
	client, err := codexworker.OpenHistory(ctx, b.Command, l.setup.RepositoryPath, b.Home, l.setup.ProcessHost.StartManaged)
	if err != nil {
		return true, err
	}
	defer client.Close()
	if err := client.ReadMetadata(ctx, b.SessionID, b.Workspace); err != nil {
		return true, err
	}
	cursor := ""
	matches := 0
	var selected codexworker.HistoryTurn
	for count := 0; count < 2048; count++ {
		page, err := client.ReadPage(ctx, b.SessionID, cursor)
		if err != nil {
			return true, err
		}
		for _, native := range page.Data {
			bound, err := historyTaskBound(native, turn.TaskID)
			if err != nil {
				return true, err
			}
			if bound {
				matches++
				selected = native
			}
		}
		if page.NextCursor == nil {
			break
		}
		if *page.NextCursor == "" || *page.NextCursor == cursor {
			return true, errors.New("original native history cursor did not advance")
		}
		cursor = *page.NextCursor
		if count == 2047 {
			return true, errors.New("original native history exceeds bounded turn lookup")
		}
	}
	if matches != 1 || selected.ID == "" || selected.CompletedAt == nil || selected.Status != "completed" && selected.Status != "failed" && selected.Status != "interrupted" {
		return true, errors.New("original Task has no unique stopped native history turn")
	}
	if selected.Status != "completed" {
		turn.Outcome = "failed"
		turn.LastMessage = "Original native Codex turn ended with " + selected.Status + ": " + string(selected.Error)
	}
	for _, raw := range selected.Items {
		var item struct {
			ID    string  `json:"id"`
			Type  string  `json:"type"`
			Phase string  `json:"phase"`
			Text  *string `json:"text"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return true, err
		}
		if item.Type == "agentMessage" && item.Phase == "final_answer" && item.ID != "" && item.Text != nil {
			turn.FinalSource, turn.FinalItemID, turn.FinalNativeTurnID = "orca.codex.history.final_answer", item.ID, selected.ID
			turn.FinalText, turn.FinalVerified, turn.FinalReadError = *item.Text, true, ""
		}
	}
	if !turn.FinalVerified {
		return true, errors.New("original stopped native history lacks final_answer identity/body")
	}
	state.ProviderSessionID = b.SessionID
	return true, nil
}

func historyTaskBound(native codexworker.HistoryTurn, task string) (bool, error) {
	bound := false
	for _, raw := range native.Items {
		var item struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return false, err
		}
		if item.Type != "userMessage" {
			continue
		}
		text := item.Text
		for _, part := range item.Content {
			if part.Type == "text" || part.Type == "input_text" {
				text += part.Text
			}
		}
		if nativeTaskHeader(text, task) {
			bound = true
		}
	}
	return bound, nil
}

func nativeTaskHeader(text, task string) bool {
	for _, line := range strings.Split(text, "\n") {
		if _, value, ok := strings.Cut(line, "Your task ID is: "); ok {
			fields := strings.Fields(value)
			if len(fields) > 0 && fields[0] == task {
				return true
			}
		}
	}
	return false
}
