package steerline

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func policyResourceDigest(p contracts.PolicyResources) string {
	p.Digest = ""
	return digestValue(p)
}

func verifyPolicyResources(c Config, p *contracts.PolicyResources, profileDigest, workspace string, records ...*ExecutionRecord) error {
	if p == nil || p.Version != 3 || p.Digest != policyResourceDigest(*p) || p.ProfileDigest != profileDigest || p.Workspace != workspace || len(p.TemporaryRoots) != 0 || p.ImplementationData.ConfigDigest != profileDigest {
		return errors.New("execution policy resource snapshot differs from original settings or identity")
	}
	identity := PolicyResourceIdentity{
		ProjectID: p.ImplementationData.ProjectID, ExecutionID: p.ImplementationData.ExecutionID,
		AttemptID: p.ImplementationData.AttemptID, Generation: p.ImplementationData.Generation,
		ConfigDigest: profileDigest,
	}
	if len(records) > 0 && records[0] != nil {
		record := records[0]
		identity = PolicyResourceIdentity{ProjectID: record.ProjectIdentity, ExecutionID: record.ExecutionID,
			AttemptID: record.CurrentAttemptID, Generation: record.Generation, ConfigDigest: record.ConfigurationDigest}
	}
	entry, err := registrationFor(c)
	if err != nil {
		return err
	}
	if entry.VerifyPolicyResource == nil {
		return errors.New("execution implementation has no policy resource codec")
	}
	if err := p.ImplementationData.ValidateResourceIdentity(identity.ProjectID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.ConfigDigest); err != nil {
		return err
	}
	if err := entry.VerifyPolicyResource(p.ImplementationData, identity); err != nil {
		return fmt.Errorf("execution policy resource codec rejected its binding: %w", err)
	}
	if c.ExecutionPolicy.Filesystem.WriteScope == "host" && len(p.WriteRoots) != 0 {
		return errors.New("full access policy must not claim bounded writable roots")
	}
	return nil
}

func bindPolicyResources(ctx context.Context, c Config, e *ExecutionRecord, workspace string, linkedArgs []string) error {
	manualRestart := awaitingRecoveryResources(e)
	if e.PolicyResources == nil && (e.Process != nil || e.SessionID != "") && !manualRestart {
		return errors.New("original execution policy resource snapshot is missing; preserve the external session")
	}
	if err := validateWorkflowConfig(&c, nil); err != nil {
		return err
	}
	if e.Generation == "" {
		e.Generation = newID()
	}
	profileDigest := c.fixedProfileDigest
	if profileDigest == "" {
		profileDigest = c.projectExecutionProfile().Digest()
	}
	if e.ConfigurationDigest != "" && e.ConfigurationDigest != profileDigest {
		return errors.New("execution configuration digest differs from the accepted profile")
	}
	e.ConfigurationDigest = profileDigest
	entry, err := registrationFor(c)
	if err != nil {
		return err
	}
	if entry.BuildPolicyResource == nil {
		return errors.New("execution implementation has no policy resource builder")
	}
	identity := PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID,
		AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: profileDigest}
	bound, err := entry.BuildPolicyResource(ctx, PolicyResourceBuildRequest{
		ProfileData: cloneImplementationData(c.ImplementationData), ProjectID: configurationProjectIdentity(c),
		ProfileID: c.selectedProfileID, AcceptedConfigVersion: c.acceptedConfigVersion(), Resource: identity,
		Policy: c.ExecutionPolicy, Host: c.hostName(), Agent: c.agentName(), Workspace: workspace,
		LinkedWorktreeArgs: append([]string(nil), linkedArgs...), RepositoryPath: c.Repository,
		CommandTimeout: duration(c.CommandTimeout),
	})
	if err != nil {
		return err
	}
	if err := bound.Envelope.ValidateResourceIdentity(identity.ProjectID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.ConfigDigest); err != nil {
		return err
	}
	if err := entry.VerifyPolicyResource(bound.Envelope.Clone(), identity); err != nil {
		return err
	}
	evidence := contracts.PolicyResources{Version: 3, ProfileDigest: profileDigest, Workspace: workspace,
		WriteRoots: append([]string(nil), bound.WriteRoots...), TemporaryRoots: []string{}, ImplementationData: bound.Envelope.Clone()}
	evidence.Digest = policyResourceDigest(evidence)
	if e.PolicyResources != nil {
		if !reflect.DeepEqual(*e.PolicyResources, evidence) {
			return errors.New("implementation resources or settings expanded or changed after binding")
		}
		return nil
	}
	e.PolicyResources = &evidence
	return nil
}
