package orcaworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type settledOutputEvidence struct {
	Version  int            `json:"v"`
	Binding  string         `json:"binding"`
	Digest   string         `json:"digest"`
	Outcome  string         `json:"outcome"`
	Material OutputMaterial `json:"material"`
}

var outputEvidenceWrites sync.Mutex

func readSettledOutput(path string, binding outputBinding) (*OutputMaterial, error) {
	if err := noLinks(filepath.Dir(path), filepath.Base(path)+".settled"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.Open(path + ".settled")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2*executionmodel.MaxConversationBodyBytes+65536 {
		return nil, errors.New("settled output evidence is not a bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, 2*executionmodel.MaxConversationBodyBytes+65537))
	if err != nil {
		return nil, err
	}
	var evidence settledOutputEvidence
	if err = executionbackend.DecodeStrictJSON(raw, &evidence, "settled output evidence has trailing data"); err != nil {
		return nil, err
	}
	material := evidence.Material
	if evidence.Version != 1 || evidence.Binding != outputBindingIdentity(binding) || evidence.Digest != digestValue(material) || evidence.Outcome != "completed" && evidence.Outcome != "failed" || material.WorkflowRunID != binding.WorkflowID || material.TurnID != binding.TurnID || material.TaskID != binding.Process.Task || material.DispatchID != binding.Process.Dispatch || material.Body == "" || len(material.Body) > executionmodel.MaxConversationBodyBytes {
		return nil, errors.New("settled output evidence changed its original binding or body")
	}
	material.Outcome = evidence.Outcome
	return &material, nil
}

func freezeSettledOutput(path string, binding outputBinding, material OutputMaterial) error {
	evidence := settledOutputEvidence{Version: 1, Binding: outputBindingIdentity(binding), Digest: digestValue(material), Outcome: material.Outcome, Material: material}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	outputEvidenceWrites.Lock()
	defer outputEvidenceWrites.Unlock()
	if existing, readErr := os.ReadFile(path + ".settled"); readErr == nil {
		if !bytes.Equal(existing, raw) {
			return errors.New("original settled output evidence is immutable")
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "orca-output-evidence-")
	if err != nil {
		return err
	}
	pending := temporary.Name()
	defer os.Remove(pending)
	if _, err = temporary.Write(raw); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	return os.Rename(pending, path+".settled")
}

func (l *ExecutionLifecycle) freezeOriginalOutput(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, turn NativeTurnState) error {
	if l.setup.RunDirectory == "" {
		return nil
	}
	path := filepath.Join(l.setup.RunDirectory, "runs", snapshot.Session.WorkflowRunID, "turns", snapshot.Session.ExecutionID, "orca-output."+turn.TurnID+".json")
	binding, err := readOutputBinding(path)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("original settled worker lacks its frozen output binding")
	}
	if err != nil {
		return err
	}
	cached, err := readSettledOutput(path, binding)
	if err != nil || cached != nil {
		return err
	}
	material, err := readOutputMaterial(ctx, l.client.MetaCall, binding)
	if err != nil {
		return err
	}
	if material == nil {
		return errors.New("original settled worker lacks complete output material; no canonical final is available")
	}
	return freezeSettledOutput(path, binding, *material)
}
