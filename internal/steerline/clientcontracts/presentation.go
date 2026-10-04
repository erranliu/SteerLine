package clientcontracts

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type ProjectPresentation struct {
	ServiceInstruction                                                             string
	NativeWorkflow                                                                 bool
	ConfigDigest, TaskAuthority, WorkflowProtocol, ShutdownGrace, DefaultProfileID string
	Profiles                                                                       []ExecutionProfileSummary
}

type ExecutionProfileSummary struct {
	ID, Name, Host, Agent, Digest, AcceptedConfigVersion, UnavailableReason string
	ExecutionPolicy                                                         executionbackend.ExecutionPolicy
	PublicationPolicy                                                       executionbackend.PublicationPolicy
	Publication                                                             executionmodel.PublicationSettings
	ImplementationDisplay                                                   []string
}

func CloneProjectRef(ref ProjectRef) ProjectRef {
	ref.Presentation.Profiles = append([]ExecutionProfileSummary(nil), ref.Presentation.Profiles...)
	for i := range ref.Presentation.Profiles {
		p := &ref.Presentation.Profiles[i]
		p.ImplementationDisplay = append([]string(nil), p.ImplementationDisplay...)
		p.Publication.AuthenticationRefs = append([]string(nil), p.Publication.AuthenticationRefs...)
		for _, command := range []*executionbackend.Command{&p.Publication.Git, &p.Publication.Provider.Query, &p.Publication.Provider.Close} {
			command.Args = append([]string(nil), command.Args...)
		}
	}
	return ref
}
