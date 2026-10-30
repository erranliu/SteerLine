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
	"github.com/erranliu/SteerLine/internal/runnerbinding"
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
	Recovery       *DeploymentRecovery            `json:"recovery,omitempty"`
	CandidateBuild *runnercontract.BuildPin       `json:"candidate_build,omitempty"`
	Takeover       *runnercontract.TakeoverReport `json:"takeover,omitempty"`
	OriginalHost   contract.Identity              `json:"original_host"`
	VerifierSHA256 string                         `json:"verifier_sha256"`
	VerifiedAt     time.Time                      `json:"verified_at"`
	Projects       []ProjectStopVerification      `json:"projects"`
	Restartable    bool                           `json:"restartable"`
	RunnerBuild    runnercontract.BuildPin        `json:"runner_build"`
	RunnerService  runnerclient.ServiceShutdown   `json:"runner_service"`
}

type applicationOfflineVerification struct {
	ApplicationStopVerification
	runnerBuild    runnercontract.BuildPin
	runnerService  runnerclient.ServiceShutdown
	candidateBuild *runnercontract.BuildPin
	takeover       *runnercontract.TakeoverReport
}

// VerifyApplicationStopped produces fresh evidence without changing the
// original failed shutdown, databases, configuration, or execution authority.
func VerifyApplicationStopped(ctx context.Context, runnerExecutable string) (ApplicationStopVerification, error) {
	result, err := verifyApplicationOffline(ctx, runnerExecutable, false)
	return result.ApplicationStopVerification, err
}

func VerifyApplicationRestartable(ctx context.Context, runnerExecutable string) (ApplicationRestartVerification, error) {
	result, err := verifyApplicationOffline(ctx, runnerExecutable, true)
	return restartVerification(result, err), err
}

func restartVerification(result applicationOfflineVerification, err error) ApplicationRestartVerification {
	return ApplicationRestartVerification{OriginalHost: result.OriginalHost, VerifierSHA256: result.VerifierSHA256,
		VerifiedAt: result.VerifiedAt, Projects: result.Projects, Restartable: err == nil,
		RunnerBuild: result.runnerBuild, RunnerService: result.runnerService, CandidateBuild: result.candidateBuild, Takeover: result.takeover}
}

func verifyApplicationOffline(ctx context.Context, runnerExecutable string, restart bool) (applicationOfflineVerification, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	release, err := localservice.Coordinate(ctx, HomeDir())
	if err != nil {
		return applicationOfflineVerification{}, err
	}
	defer release()
	return verifyApplicationOfflineCoordinated(ctx, runnerExecutable, restart)
}

