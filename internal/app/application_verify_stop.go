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
	Recovery       *DeploymentRecovery          `json:"recovery,omitempty"`
	CandidateBuild *runnercontract.BuildPin     `json:"candidate_build,omitempty"`
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
	runnerBuild    runnercontract.BuildPin
	runnerService  runnerclient.ServiceShutdown
	candidateBuild *runnercontract.BuildPin
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
		RunnerBuild: result.runnerBuild, RunnerService: result.runnerService, CandidateBuild: result.candidateBuild}
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
	if !original.Runner.Attempted {
		if !original.Clean || len(original.Projects) != 0 || *original.Runner != (contract.RunnerShutdown{ResourcesSettled: true}) {
			return result, errors.New("unstarted Runner shutdown scope is unverified")
		}
		for _, path := range []string{filepath.Join(backend.dir, "runner-build.json"), runnerstatepath.Directory(HomeDir())} {
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				return result, errors.New("unstarted Runner has an existing or unreadable binding/state")
			}
		}
		result.Runner = runnerclient.StoppedVerification{ResourcesSettled: true, Runs: []runnerclient.StoppedRun{}}
		result.VerifiedAt = time.Now().UTC()
		return result, nil
	}
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
			originalCLI := runnerclient.Client{Executable: pin.Executable, SHA256: pin.SHA256, StateDir: pin.StateDir}
			if err = originalCLI.VerifyUpgradeDrained(ctx, selected); err != nil {
				return result, err
			}
			candidate := selected.BuildPin()
			result.candidateBuild = &candidate
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
		proof, err := verifyProjectOffline(ctx, c, p, *b.Identity, result.Runner, restartPin)
		if err != nil {
			return result, fmt.Errorf("%s: %w", p.ConfigID, err)
		}
		result.Projects = append(result.Projects, proof)
	}
	if !sameApplicationCatalog(d, loadApplicationCatalog(HomeDir())) {
		return result, errors.New("project or execution configuration changed during stop verification")
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
	return verifyProjectOfflineMode(ctx, c, p, id, runnerProof, restartPin, false)
}

// Discard preparation already holds the repository lock for the complete swap.
func verifyDiscardProject(ctx context.Context, c Config, p contract.ProjectShutdown, id contract.Identity, proof runnerclient.StoppedVerification, interrupted bool) (ProjectStopVerification, error) {
	return verifyProjectOfflineMode(ctx, c, p, id, proof, nil, interrupted, true)
}

func verifyProjectOfflineMode(ctx context.Context, c Config, p contract.ProjectShutdown, id contract.Identity, runnerProof runnerclient.StoppedVerification, restartPin *runnercontract.BuildPin, interrupted bool, discardMode ...bool) (ProjectStopVerification, error) {
	discard := len(discardMode) == 1 && discardMode[0]
	proof := ProjectStopVerification{ConfigID: p.ConfigID, ProjectID: p.ProjectID, InstanceID: p.InstanceID}
	open := OpenNativeTaskReader
	if discard {
		open = openDiscardTaskReader
	}
	reader, err := open(ctx, &c)
	if err != nil {
		return proof, err
	}
	defer reader.Close()
	if httpcontract.ProjectID(c.Identity()) != p.ProjectID {
		return proof, errors.New("original project configuration identity differs")
	}
	if !discard {
		repoLock, err := host.LockRepository(c.Identity())
		if err != nil {
			return proof, err
		}
		defer repoLock.Close()
	}
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
		if p.InstanceID == "" {
			p.InstanceID = snapshot.Checkpoint.Service.InstanceID
		}
		proof.InstanceID = p.InstanceID
	}
	if discard {
		if err = verifyDiscardCheckpoint(snapshot.Checkpoint, c, p, id, interrupted); err != nil {
			return proof, err
		}
	} else if interrupted {
		if err = verifyCheckpointHost(snapshot.Checkpoint, p, id); err != nil {
			return proof, err
		}
		if snapshot.Checkpoint.Service.ConfigDigest != c.configDigest {
			return proof, errors.New("interrupted project frozen configuration differs")
		}
	} else if err = verifyStoppedCheckpoint(snapshot.Checkpoint, c, p, id); err != nil {
		return proof, err
	}
	generations, err := reader.Generations(ctx, c.Identity())
	if err != nil {
		return proof, err
	}
	roadmaps, err := reader.Roadmaps(ctx, c.Identity())
	if err != nil {
		return proof, err
	}
	contents := make([]runnerbinding.Content, 0, len(generations)+len(roadmaps))
	for _, g := range generations {
		contents = append(contents, generationContent(g))
	}
	for _, r := range roadmaps {
		contents = append(contents, roadmapContent(r))
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
				cli := runnerclient.Client{Executable: restartPin.Executable, SHA256: restartPin.SHA256, StateDir: restartPin.StateDir}
				view, _, err := inspectOriginalBeforeStart(ctx, cli, r.Build.StateDir, a.RunID, a.InputDigest, a.Request)
				if err != nil {
					return proof, err
				}
				if view.Run.Correlation["environment_activity"] != r.ID {
					return proof, errors.New("original environment correlation differs")
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
				if r.Kind == "adaptation" && !r.Settled() && !discard {
					return proof, errors.New("environment report/PR settlement unverified; preserve original records")
				}
			}
		}
	}
	if restartPin != nil {
		cli := runnerclient.Client{Executable: restartPin.Executable, StateDir: restartPin.StateDir, SHA256: restartPin.SHA256}
		for _, candidate := range snapshot.Candidates {
			if candidate.RunID == "" && (candidate.SubmitState == "pending" || candidate.SubmitState == "rejected" || candidate.SubmitState == "cancelled") {
				continue
			}
			if _, _, err := inspectOriginalBeforeStart(ctx, cli, candidate.StateDir, candidate.RunID, candidate.InputDigest, candidate.Submit); err != nil {
				return proof, err
			}
		}
		for _, g := range contents {
			if g.Unsubmitted || g.Rejected {
				continue
			}
			if _, _, err := inspectOriginalBeforeStart(ctx, cli, g.StateDir, g.RunID, g.InputDigest, g.Request); err != nil {
				return proof, err
			}
		}
	} else {
		byRun := map[string]runnerclient.StoppedRun{}
		for _, run := range runnerProof.Runs {
			byRun[run.RunID] = run
		}
		for _, g := range contents {
			if g.Unsubmitted {
				continue
			}
			if canonicalPath(g.StateDir) != canonicalPath(runnerProof.StateDir) {
				return proof, errors.New("generation Runner namespace differs")
			}
			if g.Rejected {
				continue
			}
			run := byRun[g.RunID]
			if g.RunID == "" || g.InputDigest == "" || run.InputDigest != g.InputDigest || !run.ResourcesSettled {
				return proof, errors.New("original generation resource settlement unverified")
			}
		}
	}
	if restartPin != nil {
		return proof, matchRestartBindings(c, snapshot.Checkpoint, snapshot.Candidates, *restartPin)
	}

	if canonicalConfigPath(runnerProof.StateDir) != canonicalConfigPath(runnerstatepath.Directory(c.runnerHome())) {
		return proof, errors.New("shared Runner directory changed")
	}
	if err = matchStoppedCandidates(snapshot.Candidates, runnerProof); err != nil {
		return proof, err
	}
	return proof, nil
}

