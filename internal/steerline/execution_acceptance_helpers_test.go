package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// The bounded raw output reader is a test utility for generic process evidence.

func (s *Service) requireBackendCapabilities(ctx context.Context, backend executionapi.ExecutionProbe, names ...string) (executionapi.CapabilityReport, error) {
	return s.requireBackendCapabilitiesFor(ctx, backend, targetFor(s.Config), agentFor(s.Config), names...)
}

type localOutputCursor struct {
	Version   int    `json:"v"`
	RunID     string `json:"r"`
	AttemptID string `json:"a"`
	Offset    int64  `json:"o"`
	Prefix    string `json:"p,omitempty"`
}

func (b localExecutionBackend) ReadOutput(ctx context.Context, ref executionapi.ExecutionRef, cursor string, limit int) (executionapi.OutputBatch, error) {
	if ref.Backend != executionapi.BackendLocal || ref.Protocol != executionapi.ProtocolLocalProcess || ref.Target != targetFor(b.config) || !stableExecutionID(ref.RunID) || !stableExecutionID(ref.AttemptID) {
		return executionapi.OutputBatch{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "output ref does not match the fixed local execution"}
	}
	if limit <= 0 || limit > 1<<20 {
		return executionapi.OutputBatch{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "output limit must be between 1 and 1048576 bytes"}
	}
	path := filepath.Join(b.config.RunDir, "runs", ref.RunID)
	if ref.AttemptID != ref.RunID {
		path = filepath.Join(path, "attempts", ref.AttemptID)
	}
	path = filepath.Join(path, "events.jsonl")
	select {
	case <-ctx.Done():
		return executionapi.OutputBatch{}, ctx.Err()
	default:
	}
	offset := int64(0)
	oldPrefix := ""
	if cursor != "" {
		token, decodeErr := base64.RawURLEncoding.DecodeString(cursor)
		if decodeErr != nil {
			return executionapi.OutputBatch{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "invalid output cursor", Cause: decodeErr}
		}
		var c localOutputCursor
		if decodeErr = json.Unmarshal(token, &c); decodeErr != nil || c.Version != 1 || c.RunID != ref.RunID || c.AttemptID != ref.AttemptID || c.Offset < 0 {
			return executionapi.OutputBatch{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "output cursor is malformed or belongs to another execution"}
		}
		offset, oldPrefix = c.Offset, c.Prefix
	}
	f, err := os.Open(path)
	if err != nil {
		return executionapi.OutputBatch{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return executionapi.OutputBatch{}, err
	}
	batch := executionapi.OutputBatch{Source: "local-jsonl", RetainedFrom: "0", ObservedAt: now()}
	if offset > info.Size() {
		batch.Gap = true
		offset = info.Size()
	}
	if oldPrefix != "" {
		prefix, prefixErr := readOutputPrefix(f, offset)
		if prefixErr != nil {
			return executionapi.OutputBatch{}, prefixErr
		}
		if prefix != oldPrefix {
			batch.SourceChanged = true
			offset = 0
		}
	}
	end := offset + int64(limit)
	if end > info.Size() {
		end = info.Size()
	}
	chunk := make([]byte, end-offset)
	if len(chunk) > 0 {
		n, readErr := f.ReadAt(chunk, offset)
		if readErr != nil && readErr != io.EOF {
			return executionapi.OutputBatch{}, readErr
		}
		chunk = chunk[:n]
	}
	batch.Data = bytes.Clone(chunk)
	end = offset + int64(len(chunk))
	prefix, prefixErr := readOutputPrefix(f, end)
	if prefixErr != nil {
		return executionapi.OutputBatch{}, prefixErr
	}
	next := localOutputCursor{Version: 1, RunID: ref.RunID, AttemptID: ref.AttemptID, Offset: end, Prefix: prefix}
	encoded, _ := json.Marshal(next)
	batch.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	return batch, nil
}

func readOutputPrefix(f *os.File, offset int64) (string, error) {
	n := offset
	if n > 64<<10 {
		n = 64 << 10
	}
	if n < 0 {
		n = 0
	}
	first := make([]byte, n)
	last := make([]byte, n)
	if n > 0 {
		read, err := f.ReadAt(first, 0)
		if err != nil && err != io.EOF {
			return "", err
		}
		first = first[:read]
		read, err = f.ReadAt(last, offset-n)
		if err != nil && err != io.EOF {
			return "", err
		}
		last = last[:read]
	}
	hash := sha256.New()
	_, _ = hash.Write(first)
	_, _ = hash.Write(last)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
