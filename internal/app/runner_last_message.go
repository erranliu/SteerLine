package app

import (
	"context"
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"path/filepath"
	"strings"
)

func (r *clientReader) LastMessage(ctx context.Context, candidateID, execution, attempt string) (api.LastMessageView, error) {
	var view api.LastMessageView
	config := r.config
	reader, err := OpenNativeTaskReader(ctx, &config)
	if err != nil {
		return view, err
	}
	defer reader.Close()
	candidate, err := reader.Candidate(ctx, r.project, candidateID)
	if err != nil {
		return view, err
	}
	if execution == "" || attempt == "" || candidate.RunID != execution || candidate.Projection == nil || candidate.Projection.Execution.AttemptID != attempt {
		return view, errors.New("last message differs from fixed candidate/Run/Attempt")
	}
	cli, err := config.runnerClient(ctx)
	if err != nil {
		return view, err
	}
	if !strings.EqualFold(filepath.Clean(candidate.StateDir), filepath.Clean(cli.StateDir)) {
		return view, errors.New("Runner state directory differs from the fixed candidate")
	}
	response, err := cli.Call(ctx, "last-message", []string{"--run-id", execution}, nil)
	if err != nil {
		return view, err
	}
	if json.Unmarshal(response.Data, &view) != nil || view.RunID != execution || view.AttemptID != attempt {
		return api.LastMessageView{}, errors.New("Runner last message target changed")
	}
	if m := view.Message; m != nil && (m.RunID != execution || m.AttemptID != attempt || m.SessionID == "" || m.SessionID != candidate.Projection.Execution.SessionID || m.TurnID == "" || m.ObservedAt.IsZero() || len(m.Text) > 4096 || (m.Kind != "assistant" && m.Kind != "tool")) {
		return api.LastMessageView{}, errors.New("Runner last message identity or payload invalid")
	}
	return view, nil
}
