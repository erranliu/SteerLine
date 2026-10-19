package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

type ApplicationStopVerification struct {
	OriginalHost   contract.Identity                `json:"original_host"`
	VerifierSHA256 string                           `json:"verifier_sha256"`
	VerifiedAt     time.Time                        `json:"verified_at"`
	Projects       []ProjectStopVerification        `json:"projects"`
	Runner         runnerclient.StoppedVerification `json:"runner"`
}
type ProjectStopVerification struct {
	ConfigID   string `json:"config_id"`
	ProjectID  string `json:"project_id"`
	InstanceID string `json:"instance_id"`
}

// A restart proof deliberately makes no native-resource-settlement claim.
type ApplicationRestartVerification struct {
	OriginalHost   contract.Identity            `json:"original_host"`
	VerifierSHA256 string                       `json:"verifier_sha256"`
	VerifiedAt     time.Time                    `json:"verified_at"`
	Projects       []ProjectStopVerification    `json:"projects"`
	Restartable    bool                         `json:"restartable"`
	RunnerBuild    runnercontract.BuildPin      `json:"runner_build"`
	RunnerService  runnerclient.ServiceShutdown `json:"runner_service"`
}

type applicationOfflineVerification struct {
	ApplicationStopVerification
	runnerBuild   runnercontract.BuildPin
	runnerService runnerclient.ServiceShutdown
}

// VerifyApplicationStopped produces fresh evidence without changing the
// original failed shutdown, databases, configuration, or execution authority.
func VerifyApplicationStopped(ctx context.Context, runnerExecutable string) (ApplicationStopVerification, error) {
	result, err := verifyApplicationOffline(ctx, runnerExecutable, false)
	return result.ApplicationStopVerification, err
}

func VerifyApplicationRestartable(ctx context.Context, runnerExecutable string) (ApplicationRestartVerification, error) {
	result, err := verifyApplicationOffline(ctx, runnerExecutable, true)
	return ApplicationRestartVerification{OriginalHost: result.OriginalHost, VerifierSHA256: result.VerifierSHA256,
		VerifiedAt: result.VerifiedAt, Projects: result.Projects, Restartable: err == nil,
		RunnerBuild: result.runnerBuild, RunnerService: result.runnerService}, err
}

