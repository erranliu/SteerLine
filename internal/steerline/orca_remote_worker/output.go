package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

func (b Client) ReadOutput(ctx context.Context, ref Ref, cursor string, limit int) (Output, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return Output{}, err
	}
	if limit <= 0 || limit > 1000 {
		return Output{}, &Error{Code: BackendProtocolInvalid, Diagnostic: "supervised output limit must be 1–1000 rows"}
	}
	args := []string{"orchestration", "worker-read", "--dispatch", ref.External.Dispatch, "--source", "auto", "--limit", strconv.Itoa(limit), "--json"}
	if cursor != "" {
		args = append(args, "--cursor", cursor)
	}
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), args...)
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return Output{}, &Error{Code: BackendUnreachable, Diagnostic: "supervised worker output read failed", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID {
		return Output{}, &Error{Code: BackendIdentityMismatch, Diagnostic: "worker output came from a different coordinator runtime"}
	}
	var result struct {
		DispatchID      string          `json:"dispatchId"`
		Source          string          `json:"source"`
		Cursor          string          `json:"cursor"`
		NextCursor      string          `json:"nextCursor"`
		SourceChanged   bool            `json:"sourceChanged"`
		ContentComplete bool            `json:"contentComplete"`
		Clipping        []string        `json:"clipping"`
		FallbackReason  string          `json:"fallbackReason"`
		Transcript      json.RawMessage `json:"transcript"`
		Terminal        json.RawMessage `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return Output{}, &Error{Code: BackendProtocolInvalid, Diagnostic: "supervised worker output response is malformed", Cause: err}
	}
	if result.DispatchID != ref.External.Dispatch || result.Source == "" {
		return Output{}, &Error{Code: BackendIdentityMismatch, Diagnostic: "supervised worker output source does not match the fixed Dispatch"}
	}
	next := result.Cursor
	if result.NextCursor != "" {
		next = result.NextCursor
	}
	if next == "" {
		return Output{}, &Error{Code: BackendProtocolInvalid, Diagnostic: "supervised worker output omitted its source cursor"}
	}
	if len(env.Result) > 1<<20 {
		return Output{}, &Error{Code: BackendProtocolInvalid, Diagnostic: "supervised worker output exceeded the 1 MiB response bound"}
	}
	return Output{Data: append([]byte(nil), env.Result...), Source: result.Source, ObservedAt: now(), NextCursor: next, Gap: len(result.Clipping) > 0, SourceChanged: result.SourceChanged || result.FallbackReason == "source_changed"}, nil
}
