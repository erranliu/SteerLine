package steerline

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func policyNativeVersion(ctx context.Context, c Config) (string, error) {
	if err := validateWorkflowConfig(&c, nil); err != nil {
		return "", err
	}
	native, err := codexworker.ProfileExecutable(c.ImplementationData, codexworker.ProfileConfigIdentity{
		ProjectID: configurationProjectIdentity(c), ProfileID: c.selectedProfileID,
		AcceptedConfigVersion: c.acceptedConfigVersion(),
	})
	if err != nil {
		return "", err
	}
	raw, err := (Runner{Dir: c.Repository, Timeout: min(positiveDuration(c.CommandTimeout, 10*time.Second), 10*time.Second)}).Output(ctx, Command{Executable: native}, "--version")
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(raw))
	return version, codexworker.VerifyPolicyVersion(version)
}
func policyResourceDigest(p contracts.PolicyResources) string { p.Digest = ""; return digestValue(p) }
func verifyPolicyResources(p *contracts.PolicyResources, profileDigest, workspace string, records ...*ExecutionRecord) error {
	if p == nil || p.Version != 3 || p.Digest != policyResourceDigest(*p) || p.ProfileDigest != profileDigest || p.Workspace != workspace || len(p.TemporaryRoots) != 0 || p.ImplementationData.ConfigDigest != profileDigest {
		return errors.New("execution policy resource snapshot differs from original settings or identity")
	}
	identity := codexworker.PolicyResourceIdentity{ProjectID: p.ImplementationData.ProjectID, ExecutionID: p.ImplementationData.ExecutionID, AttemptID: p.ImplementationData.AttemptID, Generation: p.ImplementationData.Generation, ConfigDigest: profileDigest}
	if len(records) > 0 && records[0] != nil {
		record := records[0]
		identity = codexworker.PolicyResourceIdentity{ProjectID: record.ProjectIdentity, ExecutionID: record.ExecutionID, AttemptID: record.CurrentAttemptID, Generation: record.Generation, ConfigDigest: record.ConfigurationDigest}
	}
	if _, err := codexworker.DecodePolicyResourceEnvelope(p.ImplementationData, identity); err != nil {
		return fmt.Errorf("execution policy resource codec rejected its binding: %w", err)
	}
	return nil
}
func bindPolicyResources(ctx context.Context, c Config, e *ExecutionRecord, workspace string, linkedArgs []string) error {
	manualRestart := e.Process == nil && e.RecoveryState == "resuming"
	if e.PolicyResources == nil && (e.Process != nil || e.Generation != "") && !manualRestart {
		return errors.New("original execution policy resource snapshot is missing; preserve the native session")
	}
	if err := validateWorkflowConfig(&c, nil); err != nil {
		return err
	}
	if e.Generation == "" {
		e.Generation = newID()
	}
	profileDigest := c.fixedProfileDigest
	// A named profile retains its selected name/ID in the full configuration digest.
	if profileDigest == "" {
		profileDigest = c.projectExecutionProfile().Digest()
	}
	if e.ConfigurationDigest != "" && e.ConfigurationDigest != profileDigest {
		return errors.New("execution configuration digest differs from the accepted profile")
	}
	e.ConfigurationDigest = profileDigest
	identity := codexworker.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: profileDigest}
	bound, err := codexworker.BuildPolicyResourceEnvelope(ctx, codexworker.PolicyResourceBuildRequest{
		ProfileData:      c.ImplementationData,
		ProfileIdentity:  codexworker.ProfileConfigIdentity{ProjectID: configurationProjectIdentity(c), ProfileID: c.selectedProfileID, AcceptedConfigVersion: c.acceptedConfigVersion()},
		ResourceIdentity: identity, Policy: c.ExecutionPolicy, Host: c.hostName(), Agent: c.agentName(),
		GOOS: runtime.GOOS, Workspace: workspace, LinkedWriteArgs: linkedArgs,
	}, func(ctx context.Context, executable string) (string, error) {
		raw, err := (Runner{Dir: c.Repository, Timeout: min(positiveDuration(c.CommandTimeout, 10*time.Second), 10*time.Second)}).Output(ctx, Command{Executable: executable}, "--version")
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(raw)), nil
	})
	if err != nil {
		return err
	}
	evidence := contracts.PolicyResources{Version: 3, ProfileDigest: profileDigest, Workspace: workspace, WriteRoots: bound.WriteRoots, TemporaryRoots: []string{}, ImplementationData: bound.Envelope}
	evidence.Digest = policyResourceDigest(evidence)
	if e.PolicyResources != nil {
		if !reflect.DeepEqual(*e.PolicyResources, evidence) {
			return errors.New("native resources/settings expanded or changed after binding")
		}
		return nil
	}
	e.PolicyResources = &evidence
	return nil
}
