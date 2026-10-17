package process

import "github.com/erranliu/SteerLine/internal/hostcontract"

type ServiceFacts struct{}

func NewServiceFacts() hostcontract.ProcessFactsPort { return ServiceFacts{} }

func (ServiceFacts) Probe(identity *hostcontract.ProcessIdentity) hostcontract.ProcessProbeFacts {
	facts := Probe(identity)
	return hostcontract.ProcessProbeFacts{State: facts.State, Reason: facts.Reason}
}
