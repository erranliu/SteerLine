package execution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	ImplementationDataVersion     = 1
	ImplementationDataMaxBytes    = 64 << 10
	ImplementationConfigPurpose   = "profile-config"
	ImplementationResourcePurpose = "execution-resources"
	ImplementationQuestionPurpose = "answer"
)

// ImplementationDataEnvelope is the only persisted carrier for implementation
// configuration, resource bindings and routed questions. Payload stays opaque.
type ImplementationDataEnvelope struct {
	Version               int    `json:"version"`
	CodecVersion          int    `json:"codec_version"`
	Purpose               string `json:"purpose"`
	ImplementationKey     string `json:"implementation_key"`
	ProjectID             string `json:"project_id"`
	ProfileID             string `json:"profile_id,omitempty"`
	AcceptedConfigVersion string `json:"accepted_config_version,omitempty"`
	ExecutionID           string `json:"execution_id,omitempty"`
	AttemptID             string `json:"attempt_id,omitempty"`
	Generation            string `json:"generation,omitempty"`
	ConfigDigest          string `json:"config_digest,omitempty"`
	WorkflowRunID         string `json:"workflow_run_id,omitempty"`
	TurnID                string `json:"turn_id,omitempty"`
	QuestionID            string `json:"question_id,omitempty"`
	Digest                string `json:"digest"`
	Payload               []byte `json:"-"`
}

type implementationDataWire struct {
	Version               int             `json:"version"`
	CodecVersion          int             `json:"codec_version"`
	Purpose               string          `json:"purpose"`
	ImplementationKey     string          `json:"implementation_key"`
	ProjectID             string          `json:"project_id"`
	ProfileID             string          `json:"profile_id,omitempty"`
	AcceptedConfigVersion string          `json:"accepted_config_version,omitempty"`
	ExecutionID           string          `json:"execution_id,omitempty"`
	AttemptID             string          `json:"attempt_id,omitempty"`
	Generation            string          `json:"generation,omitempty"`
	ConfigDigest          string          `json:"config_digest,omitempty"`
	WorkflowRunID         string          `json:"workflow_run_id,omitempty"`
	TurnID                string          `json:"turn_id,omitempty"`
	QuestionID            string          `json:"question_id,omitempty"`
	Digest                string          `json:"digest"`
	Payload               json.RawMessage `json:"payload"`
}

func NewImplementationDataEnvelope(e ImplementationDataEnvelope, payload []byte) (ImplementationDataEnvelope, error) {
	e.Version = ImplementationDataVersion
	e.Payload = append([]byte(nil), payload...)
	e.Digest = ""
	if err := e.validateFields(); err != nil {
		return ImplementationDataEnvelope{}, err
	}
	e.Digest = e.computeDigest()
	return e, nil
}

func (e ImplementationDataEnvelope) Validate() error {
	if err := e.validateFields(); err != nil {
		return err
	}
	if e.Digest == "" || e.Digest != e.computeDigest() {
		return errors.New("implementation data digest is invalid")
	}
	return nil
}

func (e ImplementationDataEnvelope) ValidateProfileIdentity(projectID, profileID, acceptedVersion string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.Purpose != ImplementationConfigPurpose || e.ProjectID != projectID || e.ProfileID != profileID || e.AcceptedConfigVersion != acceptedVersion {
		return errors.New("implementation profile data differs from its accepted project/profile version")
	}
	return nil
}

func (e ImplementationDataEnvelope) ValidateResourceIdentity(projectID, executionID, attemptID, generation, configDigest string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.Purpose != ImplementationResourcePurpose || e.ProjectID != projectID || e.ExecutionID != executionID || e.AttemptID != attemptID || e.Generation != generation || e.ConfigDigest != configDigest {
		return errors.New("implementation resource data differs from its fixed execution, generation or configuration")
	}
	return nil
}

func (e ImplementationDataEnvelope) ValidateQuestionIdentity(identity QuestionRouteIdentity) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.Purpose != ImplementationQuestionPurpose || e.ProjectID != identity.ProjectID || e.WorkflowRunID != identity.WorkflowRunID ||
		e.ExecutionID != identity.ExecutionID || e.AttemptID != identity.AttemptID || e.Generation != identity.Generation ||
		e.TurnID != identity.TurnID || e.QuestionID != identity.QuestionID {
		return errors.New("implementation question data differs from its fixed execution, generation, turn or question")
	}
	return nil
}

func (e ImplementationDataEnvelope) Clone() ImplementationDataEnvelope {
	e.Payload = append([]byte(nil), e.Payload...)
	return e
}

