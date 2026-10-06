package process

import executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"

type ServiceFacts struct{}

func NewServiceFacts() executionbackend.ProcessFactsPort { return ServiceFacts{} }

func (ServiceFacts) Probe(identity *executionbackend.ProcessIdentity) executionbackend.ProcessProbeFacts {
	facts := Probe(identity)
	return executionbackend.ProcessProbeFacts{State: facts.State, Reason: facts.Reason}
}
