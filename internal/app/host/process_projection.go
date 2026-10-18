package host

import (
	"github.com/erranliu/SteerLine/internal/hostcontract"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func publicProcessProbe(factsPort hostcontract.ProcessFactsPort, identity *hostcontract.ProcessIdentity) productapi.ProcessProbe {
	if factsPort == nil {
		return productapi.ProcessProbe{State: productapi.ProcessUnknown, Reason: "application process-facts port is required"}
	}
	facts := factsPort.Probe(identity)
	return productapi.ProcessProbe{State: productapi.ProcessProbeState(facts.State), Reason: facts.Reason}
}
