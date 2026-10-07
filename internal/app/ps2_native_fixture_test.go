//go:build ps2native

package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"errors"

	"path/filepath"
	"reflect"

	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func nativePolicyID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func nativePolicyCanonical(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(filepath.Clean(absolute))
	}
	return filepath.Clean(absolute)
}

func nativePolicyTrimBOM(raw []byte) []byte { return bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}) }

func nativePolicyTarget(c execution.Config, directory *execution.ExecutionDirectory) executionbackend.ExecutionTarget {
	entry, err := directory.Resolve(c)
	if err != nil {
		panic(err)
	}
	return entry.Target(c)
}

// These helpers construct only an acceptance report DTO. They receive neither
// a Service nor a Store, and invoke the actual catalog's resource codec.
func nativePolicyResourceDigest(resources executionmodel.PolicyResources) string {
	resources.Digest = ""
	sum := sha256.Sum256(mustJSON(resources))
	return hex.EncodeToString(sum[:])
}

func bindNativePolicyProbeResources(ctx context.Context, c execution.Config, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost, e *executionmodel.ExecutionRecord, path string, linked []string) error {
	entry, err := directory.Resolve(c)
	if err != nil {
		return err
	}
	if len(c.ImplementationData) == 0 {
		return errors.New("accepted native profile data missing")
	}
	profile := c.ImplementationData[0]
	id := execution.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}
	value, err := entry.BuildPolicyResource(ctx, execution.PolicyResourceBuildRequest{ProcessHost: processHost, ProfileData: c.ImplementationData, ProjectID: c.Identity(), ProfileID: profile.ProfileID, AcceptedConfigVersion: profile.AcceptedConfigVersion, Resource: id, Host: c.Host, Agent: c.Agent, Workspace: path, LinkedWorktreeArgs: linked, RepositoryPath: c.Repository, CommandTimeout: 90 * time.Second})
	if err != nil {
		return err
	}
	if err := entry.VerifyPolicyResource(value.Envelope, id); err != nil {
		return err
	}
	resources := executionmodel.PolicyResources{Version: 3, ProfileDigest: e.ConfigurationDigest, Workspace: path, WriteRoots: append([]string(nil), value.WriteRoots...), TemporaryRoots: []string{}, ImplementationData: value.Envelope.Clone()}
	resources.Digest = nativePolicyResourceDigest(resources)
	if e.PolicyResources != nil && !reflect.DeepEqual(*e.PolicyResources, resources) {
		return errors.New("original native acceptance resources changed")
	}
	e.PolicyResources = &resources
	return nil
}

func verifyNativePolicyProbeResources(c execution.Config, directory *execution.ExecutionDirectory, e *executionmodel.ExecutionRecord, digest, path string) error {
	r := e.PolicyResources
	if r == nil || r.Version != 3 || r.Digest != nativePolicyResourceDigest(*r) || r.ProfileDigest != digest || r.Workspace != path || len(r.TemporaryRoots) != 0 || r.ImplementationData.ConfigDigest != digest {
		return errors.New("original acceptance resource snapshot differs")
	}
	entry, err := directory.Resolve(c)
	if err != nil {
		return err
	}
	return entry.VerifyPolicyResource(r.ImplementationData, execution.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest})
}

func nativePolicyBackend(t *testing.T, c Config, run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord, profile executionmodel.FixedExecutionProfile) codexworker.Backend {
	t.Helper()
	executionConfig := c.executionConfig()
	entry, err := c.ExecutionDirectory.Resolve(executionConfig)
	require(t, err)
	snapshot := executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol, Target: e.Target, Agent: executionbackend.AgentSpec{Name: c.Agent}, RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: *run.WorktreePath}, Session: executionbackend.SessionContext{ProjectID: c.Identity(), ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, WorkflowRunID: run.RunID, Agent: executionbackend.AgentSpec{Name: c.Agent}}, ResourceGeneration: e.Generation, ConfigurationDigest: e.ConfigurationDigest}
	port, err := entry.Lifecycle(executionbackend.LifecycleSetup{Snapshot: snapshot, ProfileData: executionConfig.ImplementationData, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion, ProcessHost: c.ProcessHost, RunDirectory: executionConfig.RunDir, RepositoryPath: executionConfig.Repository, CommandTimeout: 90 * time.Second, BlockedEnvironment: []string{"GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN"}})
	require(t, err)
	actual, ok := port.(*codexworker.ExecutionLifecycle)
	if !ok {
		t.Fatal("native app-server lifecycle type differs")
	}
	return actual.Backend
}
