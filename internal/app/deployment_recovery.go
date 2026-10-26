package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

// DeploymentRecovery records why the original installation was restarted. It
// retains the superseded receipt without turning it into native-stop evidence.
type DeploymentRecovery struct {
	InterruptedHost        contract.Identity             `json:"interrupted_host"`
	PreviousShutdown       *contract.Shutdown            `json:"previous_shutdown,omitempty"`
	PreviousRunnerShutdown *runnerclient.ServiceShutdown `json:"previous_runner_shutdown,omitempty"`
	InstalledFiles         map[string]string             `json:"installed_files"`
}

type installedDeployment struct {
	CreatedAt     string            `json:"created_at"`
	Home          string            `json:"home"`
	Source        string            `json:"steerline_source"`
	Dirty         bool              `json:"steerline_source_dirty"`
	RunnerSource  string            `json:"runner_source"`
	RunnerVersion string            `json:"runner_version"`
	Files         map[string]string `json:"files"`
}

// RecoverApplicationForDeployment is an explicitly mutating deployment action.
// Verification commands remain read-only. Coordination spans admission, the
// ORIGINAL installed processes, their normal shutdown, and the final proof.
func RecoverApplicationForDeployment(ctx context.Context, candidate string) (ApplicationRestartVerification, error) {
	return recoverApplicationForDeployment(ctx, candidate, cycleOriginalInstallation)
}

func recoverApplicationForDeployment(ctx context.Context, candidate string, cycle func(context.Context) error) (ApplicationRestartVerification, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	release, err := localservice.Coordinate(ctx, HomeDir())
	if err != nil {
		return ApplicationRestartVerification{}, err
	}
	defer release()
	return recoverApplicationForDeploymentCoordinated(ctx, candidate, cycle)
}

// Caller owns the application startup coordination for the entire operation.
func recoverApplicationForDeploymentCoordinated(ctx context.Context, candidate string, cycle func(context.Context) error) (ApplicationRestartVerification, error) {
	recovery, catalog, candidateHash, err := admitDeploymentRecovery(ctx, candidate)
	if err != nil {
		return ApplicationRestartVerification{}, err
	}
	if err = cycle(ctx); err != nil {
		return ApplicationRestartVerification{}, fmt.Errorf("original installation recovery failed: %w", err)
	}
	// Recovery must not conceal a concurrent configuration or binary replacement.
	if err = verifyRecoveryFiles(recovery.InstalledFiles); err != nil {
		return ApplicationRestartVerification{}, err
	}
	current := loadApplicationCatalog(HomeDir())
	if current.err != nil || !bytes.Equal(catalog.global, current.global) || len(catalog.entries) != len(current.entries) {
		return ApplicationRestartVerification{}, errors.New("configuration changed during deployment recovery")
	}
	for i, e := range catalog.entries {
		if e.view.ConfigID != current.entries[i].view.ConfigID || !bytes.Equal(e.data, current.entries[i].data) {
			return ApplicationRestartVerification{}, errors.New("project configuration changed during deployment recovery")
		}
	}
	result, err := verifyApplicationOfflineCoordinated(ctx, candidate, true)
	if err != nil {
		return ApplicationRestartVerification{}, err
	}
	if result.VerifierSHA256 != candidateHash || result.OriginalHost.InstanceID == recovery.InterruptedHost.InstanceID {
		return ApplicationRestartVerification{}, errors.New("recovery did not produce a new host shutdown for the selected candidate")
	}
	proof := restartVerification(result, nil)
	proof.Recovery = recovery
	return proof, nil
}

