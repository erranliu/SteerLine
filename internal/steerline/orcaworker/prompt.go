package orcaworker

import (
	"context"
	"errors"
	"strings"
	"time"
)

// StagePrompt sends fixed chunks once. A transport error is not evidence that
// a chunk was rejected; only its exact output marker confirms acceptance.
func StagePrompt(ctx context.Context, call MetaCall, identity TerminalIdentity, timeout time.Duration, executionID, turnID, prompt string, mapError func(error) error) error {
	translate := func(err error) error {
		if err != nil && mapError != nil {
			return mapError(err)
		}
		return err
	}
	for index, chunk := range PromptChunks(prompt) {
		command, marker, err := ChunkCommand(executionID, turnID, index, chunk)
		if err != nil {
			return err
		}
		cursor, err := CurrentCursor(ctx, call, identity.Handle, identity.RuntimeID)
		if err != nil {
			return translate(err)
		}
		_, sendErr := SendCommand(ctx, call, identity, command)
		if err := waitPromptMarker(ctx, call, identity, cursor, marker, timeout, translate); err != nil {
			return errors.Join(translate(sendErr), err)
		}
	}
	return nil
}

func waitPromptMarker(ctx context.Context, call MetaCall, identity TerminalIdentity, cursor int64, marker string, timeout time.Duration, translate func(error) error) error {
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		batch, err := ReadOutput(deadline, call, identity, cursor)
		if err != nil {
			return translate(err)
		}
		for _, raw := range batch.Rows {
			if strings.TrimSpace(raw) == marker {
				return nil
			}
		}
		cursor = batch.NextCursor
		select {
		case <-deadline.Done():
			return translate(&PlatformError{Code: "outcome_unknown", Diagnostic: "Orca did not confirm the fixed prompt chunk", Cause: deadline.Err()})
		case <-ticker.C:
		}
	}
}
