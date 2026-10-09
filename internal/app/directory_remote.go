package app

import (
	"context"
	"github.com/erranliu/SteerLine/internal/execution"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func buildRemotePolicyResource(_ context.Context, r execution.PolicyResourceBuildRequest) (execution.PolicyResourceBuildResult, error) {
	envelope, err := remote.BuildNativeResource(remote.PolicyResourceIdentity{ProjectID: r.Resource.ProjectID, ExecutionID: r.Resource.ExecutionID, AttemptID: r.Resource.AttemptID, Generation: r.Resource.Generation, ConfigDigest: r.Resource.ConfigDigest}, r.ProfileID, r.AcceptedConfigVersion, r.Workspace, r.Agent, r.ProfileData)
	return execution.PolicyResourceBuildResult{Envelope: envelope, WriteRoots: []string{r.Workspace}}, err
}
func verifyRemotePolicyResource(envelope executionbackend.ImplementationDataEnvelope, r execution.PolicyResourceIdentity) error {
	_, err := remote.DecodeNativeResource(envelope, remote.PolicyResourceIdentity{ProjectID: r.ProjectID, ExecutionID: r.ExecutionID, AttemptID: r.AttemptID, Generation: r.Generation, ConfigDigest: r.ConfigDigest})
	return err
}