func (e ImplementationDataEnvelope) MarshalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(implementationDataWire{
		Version: e.Version, CodecVersion: e.CodecVersion, Purpose: e.Purpose,
		ImplementationKey: e.ImplementationKey, ProjectID: e.ProjectID,
		ProfileID: e.ProfileID, AcceptedConfigVersion: e.AcceptedConfigVersion,
		ExecutionID: e.ExecutionID, AttemptID: e.AttemptID, Generation: e.Generation,
		ConfigDigest: e.ConfigDigest, WorkflowRunID: e.WorkflowRunID, TurnID: e.TurnID,
		QuestionID: e.QuestionID, Digest: e.Digest, Payload: append(json.RawMessage(nil), e.Payload...),
	})
}

func (e *ImplementationDataEnvelope) UnmarshalJSON(data []byte) error {
	var wire implementationDataWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return errors.New("implementation data envelope contains trailing JSON")
		}
		return err
	}
	if len(wire.Payload) == 0 || !json.Valid(wire.Payload) {
		return errors.New("implementation data payload is not valid JSON")
	}
	*e = ImplementationDataEnvelope{
		Version: wire.Version, CodecVersion: wire.CodecVersion, Purpose: wire.Purpose,
		ImplementationKey: wire.ImplementationKey, ProjectID: wire.ProjectID,
		ProfileID: wire.ProfileID, AcceptedConfigVersion: wire.AcceptedConfigVersion,
		ExecutionID: wire.ExecutionID, AttemptID: wire.AttemptID, Generation: wire.Generation,
		ConfigDigest: wire.ConfigDigest, WorkflowRunID: wire.WorkflowRunID, TurnID: wire.TurnID,
		QuestionID: wire.QuestionID, Digest: wire.Digest, Payload: append([]byte(nil), wire.Payload...),
	}
	return e.Validate()
}

func (e ImplementationDataEnvelope) validateFields() error {
	if e.Version != ImplementationDataVersion || e.CodecVersion <= 0 || e.ImplementationKey == "" || e.ProjectID == "" {
		return errors.New("implementation data has an unsupported version or incomplete owner identity")
	}
	if len(e.Payload) == 0 || len(e.Payload) > ImplementationDataMaxBytes || !json.Valid(e.Payload) {
		return errors.New("implementation data payload is empty, invalid or exceeds 64 KiB")
	}
	switch e.Purpose {
	case ImplementationConfigPurpose:
		if e.ProfileID == "" || e.AcceptedConfigVersion == "" || e.ExecutionID != "" || e.AttemptID != "" || e.Generation != "" || e.ConfigDigest != "" || e.WorkflowRunID != "" || e.TurnID != "" || e.QuestionID != "" {
			return errors.New("implementation profile data has the wrong identity scope")
		}
	case ImplementationResourcePurpose:
		if e.ProfileID != "" || e.AcceptedConfigVersion != "" || e.ExecutionID == "" || e.AttemptID == "" || e.Generation == "" || e.ConfigDigest == "" || e.WorkflowRunID != "" || e.TurnID != "" || e.QuestionID != "" {
			return errors.New("implementation resource data has the wrong identity scope")
		}
	case ImplementationQuestionPurpose:
		if e.ProfileID != "" || e.AcceptedConfigVersion != "" || e.ExecutionID == "" || e.AttemptID == "" || e.Generation == "" || e.ConfigDigest != "" || e.WorkflowRunID == "" || e.TurnID == "" || e.QuestionID == "" {
			return errors.New("implementation question data has the wrong identity scope")
		}
	default:
		return fmt.Errorf("unsupported implementation data purpose %q", e.Purpose)
	}
	return nil
}

func (e ImplementationDataEnvelope) computeDigest() string {
	material := struct {
		Version               int    `json:"version"`
		CodecVersion          int    `json:"codec_version"`
		Purpose               string `json:"purpose"`
		ImplementationKey     string `json:"implementation_key"`
		ProjectID             string `json:"project_id"`
		ProfileID             string `json:"profile_id,omitempty"`
		AcceptedConfigVersion string `json:"accepted_config_version,omitempty"`
		ExecutionID           string `json:"execution_id,omitempty"`
		AttemptID             string `json:"attempt_id,omitempty"`
		Generation            string `json:"generation,omitempty"`
		ConfigDigest          string `json:"config_digest,omitempty"`
		WorkflowRunID         string `json:"workflow_run_id,omitempty"`
		TurnID                string `json:"turn_id,omitempty"`
		QuestionID            string `json:"question_id,omitempty"`
		Payload               []byte `json:"payload"`
	}{e.Version, e.CodecVersion, e.Purpose, e.ImplementationKey, e.ProjectID, e.ProfileID,
		e.AcceptedConfigVersion, e.ExecutionID, e.AttemptID, e.Generation, e.ConfigDigest,
		e.WorkflowRunID, e.TurnID, e.QuestionID, e.Payload}
	encoded, _ := json.Marshal(material)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
