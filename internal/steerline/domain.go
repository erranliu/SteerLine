package steerline

import (
	"regexp"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

type TaskDeliveryRecord = contracts.TaskDeliveryRecord

type TaskDeliveryEvidence = contracts.TaskDeliveryEvidence

var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
