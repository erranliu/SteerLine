package steerline

import (
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

type Repository = contracts.Repository

type Runtime = contracts.Runtime

type ServiceState = contracts.ServiceState

type ControlState = contracts.ControlState

type ControlOperation = contracts.ControlOperation

type ControlBlocker = contracts.ControlBlocker

type TaskSnapshot = contracts.TaskSnapshot

type TaskCandidate = contracts.TaskCandidate

type Run = contracts.Run

type LegacyBeadsContext = contracts.LegacyBeadsContext

func ptr[T any](v T) *T { return &v }

func now() time.Time { return time.Now().UTC() }
