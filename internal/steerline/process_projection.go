package steerline

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func publicProcessProbe(identity *executionbackend.ProcessIdentity) productapi.ProcessProbe {
	facts := resourceprocess.Probe(identity)
	return productapi.ProcessProbe{State: productapi.ProcessProbeState(facts.State), Reason: facts.Reason}
}