func verifyApplicationOffline(ctx context.Context, runnerExecutable string, restart bool) (applicationOfflineVerification, error) {
	result := applicationOfflineVerification{ApplicationStopVerification: ApplicationStopVerification{Projects: []ProjectStopVerification{}}}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if !filepath.IsAbs(runnerExecutable) {
		return result, errors.New("verification requires an absolute staged Runner executable")
	}
	data, err := os.ReadFile(runnerExecutable)
	if err != nil {
		return result, err
	}
	hash := sha256.Sum256(data)
	runnerHash := hex.EncodeToString(hash[:])
	result.VerifierSHA256 = runnerHash
	release, err := localservice.Coordinate(ctx, HomeDir())
	if err != nil {
		return result, err
	}
	defer release()
	backend, web := components(HomeDir())
	b, err := observeComponent(ctx, HomeDir(), backend)
	if err != nil || b.State != "exited" || b.Identity == nil {
		return result, fmt.Errorf("original application host exit is unverified: state=%s: %v", b.State, err)
	}
	w, err := observeComponent(ctx, HomeDir(), web)
	if err != nil || w.State != "absent" && w.State != "exited" {
		return result, fmt.Errorf("original Web exit is unverified: state=%s: %v", w.State, err)
	}
	var original contract.Shutdown
	if err = localservice.ReadPrivateJSON(filepath.Join(backend.dir, "shutdown.json"), &original, 1<<20); err != nil {
		return result, err
	}
	if err = recheckableShutdown(original, *b.Identity); err != nil {
		return result, err
	}
	d := loadApplicationCatalog(HomeDir())
	if d.err != nil {
		return result, d.err
	}
	if len(d.entries) != len(original.Projects) {
		return result, errors.New("project configuration membership changed since original shutdown")
	}
	byConfig := map[string]contract.ProjectShutdown{}
	for _, p := range original.Projects {
		byConfig[p.ConfigID] = p
	}
	result.OriginalHost = *b.Identity
	if err := runnerstatepath.Validate(HomeDir()); err != nil {
		return result, err
	}
	if original.Runner.StateDir != runnerstatepath.Directory(HomeDir()) {
		return result, errors.New("original shared Runner directory differs")
	}
	var pin runnercontract.BuildPin
	if err = localservice.ReadPrivateJSON(filepath.Join(backend.dir, "runner-build.json"), &pin, 64<<10); err != nil {
		return result, err
	}
	if pin.StateDir != original.Runner.StateDir || pin.SHA256 != original.Runner.ExecutableSHA256 || pin.Version != original.Runner.Version || pin.ResourceDigest != original.Runner.ResourceDigest {
		return result, errors.New("original shared Runner build differs from its immutable home binding")
	}
	cli := runnerclient.Client{Executable: runnerExecutable, SHA256: runnerHash, StateDir: original.Runner.StateDir, Timeout: 30 * time.Second}
	if restart {
		if !original.Runner.Stopped || runnerHash != pin.SHA256 {
			return result, errors.New("restart requires the original stopped Runner service and identical executable; changing Runner requires resource settlement")
		}
		selection := runnerclient.Selection{Executable: runnerExecutable, Version: pin.Version, SHA256: pin.SHA256}
		selected, _, openErr := selection.Open(ctx, pin.StateDir, 30*time.Second)
		if openErr != nil {
			return result, openErr
		}
		if selected.ResourceDigest != pin.ResourceDigest {
			return result, errors.New("restart Runner resources differ from the original build")
		}
		result.runnerBuild = pin
		result.runnerService, err = selected.VerifyServiceStopped(ctx, original.Runner.InstanceID, original.Runner.RequestID)
	} else {
		result.Runner, err = cli.VerifyStopped(ctx)
	}
	if err != nil {
		return result, err
	}
	for _, entry := range d.entries {
		p, ok := byConfig[entry.view.ConfigID]
		if !ok {
			return result, errors.New("project configuration membership changed since original shutdown")
		}
		c, err := loadConfigSnapshot(entry.ref.ConfigPath, entry.data, d.global, nil, false)
		if err != nil {
			return result, err
		}
		var restartPin *runnercontract.BuildPin
		if restart {
			restartPin = &pin
		}
		proof, err := verifyProjectOffline(ctx, c, p, *b.Identity, result.Runner, restartPin)
		if err != nil {
			return result, fmt.Errorf("%s: %w", p.ConfigID, err)
		}
		result.Projects = append(result.Projects, proof)
	}
	result.VerifiedAt = time.Now().UTC()
	return result, nil
}

func recheckableShutdown(r contract.Shutdown, id contract.Identity) error {
	if r.Version != 2 || r.Identity != id || r.Runner == nil || r.Projects == nil || len(r.Projects) > 512 {
		return errors.New("original host shutdown identity or membership is unverified")
	}
	seen := map[string]bool{}
	for _, p := range r.Projects {
		if p.ConfigID == "" || seen[p.ConfigID] || !p.LocalClosed || !p.LocksReleased || p.ProjectID == "" || p.InstanceID == "" {
			return errors.New("original project local closure, locks or identity is unverified")
		}
		seen[p.ConfigID] = true
	}
	return nil
}

func verifyProjectOffline(ctx context.Context, c Config, p contract.ProjectShutdown, id contract.Identity, runnerProof runnerclient.StoppedVerification, restartPin *runnercontract.BuildPin) (ProjectStopVerification, error) {
	proof := ProjectStopVerification{ConfigID: p.ConfigID, ProjectID: p.ProjectID, InstanceID: p.InstanceID}
	reader, err := OpenNativeTaskReader(ctx, &c)
	if err != nil {
		return proof, err
	}
	defer reader.Close()
	if httpcontract.ProjectID(c.Identity()) != p.ProjectID {
		return proof, errors.New("original project configuration identity differs")
	}
	repoLock, err := host.LockRepository(c.Identity())
	if err != nil {
		return proof, err
	}
	defer repoLock.Close()
	runLock, err := host.LockRepository(c.RunDir)
	if err != nil {
		return proof, err
	}
	defer runLock.Close()
	snapshot, err := reader.ReadRunnerSnapshot(ctx, c.Identity())
	if err != nil {
		return proof, err
	}
	if err = verifyStoppedCheckpoint(snapshot.Checkpoint, c.configDigest, p, id); err != nil {
		return proof, err
	}
	checks, err := reader.Checks(ctx, c.Identity())
	if err != nil {
		return proof, err
	}
	if restartPin != nil {
		return proof, matchRestartBindings(c, snapshot.Checkpoint, snapshot.Candidates, checks, *restartPin)
	}
	if canonicalConfigPath(runnerProof.StateDir) != canonicalConfigPath(runnerstatepath.Directory(c.runnerHome())) {
		return proof, errors.New("shared Runner directory changed")
	}
	if err = matchStoppedCandidates(snapshot.Candidates, runnerProof); err != nil {
		return proof, err
	}
	if err = matchStoppedChecks(checks, runnerProof); err != nil {
		return proof, err
	}
	return proof, nil
}

