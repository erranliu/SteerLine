package backend

import (
	"encoding/hex"
	"errors"
	"regexp"
)

var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func StableExecutionID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func StableOperationID(value string) bool {
	if value == "" || len(value) > 160 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func ValidateExecutionRef(ref ExecutionRef) error {
	if ref.Version != ExecutionRecordVersion || !StableExecutionID(ref.RunID) || !StableExecutionID(ref.AttemptID) || ref.Backend == "" || ref.Protocol == "" {
		return errors.New("incomplete execution binding")
	}
	if ref.Target.ID == "" || ref.Agent.Name == "" || !commitHash.MatchString(ref.BaseCommit) {
		return errors.New("execution binding lacks a verified target, agent or workspace")
	}
	if ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || ref.External.Kind == "" {
		return errors.New("execution binding lacks workspace or resource identity")
	}
	return nil
}

func ValidateExecutionOperation(op Operation) error {
	if op.Version != ExecutionRecordVersion || !StableOperationID(op.ID) || !StableExecutionID(op.RunID) || !StableExecutionID(op.AttemptID) || len(op.PayloadDigest) != 64 || op.Action == "" || op.Backend == "" || op.Protocol == "" || op.Target.ID == "" {
		return errors.New("incomplete operation record")
	}
	if _, err := hex.DecodeString(op.PayloadDigest); err != nil {
		return errors.New("invalid operation payload digest")
	}
	if op.Action != "start" && op.Action != "stop" && op.Action != "reply" {
		return errors.New("unsupported execution operation action")
	}
	switch op.Status {
	case OperationPending, OperationInFlight, OperationAccepted, OperationConfirmed, OperationRejected, OperationUnknown:
	default:
		return errors.New("invalid operation status")
	}
	if op.Ref != nil {
		if err := ValidateExecutionRef(*op.Ref); err != nil {
			return err
		}
		if op.Ref.RunID != op.RunID || op.Ref.AttemptID != op.AttemptID || op.Ref.Backend != op.Backend || op.Ref.Protocol != op.Protocol || op.Ref.Target != op.Target {
			return errors.New("operation ref conflicts with fixed identity")
		}
	}
	return nil
}
