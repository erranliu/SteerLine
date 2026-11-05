package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

type RunnerUpgradeVerification struct {
	Contract        string `json:"contract"`
	Home            string `json:"home"`
	StateDir        string `json:"state_dir"`
	CandidateSHA256 string `json:"candidate_sha256"`
	StateFormat     string `json:"state_format"`
}

// VerifyRunnerUpgrade is a preflight, not a writer lease. It neither stops Runs
// nor migrates namespaces. An incompatible installation remains fully intact.
func VerifyRunnerUpgrade(ctx context.Context, home, executable, sha string) (RunnerUpgradeVerification, error) {
	result := RunnerUpgradeVerification{Contract: "steerline-runner-upgrade/v1", Home: home, StateDir: runnerstatepath.Directory(home), CandidateSHA256: sha, StateFormat: runnerclient.StateFormat}
	if err := checkDeploymentDiscard(home); err != nil {
		return result, err
	}
	if err := runnerstatepath.Validate(home); err != nil {
		return result, err
	}
	var pin runnercontract.BuildPin
	err := localservice.ReadPrivateJSON(filepath.Join(home, "application-api", "runner-build.json"), &pin, 64<<10)
	if err == nil && !runnerbinding.SamePath(pin.StateDir, result.StateDir) {
		return result, errors.New("Runner state format changed to v8; drain the original installation with its original executable, then deploy to a fresh InstallHome; no migration or automatic cleanup is supported")
	}
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	cli := runnerclient.Client{Executable: executable, SHA256: sha, StateDir: result.StateDir}
	if e := verifyRunnerUpgradeCapabilities(ctx, cli); e != nil {
		return result, e
	}
	if err == nil && pin.SHA256 != sha {
		original := runnerclient.Client{Executable: pin.Executable, SHA256: pin.SHA256, StateDir: pin.StateDir}
		if err := original.VerifyUpgradeDrained(ctx, cli); err != nil {
			return result, err
		}
	}
	if os.IsNotExist(err) {
		if _, statErr := os.Lstat(result.StateDir); !os.IsNotExist(statErr) {
			return result, errors.New("Runner state exists without its original home binding; use a fresh application home")
		}
	}
	return result, nil
}

func verifyRunnerUpgradeCapabilities(ctx context.Context, cli runnerclient.Client) error {
	version, versionErr := cli.Version(ctx)
	if versionErr != nil {
		return versionErr
	}
	catalog, catalogErr := cli.OfflineCatalog(ctx, version)
	if catalogErr != nil {
		return catalogErr
	}
	recovery := catalog.Capabilities.Recovery
	if recovery == nil || recovery.StateFormat != runnerclient.StateFormat || !recovery.SameBuildRestart || recovery.ResourceRebuild || recovery.Upgrade == "" {
		return errors.New("selected Runner must publish the v8 restart and drain-before-upgrade contract")
	}
	return nil
}