func admitDeploymentRecovery(ctx context.Context, candidate string) (*DeploymentRecovery, *applicationCatalog, string, error) {
	fail := func(err error) (*DeploymentRecovery, *applicationCatalog, string, error) { return nil, nil, "", err }
	if !filepath.IsAbs(candidate) {
		return fail(errors.New("recovery requires an absolute staged Runner executable"))
	}
	home := HomeDir()
	backend, web := components(home)
	b, err := observeComponent(ctx, home, backend)
	if err != nil || b.State != "exited" || b.Identity == nil {
		return fail(fmt.Errorf("deployment recovery requires an exactly exited original host: %s: %v", b.State, err))
	}
	if !runnerbinding.SamePath(b.Identity.Process.Executable, backend.exe) {
		return fail(errors.New("original host executable differs from installation"))
	}
	var launch launchRecord
	if err = localservice.ReadPrivateJSON(filepath.Join(backend.dir, "launch.json"), &launch, 64<<10); err != nil {
		return fail(err)
	}
	if launch.Process == nil || *launch.Process != b.Identity.Process {
		return fail(errors.New("original launch and host process identities differ"))
	}
	w, err := observeComponent(ctx, home, web)
	if err != nil || w.State != "exited" && w.State != "absent" {
		return fail(errors.New("original Web exit is unverified"))
	}
	recovery := &DeploymentRecovery{InterruptedHost: *b.Identity}
	var previous contract.Shutdown
	err = localservice.ReadPrivateJSON(filepath.Join(backend.dir, "shutdown.json"), &previous, 1<<20)
	if err == nil {
		if previous.Identity.InstanceID == b.Identity.InstanceID {
			return fail(errors.New("current host has a matching shutdown receipt; recovery cannot bypass its failed verification"))
		}
		if previous.Version != 2 || len(previous.Identity.InstanceID) != 64 || previous.Identity.HomeID != b.Identity.HomeID || previous.Identity.Protocol != b.Identity.Protocol {
			return fail(errors.New("previous shutdown belongs to an unverified home or protocol"))
		}
		recovery.PreviousShutdown = &previous
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	var record installedDeployment
	if err = localservice.ReadPrivateJSON(filepath.Join(home, "deployment.json"), &record, 64<<10); err != nil {
		return fail(fmt.Errorf("read installed deployment record %s: %w", filepath.Join(home, "deployment.json"), err))
	}
	if !runnerbinding.SamePath(record.Home, home) {
		return fail(errors.New("installed deployment belongs to another home"))
	}
	if err = verifyRecoveryFiles(record.Files); err != nil {
		return fail(err)
	}
	recovery.InstalledFiles = record.Files
	catalog := loadApplicationCatalog(home)
	if catalog.err != nil {
		return fail(catalog.err)
	}
	var global RunnerExecutionConfig
	if err = decodeStrictJSON(catalog.global, &global); err != nil {
		return fail(err)
	}
	if err = global.Validate(); err != nil {
		return fail(err)
	}
	var pin runnercontract.BuildPin
	if err = localservice.ReadPrivateJSON(filepath.Join(backend.dir, "runner-build.json"), &pin, 64<<10); err != nil {
		return fail(err)
	}
	if err = runnerstatepath.Validate(home); err != nil {
		return fail(err)
	}
	if !runnerbinding.SamePath(pin.StateDir, runnerstatepath.Directory(home)) || !runnerbinding.SamePath(pin.Executable, filepath.Join(BinDir(), "steerrunner.exe")) || pin.SHA256 != record.Files["steerrunner.exe"] || record.RunnerVersion != pin.Version {
		return fail(errors.New("original Runner pin differs from the installed release"))
	}
	original, _, err := global.Runner.Open(ctx, pin.StateDir, 30*time.Second)
	if err != nil {
		return fail(err)
	}
	if !runnerbinding.SameBuild(original.BuildPin(), pin) {
		return fail(errors.New("execution configuration differs from original Runner binding"))
	}
	info, err := original.ServiceStatus(ctx)
	if err != nil || info.Status != "not_running" {
		return fail(errors.New("original Runner writer exit is unverified"))
	}
	recovery.PreviousRunnerShutdown = info.LastShutdown
	versionClient := runnerclient.Client{Executable: candidate, StateDir: pin.StateDir, Timeout: 30 * time.Second}
	version, err := versionClient.Version(ctx)
	if err != nil {
		return fail(err)
	}
	data, err := os.ReadFile(candidate)
	if err != nil {
		return fail(err)
	}
	sha := taskPayloadDigest(data)
	selected, _, err := (runnerclient.Selection{Executable: candidate, Version: version.Version, SHA256: sha}).Open(ctx, pin.StateDir, 30*time.Second)
	if err != nil {
		return fail(err)
	}
	takeover, err := selected.VerifyTakeover(ctx)
	if err != nil {
		return fail(err)
	}
	expected := map[string]bool{}
	for _, entry := range catalog.entries {
		c, err := loadConfigSnapshot(entry.ref.ConfigPath, entry.data, catalog.global, nil, false)
		if err != nil {
			return fail(err)
		}
		if err = c.ResolveIdentity(ctx); err != nil {
			return fail(err)
		}
		key := canonicalConfigPath(c.RunDir)
		if expected[key] {
			return fail(errors.New("duplicate recovery project state directory"))
		}
		expected[key] = true
		p := contract.ProjectShutdown{ConfigID: entry.view.ConfigID, ProjectID: httpcontract.ProjectID(c.Identity())}
		if _, err = verifyProjectOfflineMode(ctx, c, p, *b.Identity, runnerclient.StoppedVerification{}, &pin, true, &takeover); err != nil {
			return fail(fmt.Errorf("%s: %w", p.ConfigID, err))
		}
	}
	// A removed project must not disappear from the set whose locks/checkpoints
	// are proved. Current project identities are backed by all managed stores.
	dirs, err := os.ReadDir(StateRoot())
	if err != nil {
		return fail(err)
	}
	for _, dir := range dirs {
		path := filepath.Join(StateRoot(), dir.Name())
		if _, err := os.Lstat(filepath.Join(path, "steerline.db")); err == nil && !expected[canonicalConfigPath(path)] {
			return fail(errors.New("project state membership changed since interruption"))
		} else if err != nil && !os.IsNotExist(err) {
			return fail(err)
		}
	}
	return recovery, catalog, sha, nil
}

func verifyRecoveryFiles(files map[string]string) error {
	for _, name := range []string{"steerline.exe", "steerline-web.exe", "steerrunner.exe"} {
		path := filepath.Join(BinDir(), name)
		for cursor := path; cursor != filepath.Dir(cursor); cursor = filepath.Dir(cursor) {
			info, err := os.Lstat(cursor)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("linked installed recovery path")
			}
		}
		if err := runnerclient.VerifyExecutable(path, files[name]); err != nil {
			return err
		}
	}
	return nil
}

func cycleOriginalInstallation(ctx context.Context) error {
	backend, web := components(HomeDir())
	cleanup := func(err error) error {
		stopCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		return errors.Join(err, stopAllCoordinated(stopCtx, HomeDir()))
	}
	if _, err := ensureComponent(ctx, HomeDir(), backend); err != nil {
		return cleanup(err)
	}
	if _, err := ensureComponent(ctx, HomeDir(), web); err != nil {
		return cleanup(err)
	}
	if err := waitDeploymentRecoveryReady(ctx); err != nil {
		return cleanup(err)
	}
	// Strict stop may report retained native resources. Only the following
	// full restart verification can authorize replacement in that case.
	_ = stopAllCoordinated(ctx, HomeDir())
	return nil
}

func waitDeploymentRecoveryReady(ctx context.Context) error {
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		status, err := StatusApplication(ctx)
		if err != nil || status.Degraded {
			return errors.Join(errors.New("original recovery projects failed readiness"), err)
		}
		ready := status.Backend.State == "ready" && status.Web.State == "ready"
		for _, p := range status.Projects {
			ready = ready && p.RuntimeState == "ready" && p.Online
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