func verifyApplicationOfflineCoordinated(ctx context.Context, runnerExecutable string, restart bool) (applicationOfflineVerification, error) {
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
		if !original.Runner.Stopped {
			return result, errors.New("restart requires the original stopped Runner service")
		}
		version, versionErr := cli.Version(ctx)
		if versionErr != nil {
			return result, versionErr
		}
		selection := runnerclient.Selection{Executable: runnerExecutable, Version: version.Version, SHA256: runnerHash}
		selected, _, openErr := selection.Open(ctx, pin.StateDir, 30*time.Second)
		if openErr != nil {
			return result, openErr
		}
		if runnerHash == pin.SHA256 && selected.ResourceDigest != pin.ResourceDigest {
			return result, errors.New("restart Runner resources differ from the original build")
		}
		result.runnerBuild = pin
		result.runnerService, err = selected.VerifyPreviousServiceStopped(ctx, pin.SHA256, original.Runner.InstanceID, original.Runner.RequestID)
		if err == nil && runnerHash != pin.SHA256 {
			proof, proofErr := selected.VerifyTakeover(ctx)
			if proofErr != nil {
				return result, proofErr
			}
			candidate := selected.BuildPin()
			result.candidateBuild, result.takeover = &candidate, &proof
		}
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
		proof, err := verifyProjectOffline(ctx, c, p, *b.Identity, result.Runner, restartPin, result.takeover)
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

func verifyProjectOffline(ctx context.Context, c Config, p contract.ProjectShutdown, id contract.Identity, runnerProof runnerclient.StoppedVerification, restartPin *runnercontract.BuildPin, takeover ...*runnercontract.TakeoverReport) (ProjectStopVerification, error) {
	return verifyProjectOfflineMode(ctx, c, p, id, runnerProof, restartPin, false, takeover...)
}

func verifyProjectOfflineMode(ctx context.Context, c Config, p contract.ProjectShutdown, id contract.Identity, runnerProof runnerclient.StoppedVerification, restartPin *runnercontract.BuildPin, interrupted bool, takeover ...*runnercontract.TakeoverReport) (ProjectStopVerification, error) {
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
	if interrupted {
		if snapshot.Checkpoint == nil || snapshot.Checkpoint.Service.InstanceID == "" {
			return proof, errors.New("interrupted project checkpoint identity is missing")
		}
		p.InstanceID = snapshot.Checkpoint.Service.InstanceID
		proof.InstanceID = p.InstanceID
	}
	if err = verifyCheckpointHost(snapshot.Checkpoint, c.configDigest, p, id); err != nil {
		return proof, err
	}
	if !interrupted && (snapshot.Checkpoint.Service.Status != "stopped" || snapshot.Checkpoint.Service.Ready) {
		return proof, errors.New("original stopped project checkpoint or frozen configuration differs")
	}
	generations, err := reader.Generations(ctx, c.Identity())
	if err != nil {
		return proof, err
	}
	roadmaps, err := reader.Roadmaps(ctx, c.Identity())
	if err != nil {
		return proof, err
	}
	for _, r := range roadmaps {
		generations = append(generations, roadmapBinding(r))
	}
	environments, err := reader.Environments(ctx, c.Identity())
	if err != nil {
		return proof, err
	}
	for _, r := range environments {
		for _, a := range r.Rounds {
			if unsubmittedEnvironment(a) {
				continue
			}
			if restartPin != nil {
				if !interrupted && runnerbinding.SameBuild(r.Build, *restartPin) {
					continue
				}
				var report *runnercontract.TakeoverReport
				if len(takeover) > 0 {
					report = takeover[0]
				}
				if report == nil {
					cli := runnerclient.Client{Executable: restartPin.Executable, StateDir: restartPin.StateDir, SHA256: restartPin.SHA256, BuildVersion: restartPin.Version, ResourceDigest: restartPin.ResourceDigest}
					v, e := cli.VerifyTakeover(ctx)
					if e != nil {
						return proof, e
					}
					report = &v
				}
				if _, ok := environmentTakeover(*report, r, a); !ok {
					return proof, errors.New("original environment Run/input differs from takeover evidence")
				}
			} else {
				if canonicalPath(r.Build.StateDir) != canonicalPath(runnerProof.StateDir) {
					return proof, errors.New("environment Runner namespace differs")
				}
				found := false
				for _, run := range runnerProof.Runs {
					if run.RunID == a.RunID && a.RunID != "" && a.InputDigest != "" && run.InputDigest == a.InputDigest && run.ResourcesSettled {
						found = true
					}
				}
				if !found {
					return proof, errors.New("original environment resource settlement unverified")
				}
				if r.Kind == "adaptation" && !r.Settled() {
					return proof, errors.New("environment report/PR settlement unverified; preserve original records")
				}
			}
		}
	}
	if interrupted {
		if len(takeover) != 1 || takeover[0] == nil {
			return proof, errors.New("interrupted recovery requires original-record takeover evidence")
		}
		for _, candidate := range snapshot.Candidates {
			if candidate.RunID == "" && (candidate.SubmitState == "pending" || candidate.SubmitState == "rejected" || candidate.SubmitState == "cancelled") {
				continue
			}
			if _, ok := runnerbinding.Candidate(*takeover[0], candidate); !ok {
				return proof, errors.New("interrupted candidate is absent or its original input differs")
			}
		}
	}
	if restartPin != nil {
		cli := runnerclient.Client{Executable: restartPin.Executable, StateDir: restartPin.StateDir, SHA256: restartPin.SHA256, BuildVersion: restartPin.Version, ResourceDigest: restartPin.ResourceDigest}
		if len(takeover) > 0 {
			cli.Takeover = takeover[0]
		}
		for _, g := range generations {
			if g.Unsubmitted() || g.State == "rejected" && g.RunID == "" {
				continue
			}
			if interrupted {
				if _, ok := generationTakeover(*takeover[0], g); !ok {
					return proof, errors.New("interrupted generation differs from takeover evidence")
				}
				continue
			}
			if runnerbinding.SameBuild(g.Runner, *restartPin) {
				continue
			}
			if cli.Takeover == nil {
				report, err := cli.VerifyTakeover(ctx)
				if err != nil {
					return proof, err
				}
				cli.Takeover = &report
			}
			// Supplied takeover evidence was validated against the selected
			// deployment candidate. The original home pin remains the restart
			// checkpoint identity, not the verifier of that candidate's report.
			if _, ok := generationTakeover(*cli.Takeover, g); !ok {
				return proof, errors.New("original generation Run/input differs from takeover proof")
			}
		}
	} else {
		byRun := map[string]runnerclient.StoppedRun{}
		for _, run := range runnerProof.Runs {
			byRun[run.RunID] = run
		}
		for _, g := range generations {
			if g.Unsubmitted() {
				continue
			}
			if canonicalPath(g.Runner.StateDir) != canonicalPath(runnerProof.StateDir) {
				return proof, errors.New("generation Runner namespace differs")
			}
			if g.State == "rejected" && g.RunID == "" {
				continue
			}
			run := byRun[g.RunID]
			if g.RunID == "" || g.InputDigest == "" || run.InputDigest != g.InputDigest || !run.ResourcesSettled {
				return proof, errors.New("original generation resource settlement unverified")
			}
		}
	}
	if restartPin != nil {
		if len(takeover) == 0 || takeover[0] == nil {
			needsProof := false
			for _, candidate := range snapshot.Candidates {
				if !runnerbinding.SameBuild(candidate.RunnerBuild, *restartPin) {
					needsProof = true
				}
			}
			if needsProof {
				cli, err := c.runnerClient(ctx)
				if err != nil {
					return proof, err
				}
				report, err := cli.VerifyTakeover(ctx)
				if err != nil {
					return proof, err
				}
				takeover = []*runnercontract.TakeoverReport{&report}
			}
		}
		return proof, matchRestartBindings(c, snapshot.Checkpoint, snapshot.Candidates, *restartPin, takeover...)
	}
	if canonicalConfigPath(runnerProof.StateDir) != canonicalConfigPath(runnerstatepath.Directory(c.runnerHome())) {
		return proof, errors.New("shared Runner directory changed")
	}
	if err = matchStoppedCandidates(snapshot.Candidates, runnerProof); err != nil {
		return proof, err
	}
	return proof, nil
}

func matchRestartBindings(c Config, cp *hostcontract.ServiceCheckpoint, candidates []runnerstate.Candidate, pin runnercontract.BuildPin, takeover ...*runnercontract.TakeoverReport) error {
	var proof *runnercontract.TakeoverReport
	if len(takeover) > 0 {
		proof = takeover[0]
	}
	selected := c.RunnerExecution.Runner
	if selected.SHA256 != pin.SHA256 || selected.Version != pin.Version || canonicalConfigPath(selected.Executable) != canonicalConfigPath(pin.Executable) || canonicalConfigPath(pin.StateDir) != canonicalConfigPath(runnerstatepath.Directory(c.runnerHome())) || cp == nil || cp.Service.RunnerBuild == nil || !runnerbinding.SameBuild(*cp.Service.RunnerBuild, pin) {
		return errors.New("restart project or checkpoint differs from the original Runner build")
	}
	return runnerbinding.ValidateRestartBindings(pin, proof, candidates)
}

func verifyStoppedCheckpoint(cp *hostcontract.ServiceCheckpoint, digest string, p contract.ProjectShutdown, id contract.Identity) error {
	if cp == nil || cp.Service.Status != "stopped" || cp.Service.Ready {
		return errors.New("original stopped project checkpoint or frozen configuration differs")
	}
	return verifyCheckpointHost(cp, digest, p, id)
}

func verifyCheckpointHost(cp *hostcontract.ServiceCheckpoint, digest string, p contract.ProjectShutdown, id contract.Identity) error {
	if cp == nil || cp.Service.InstanceID != p.InstanceID || cp.Service.ConfigDigest != digest || cp.Service.ProcessIdentity == nil {
		return errors.New("original project checkpoint or frozen configuration differs")
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
