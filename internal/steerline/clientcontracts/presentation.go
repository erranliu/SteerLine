package clientcontracts

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/execution"
)

// ProjectPresentation is a read-only description of accepted configuration.
// It never carries implementation payloads, factories, runtime or services.
type ProjectPresentation struct {
	ServiceInstruction                                                             string
	NativeWorkflow                                                                 bool
	ConfigDigest, TaskAuthority, WorkflowProtocol, ShutdownGrace, DefaultProfileID string
	Profiles                                                                       []ExecutionProfileSummary
}
type ExecutionProfileSummary struct {
	ID, Name, Host, Agent, Digest, AcceptedConfigVersion, UnavailableReason string
	ExecutionPolicy                                                         execution.ExecutionPolicy
	PublicationPolicy                                                       execution.PublicationPolicy
	Publication                                                             contracts.PublicationSettings
	ImplementationDisplay                                                   []string
}

func CloneProjectRef(ref ProjectRef) ProjectRef {
	ref.Presentation.Profiles = append([]ExecutionProfileSummary(nil), ref.Presentation.Profiles...)
	for i := range ref.Presentation.Profiles {
		p := &ref.Presentation.Profiles[i]
		p.ImplementationDisplay = append([]string(nil), p.ImplementationDisplay...)
		p.Publication.AuthenticationRefs = append([]string(nil), p.Publication.AuthenticationRefs...)
		for _, command := range []*contracts.Command{&p.Publication.Git, &p.Publication.Provider.Query, &p.Publication.Provider.Close} {
			command.Args = append([]string(nil), command.Args...)
		}
	}
	return ref
}
