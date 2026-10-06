package execution

import (
	"slices"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func (c Config) ValidateStoredExecutions(snapshot reconstruction.Snapshot, directory *ExecutionDirectory) error {
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.WorkflowVersion != executionmodel.Version || workflow.ExecutionProfile == nil || !VerifyFixedExecutionProfile(*workflow.ExecutionProfile, directory) || workflow.ConfigDigest != workflow.ExecutionProfile.AcceptedConfigVersion {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses an old WorkflowRun; preserve the original state and external session")
		}
	}
	for _, execution := range snapshot.Executions {
		boundSnapshot := false
		var fixedProfile *executionmodel.FixedExecutionProfile
		for _, workflow := range snapshot.WorkflowRuns {
			if workflow.WorkflowRunID == execution.WorkflowRunID && workflow.ProjectIdentity == execution.ProjectIdentity && workflow.ExecutionProfile != nil && execution.ConfigurationDigest == workflow.ExecutionProfile.Digest {
				boundSnapshot = true
				fixedProfile = workflow.ExecutionProfile
				break
			}
		}
		if !boundSnapshot {
			return nativeErr("unsupported_schema", "execution is not bound to its original configuration snapshot")
		}
		if err := validateExecutionImplementationData(&execution); err != nil {
			return nativeErr("unsupported_schema", "execution implementation binding is invalid: "+err.Error())
		}
		fixedConfig := c.ApplyFixedExecutionProfile(*fixedProfile, directory)
		entry, resolveErr := registrationFor(fixedConfig, directory)
		if resolveErr != nil || execution.Backend != string(entry.Key.Backend) || execution.Agent != entry.Key.Agent || execution.Protocol != string(entry.Key.Protocol) {
			return nativeErr("unsupported_schema", "fixed execution profile differs from the persisted run identity")
		}
		if (execution.Process != nil || execution.SessionID != "") && !awaitingRecoveryResources(&execution) {
			if execution.PolicyResources == nil {
				return nativeErr("unsupported_schema", "bound execution requires its original policy resource snapshot; preserve external state")
			}
			matched := false
			for _, workflow := range snapshot.WorkflowRuns {
				if workflow.WorkflowRunID == execution.WorkflowRunID && workflow.ExecutionProfile != nil && verifyPolicyResources(fixedConfig, directory, execution.PolicyResources, workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, &execution) == nil {
					matched = true
				}
			}
			if !matched {
				return nativeErr("unsupported_schema", "execution policy resources differ from the original snapshot")
			}
		}
		if entry.ReadinessRequired(fixedConfig) || execution.Environment != nil {
			env := execution.Environment
			if env == nil || env.Preparation == nil || !executionbackend.StableExecutionID(env.Preparation.TurnID) || !executionbackend.StableExecutionID(env.Preparation.Nonce) || env.Preparation.StartedAt.IsZero() || !env.Preparation.Deadline.After(env.Preparation.StartedAt) {
				return nativeErr("unsupported_schema", "execution readiness lacks its frozen preparation intent")
			}
			matched := false
			for _, workflow := range snapshot.WorkflowRuns {
				if workflow.WorkflowRunID == execution.WorkflowRunID && workflow.ExecutionProfile != nil && VerifyFixedExecutionProfile(*workflow.ExecutionProfile, directory) && workflow.ExecutionProfile.Digest == env.ProfileDigest {
					matched = true
				}
			}
			if !matched {
				return nativeErr("unsupported_schema", "execution readiness differs from its frozen profile")
			}
			p := env.Preparation
			if p.Verified && (p.Receipt == nil || !p.Evidence.Verified || p.Receipt.ProbeIdentity != p.Probe || p.Receipt.InputSHA256 != p.InputDigest) {
				return nativeErr("unsupported_schema", "execution readiness lacks verified receipt/tool facts")
			}
			if env.Ready && (!p.Verified || p.Invalidated || execution.Process == nil || execution.SessionID == "" || execution.SessionGeneration == "") {
				return nativeErr("unsupported_schema", "ready execution has no verified original session")
			}
			// The original policy resources were verified above. Full access has
			// no bounded roots; workspace access must retain its exact root list.
			if execution.Process != nil && (env.Runtime != execution.Process.RuntimeID || env.LaunchCommand == "" || execution.PolicyResources == nil || !slices.Equal(env.WriteRoots, execution.PolicyResources.WriteRoots)) {
				return nativeErr("unsupported_schema", "execution resource evidence is incomplete or changed")
			}
		}
	}
	return nil
}
