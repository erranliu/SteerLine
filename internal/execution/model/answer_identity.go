package model

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
)

func SameDecisionAnswer(a, b DecisionAnswer) bool {
	return a.DecisionID == b.DecisionID && a.RunID == b.RunID && a.SourceAttemptID == b.SourceAttemptID && a.ExpectedRecoveryVersion == b.ExpectedRecoveryVersion && strings.TrimSpace(a.OptionID) == strings.TrimSpace(b.OptionID) && strings.TrimSpace(a.Note) == strings.TrimSpace(b.Note) && answerFingerprint(a.Answers) == answerFingerprint(b.Answers)
}

func answerFingerprint(v any) [32]byte { b, _ := json.Marshal(v); return sha256.Sum256(b) }
