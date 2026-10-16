package runnerclient

import (
	"context"
	"encoding/json"
)

type NativeSessionTarget struct {
	RunID      string `json:"run_id"`
	AttemptID  string `json:"attempt_id"`
	SessionID  string `json:"session_id"`
	Generation string `json:"generation"`
}

func (c Client) OpenNativeSession(ctx context.Context, target NativeSessionTarget) error {
	if target.RunID == "" || target.AttemptID == "" || target.SessionID == "" || target.Generation == "" {
		return &Failure{Code: "invalid_arguments", Message: "Exact original native session target required", Exit: 2}
	}
	response, err := c.Call(ctx, "open-native-session", []string{"--run-id", target.RunID, "--attempt-id", target.AttemptID, "--session-id", target.SessionID, "--generation", target.Generation}, nil)
	if err != nil {
		return err
	}
	var result struct {
		NativeSessionTarget
		Opened bool `json:"opened"`
	}
	if json.Unmarshal(response.Data, &result) != nil || result.NativeSessionTarget != target || !result.Opened {
		return &Failure{Code: "request_outcome_unknown", Message: "Native viewer result identity is unverified; do not replay", Exit: 6, Unknown: true}
	}
	return nil
}
