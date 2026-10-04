package contracts

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type LegacyCompletionConfig struct {
	Command executionbackend.Command `json:"command"`
	Timeout string                   `json:"timeout"`
}
