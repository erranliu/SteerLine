package steerline

import (
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

const WorkerFlowRuntimeSchema = contracts.WorkerFlowRuntimeSchema

type Repository = contracts.Repository
type ServiceCheckpoint = contracts.ServiceCheckpoint
type Runtime = contracts.Runtime
type ServiceState = contracts.ServiceState
type ControlState = contracts.ControlState
type ControlOperation = contracts.ControlOperation
type ControlBlocker = contracts.ControlBlocker
type TaskSnapshot = contracts.TaskSnapshot
type TaskCandidate = contracts.TaskCandidate
type Run = contracts.Run
type LegacyBeadsContext = contracts.LegacyBeadsContext
type ManualStopConfirmation = contracts.ManualStopConfirmation
type DecisionRequest = contracts.DecisionRequest
type DecisionState = contracts.DecisionState
type DecisionAnswer = contracts.DecisionAnswer
type PRState = contracts.PRState

func ptr[T any](v T) *T { return &v }
func now() time.Time    { return time.Now().UTC() }

type HostActionTarget = contracts.HostActionTarget
