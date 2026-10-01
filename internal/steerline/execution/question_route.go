package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

func NewQuestionRoute(implementationKey string, codecVersion int, identity QuestionRouteIdentity, payload []byte) (QuestionRoute, error) {
	route := QuestionRoute{
		Version: QuestionRouteVersion, ImplementationKey: implementationKey, CodecVersion: codecVersion,
		Purpose: QuestionRouteAnswerPurpose, ProjectID: identity.ProjectID, WorkflowRunID: identity.WorkflowRunID,
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID, Generation: identity.Generation,
		TurnID: identity.TurnID, QuestionID: identity.QuestionID, Payload: append([]byte(nil), payload...),
	}
	if err := route.validateFields(); err != nil {
		return QuestionRoute{}, err
	}
	route.Digest = route.computeDigest()
	return route, nil
}

func (r QuestionRoute) Validate() error {
	if err := r.validateFields(); err != nil {
		return err
	}
	if r.Digest == "" || r.Digest != r.computeDigest() {
		return errors.New("question route digest is invalid")
	}
	return nil
}

func (r QuestionRoute) ValidateIdentity(identity QuestionRouteIdentity) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.ProjectID != identity.ProjectID || r.WorkflowRunID != identity.WorkflowRunID ||
		r.ExecutionID != identity.ExecutionID || r.AttemptID != identity.AttemptID ||
		r.Generation != identity.Generation || r.TurnID != identity.TurnID || r.QuestionID != identity.QuestionID {
		return errors.New("question route differs from its fixed execution, generation, turn or question")
	}
	return nil
}

func (r QuestionRoute) Clone() QuestionRoute {
	r.Payload = append([]byte(nil), r.Payload...)
	return r
}

func (r QuestionRoute) validateFields() error {
	if r.Version != QuestionRouteVersion || r.CodecVersion <= 0 || strings.TrimSpace(r.ImplementationKey) == "" ||
		r.Purpose != QuestionRouteAnswerPurpose || r.ProjectID == "" || r.WorkflowRunID == "" || r.ExecutionID == "" ||
		r.AttemptID == "" || r.Generation == "" || r.TurnID == "" || r.QuestionID == "" {
		return errors.New("question route has an unsupported version, purpose or incomplete identity")
	}
	if len(r.Payload) == 0 || len(r.Payload) > QuestionRouteMaxBytes {
		return errors.New("question route payload is empty or exceeds 64 KiB")
	}
	return nil
}

func (r QuestionRoute) computeDigest() string {
	material := struct {
		Version           int    `json:"version"`
		ImplementationKey string `json:"implementation_key"`
		CodecVersion      int    `json:"codec_version"`
		Purpose           string `json:"purpose"`
		ProjectID         string `json:"project_id"`
		WorkflowRunID     string `json:"workflow_run_id"`
		ExecutionID       string `json:"execution_id"`
		AttemptID         string `json:"attempt_id"`
		Generation        string `json:"generation"`
		TurnID            string `json:"turn_id"`
		QuestionID        string `json:"question_id"`
		Payload           []byte `json:"payload"`
	}{r.Version, r.ImplementationKey, r.CodecVersion, r.Purpose, r.ProjectID, r.WorkflowRunID, r.ExecutionID,
		r.AttemptID, r.Generation, r.TurnID, r.QuestionID, r.Payload}
	encoded, _ := json.Marshal(material)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
