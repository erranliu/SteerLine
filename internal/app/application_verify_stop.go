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
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

type ApplicationStopVerification struct {
	OriginalHost   contract.Identity         `json:"original_host"`
	VerifierSHA256 string                    `json:"verifier_sha256"`
	VerifiedAt     time.Time                 `json:"verified_at"`
	Projects       []ProjectStopVerification `json:"projects"`
}
type ProjectStopVerification struct {
	ConfigID   string                           `json:"config_id"`
	ProjectID  string                           `json:"project_id"`
	InstanceID string                           `json:"instance_id"`
	Runner     runnerclient.StoppedVerification `json:"runner"`
}

// VerifyApplicationStopped produces fresh evidence without changing the
// original failed shutdown, databases, configuration, or execution authority.
func VerifyApplicationStopped(ctx context.Context, runnerExecutable string) (ApplicationStopVerification, error) {
	result := ApplicationStopVerification{Projects: []ProjectStopVerification{}}
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
	for _, entry := range d.entries {
		p, ok := byConfig[entry.view.ConfigID]
		if !ok {
			return result, errors.New("project configuration membership changed since original shutdown")
		}
		c, err := loadConfigSnapshot(entry.ref.ConfigPath, entry.data, d.global, nil, false)
		if err != nil {
			return result, err
		}
		proof, err := verifyProjectStopped(ctx, c, p, *b.Identity, runnerExecutable, runnerHash)
		if err != nil {
			return result, fmt.Errorf("%s: %w", p.ConfigID, err)
		}
		result.Projects = append(result.Projects, proof)
	}
	result.VerifiedAt = time.Now().UTC()
	return result, nil
}

func recheckableShutdown(r contract.Shutdown, id contract.Identity) error {
	if r.Version != 1 || r.Identity != id || r.Projects == nil || len(r.Projects) > 512 {
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

func verifyProjectStopped(ctx context.Context, c Config, p contract.ProjectShutdown, id contract.Identity, exe, hash string) (ProjectStopVerification, error) {
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
	if err := runnerstatepath.Validate(c.RunDir); err != nil {
		return proof, err
	}
	cli := runnerclient.Client{Executable: exe, SHA256: hash, StateDir: runnerstatepath.Directory(c.RunDir), Timeout: 30 * time.Second}
	proof.Runner, err = cli.VerifyStopped(ctx)
	if err != nil {
		return proof, err
	}
	if err = matchStoppedCandidates(snapshot.Candidates, proof.Runner); err != nil {
		return proof, err
	}
	return proof, nil
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
			continue
		}
		r, ok := runs[c.RunID]
		if !ok || c.InputDigest != "" && c.InputDigest != r.InputDigest {
			return errors.New("original candidate Run is absent or differs from verified Runner evidence")
		}
	}
	return nil
}