func matchRestartBindings(c Config, cp *hostcontract.ServiceCheckpoint, candidates []runnerstate.Candidate, pin runnercontract.BuildPin) error {
	selected := c.RunnerExecution.Runner
	if selected.SHA256 != pin.SHA256 || selected.Version != pin.Version || canonicalConfigPath(selected.Executable) != canonicalConfigPath(pin.Executable) || canonicalConfigPath(pin.StateDir) != canonicalConfigPath(runnerstatepath.Directory(c.runnerHome())) || cp == nil || cp.Service.RunnerBuild == nil || !runnerbinding.SameBuild(*cp.Service.RunnerBuild, pin) {
		return errors.New("restart project or checkpoint differs from the original Runner build")
	}
	return runnerbinding.ValidateRestartBindings(pin, candidates)
}

func verifyStoppedCheckpoint(cp *hostcontract.ServiceCheckpoint, c Config, p contract.ProjectShutdown, id contract.Identity) error {
	if cp == nil || cp.Service.Status != "stopped" || cp.Service.Ready {
		return errors.New("original project stopped checkpoint is unverified")
	}
	if err := verifyCheckpointHost(cp, p, id); err != nil {
		return err
	}
	return verifyCheckpointBindings(cp, c)
}

// An exited interrupted host cannot publish a receipt. Discard proves local
// closure through exact host exit and actual locks, retaining checkpoint
// ownership and requiring independent native settlement before the swap.
func verifyDiscardCheckpoint(cp *hostcontract.ServiceCheckpoint, c Config, p contract.ProjectShutdown, id contract.Identity, interrupted bool) error {
	if !interrupted {
		return verifyStoppedCheckpoint(cp, c, p, id)
	}
	if p.InstanceID == "" {
		return errors.New("interrupted project checkpoint identity is missing")
	}
	if err := verifyCheckpointHost(cp, p, id); err != nil {
		return err
	}
	return verifyCheckpointBindings(cp, c)
}

func verifyCheckpointBindings(cp *hostcontract.ServiceCheckpoint, c Config) error {
	// Saved configuration belongs to the next host. The closed host is proved
	// by its checkpoint/receipt identity, not by equality with editable files.
	// Repository and Runner ownership must still describe the original runtime.
	if canonicalPath(cp.Repository.Path) != canonicalPath(c.Repository) || canonicalPath(cp.Repository.Identity) != canonicalPath(c.Identity()) || cp.Repository.Remote != c.Remote || cp.Repository.Branch != c.Branch {
		return errors.New("original project repository binding differs")
	}
	pin, selected := cp.Service.RunnerBuild, c.RunnerExecution.Runner
	if pin == nil || !runnerbinding.SamePath(pin.Executable, selected.Executable) || pin.SHA256 != selected.SHA256 || pin.Version != selected.Version || !runnerbinding.SamePath(pin.StateDir, runnerstatepath.Directory(c.runnerHome())) {
		return errors.New("saved configuration differs from the original Runner binding")
	}
	return nil
}

func verifyCheckpointHost(cp *hostcontract.ServiceCheckpoint, p contract.ProjectShutdown, id contract.Identity) error {
	if cp == nil || cp.Service.InstanceID != p.InstanceID || cp.Service.ProcessIdentity == nil {
		return errors.New("original project checkpoint identity differs")
	}
	if digest, err := hex.DecodeString(cp.Service.ConfigDigest); err != nil || len(digest) != sha256.Size {
		return errors.New("original project frozen configuration digest is missing or invalid")
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
