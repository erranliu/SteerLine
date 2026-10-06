package host

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func publicProcessProbe(factsPort executionbackend.ProcessFactsPort, identity *executionbackend.ProcessIdentity) productapi.ProcessProbe {
	if factsPort == nil {
		return productapi.ProcessProbe{State: productapi.ProcessUnknown, Reason: "application process-facts port is required"}
	}
	facts := factsPort.Probe(identity)
	return productapi.ProcessProbe{State: productapi.ProcessProbeState(facts.State), Reason: facts.Reason}
}