func matchRestartBindings(c Config, cp *hostcontract.ServiceCheckpoint, candidates []runnerstate.Candidate, checks []runnerstate.EnvironmentCheckRecord, pin runnercontract.BuildPin) error {
	selected := c.RunnerExecution.Runner
	if selected.SHA256 != pin.SHA256 || selected.Version != pin.Version || canonicalConfigPath(selected.Executable) != canonicalConfigPath(pin.Executable) || canonicalConfigPath(pin.StateDir) != canonicalConfigPath(runnerstatepath.Directory(c.runnerHome())) || cp == nil || cp.Service.RunnerBuild == nil || !sameRunnerBuild(*cp.Service.RunnerBuild, pin) {
		return errors.New("restart project or checkpoint differs from the original Runner build")
	}
	for _, candidate := range candidates {
		if !sameRunnerBuild(candidate.RunnerBuild, pin) || canonicalConfigPath(candidate.StateDir) != canonicalConfigPath(pin.StateDir) || candidate.RunID != "" && candidate.InputDigest == "" {
			return errors.New("restart candidate differs from the original Runner build or directory")
		}
	}
	for _, check := range checks {
		if check.Selection.SHA256 != pin.SHA256 || check.Selection.Version != pin.Version || canonicalConfigPath(check.Selection.Executable) != canonicalConfigPath(pin.Executable) || check.ResourceDigest != pin.ResourceDigest || canonicalConfigPath(check.StateDir) != canonicalConfigPath(pin.StateDir) {
			return errors.New("restart environment check differs from the original Runner build or directory")
		}
	}
	return nil
}

func matchStoppedChecks(checks []runnerstate.EnvironmentCheckRecord, runnerProof runnerclient.StoppedVerification) error {
	byCheck := map[string]runnerclient.StoppedCheck{}
	for _, v := range runnerProof.Checks {
		byCheck[v.CheckID] = v
	}
	for _, check := range checks {
		if canonicalConfigPath(check.StateDir) != canonicalConfigPath(runnerProof.StateDir) {
			return errors.New("check belongs to another Runner directory")
		}
		if check.CheckID == "" {
			if len(check.Operations) == 0 || check.Operations[0].Status != "rejected" {
				return errors.New("original check submission remains unverified")
			}
			continue
		}
		v, ok := byCheck[check.CheckID]
		if !ok || check.Report == nil || check.Report.InputDigest == "" || v.InputDigest != check.Report.InputDigest {
			return errors.New("original check resource proof is absent or differs")
		}
	}

	return nil
}

func verifyStoppedCheckpoint(cp *hostcontract.ServiceCheckpoint, digest string, p contract.ProjectShutdown, id contract.Identity) error {
	if cp == nil || cp.Service.InstanceID != p.InstanceID || cp.Service.ConfigDigest != digest || cp.Service.Status != "stopped" || cp.Service.Ready || cp.Service.ProcessIdentity == nil {
		return errors.New("original stopped project checkpoint or frozen configuration differs")
	}
	process := cp.Service.ProcessIdentity
	if process.Kind != id.Process.Kind || process.PID != id.Process.PID || process.Created != id.Process.Created || process.Executable != id.Process.Executable {
		return errors.New("original project checkpoint belongs to another host process")
	}
	return nil
}

func matchStoppedCandidates(candidates []runnerstate.Candidate, proof runnerclient.StoppedVerification) error {
	runs := map[string]runnerclient.StoppedRun{}
	for _, r := range proof.Runs {
		runs[r.RunID] = r
	}
	for _, c := range candidates {
		if canonicalConfigPath(c.StateDir) != canonicalConfigPath(proof.StateDir) {
			return errors.New("candidate belongs to another Runner state directory")
		}
		if c.RunID == "" {
			if c.SubmitState != "rejected" && c.SubmitState != "cancelled" {
				return errors.New("original candidate submission remains unverified")
			}
			continue
		}
		r, ok := runs[c.RunID]
		if !ok || c.InputDigest == "" || c.InputDigest != r.InputDigest {
			return errors.New("original candidate Run is absent or differs from verified Runner evidence")
		}
	}
	return nil
}
