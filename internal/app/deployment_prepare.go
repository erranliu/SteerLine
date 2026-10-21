package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
)

const DeploymentPreparationContract = "steerline-deployment-preparation/v1"

type DeploymentGoal string

const (
	DeploymentUpdate  DeploymentGoal = "update"
	DeploymentReplace DeploymentGoal = "replace"
)

// These facts have different meanings. In particular, restart eligibility does
// not turn retained native resources into settled resources.
type DeploymentFacts struct {
	HostExit        string `json:"host_exit"`
	ProjectClosure  string `json:"project_closure"`
	WriterExit      string `json:"writer_exit"`
	Compatibility   string `json:"compatibility"`
	NativeResources string `json:"native_resources"`
}

type DeploymentPreparation struct {
	Contract       string                          `json:"contract"`
	Home           string                          `json:"home"`
	Goal           DeploymentGoal                  `json:"goal"`
	InvocationID   string                          `json:"invocation_id"`
	PackageSHA256  string                          `json:"package_sha256"`
	VerifierSHA256 string                          `json:"verifier_sha256"`
	Outcome        string                          `json:"outcome"`
	Source         string                          `json:"source,omitempty"`
	VerifiedAt     time.Time                       `json:"verified_at"`
	Facts          DeploymentFacts                 `json:"facts"`
	Normal         *contract.Shutdown              `json:"normal,omitempty"`
	Stopped        *ApplicationStopVerification    `json:"stopped,omitempty"`
	Restart        *ApplicationRestartVerification `json:"restart,omitempty"`
	Diagnostic     string                          `json:"diagnostic,omitempty"`
}

// PrepareApplicationDeployment owns the deployment decision, not the later
// replacement lease. Its report cannot authorize a reset or be reused as a
// continuously held writer lock. Only update permits the guarded recovery.
func PrepareApplicationDeployment(ctx context.Context, runner string, goal DeploymentGoal, invocationID, packageSHA string) (DeploymentPreparation, error) {
	blocked := DeploymentPreparation{Contract: DeploymentPreparationContract, Home: HomeDir(), Goal: goal, InvocationID: invocationID, PackageSHA256: packageSHA, Outcome: "blocked", Facts: DeploymentFacts{"unverified", "unverified", "unverified", "not_applicable", "unverified"}}
	fail := func(err error) (DeploymentPreparation, error) { blocked.Diagnostic = err.Error(); return blocked, err }
	nonce, err := hex.DecodeString(invocationID)
	if err != nil || len(nonce) != 16 || strings.ToLower(invocationID) != invocationID {
		return fail(errors.New("deployment requires a fresh 128-bit lowercase invocation ID"))
	}
	executable, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	dir := filepath.Dir(executable)
	if canonicalPath(runner) != canonicalPath(filepath.Join(dir, "steerrunner.exe")) || canonicalPath(executable) != canonicalPath(filepath.Join(dir, "steerline.exe")) {
		return fail(errors.New("deployment must use the selected staged package's own orchestrator and Runner"))
	}
	digest, err := deploymentPackageDigest(dir)
	if err != nil {
		return fail(err)
	}
	if digest != packageSHA {
		return fail(errors.New("staged package differs from this deployment invocation"))
	}
	result, err := prepareApplicationDeployment(ctx, runner, goal, cycleOriginalInstallation)
	result.InvocationID, result.PackageSHA256 = invocationID, digest
	if err == nil {
		current, readErr := deploymentPackageDigest(dir)
		if readErr != nil || current != digest {
			result.Outcome = "blocked"
			err = errors.Join(errors.New("staged package changed during preparation"), readErr)
		}
	}
	if err != nil {
		result.Diagnostic = err.Error()
	}
	return result, err
}

// The public package binding uses fixed names and LF separators on all hosts.
func deploymentPackageDigest(dir string) (string, error) {
	var text strings.Builder
	for _, name := range []string{"steerline.exe", "steerline-web.exe", "steerrunner.exe"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(data)
		fmt.Fprintf(&text, "%s:%x\n", name, digest)
	}
	digest := sha256.Sum256([]byte(text.String()))
	return hex.EncodeToString(digest[:]), nil
}

func prepareApplicationDeployment(ctx context.Context, runner string, goal DeploymentGoal, cycle func(context.Context) error) (DeploymentPreparation, error) {
	result := DeploymentPreparation{Contract: DeploymentPreparationContract, Home: HomeDir(), Goal: goal, Outcome: "blocked",
		Facts: DeploymentFacts{"unverified", "unverified", "unverified", "not_applicable", "unverified"}}
	fail := func(err error) (DeploymentPreparation, error) {
		result.Diagnostic = err.Error()
		return result, err
	}
	if goal != DeploymentUpdate && goal != DeploymentReplace {
		return fail(errors.New("deployment goal must be update or replace; reset is read-only and requires its own resource proof"))
	}
	if !filepath.IsAbs(runner) {
		return fail(errors.New("deployment requires an absolute staged Runner executable"))
	}
	data, err := os.ReadFile(runner)
	if err != nil {
		return fail(err)
	}
	digest := sha256.Sum256(data)
	result.VerifierSHA256 = hex.EncodeToString(digest[:])
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	release, err := localservice.Coordinate(ctx, result.Home)
	if err != nil {
		return fail(err)
	}
	defer release()
	stopCtx, stopCancel := context.WithTimeout(ctx, 120*time.Second)
	stopErr := stopAllCoordinated(stopCtx, result.Home)
	stopCancel()
	if stopErr == nil {
		result.Source = "normal_stop"
		backend, _ := components(result.Home)
		status, observeErr := observeComponent(ctx, result.Home, backend)
		if observeErr != nil {
			return fail(observeErr)
		}
		if status.State == "absent" {
			result.Source = "no_original_host"
			result.Facts = DeploymentFacts{"not_applicable", "not_applicable", "not_applicable", "not_applicable", "unverified"}
		} else if status.State == "exited" && status.Identity != nil {
			var normal contract.Shutdown
			if err = localservice.ReadPrivateJSON(filepath.Join(backend.dir, "shutdown.json"), &normal, 1<<20); err != nil {
				return fail(err)
			}
			if normal.Identity != *status.Identity || !normal.Clean || !normal.Runner.Settled() {
				return fail(errors.New("normal shutdown changed after stop verification"))
			}
			result.Normal = &normal
			result.Facts = DeploymentFacts{"verified", "verified", "verified", "not_applicable", "verified"}
		} else {
			return fail(errors.New("application host changed after stop verification"))
		}
	} else {
		restart := goal == DeploymentUpdate
		proof, verifyErr := verifyApplicationOfflineCoordinated(ctx, runner, restart)
		if verifyErr != nil {
			if !restart {
				return fail(errors.Join(stopErr, verifyErr))
			}
			recovered, recoveryErr := recoverApplicationForDeploymentCoordinated(ctx, runner, cycle)
			if recoveryErr != nil {
				return fail(errors.Join(stopErr, verifyErr, recoveryErr))
			}
			result.Restart, result.Source = &recovered, "interrupted_recovery"
		} else if restart {
			restarted := restartVerification(proof, nil)
			result.Restart, result.Source = &restarted, "restart_verification"
		} else {
			result.Stopped, result.Source = &proof.ApplicationStopVerification, "resource_verification"
		}
		result.Facts.HostExit, result.Facts.ProjectClosure, result.Facts.WriterExit = "verified", "verified", "verified"
		if result.Stopped != nil || result.Restart != nil && result.Restart.RunnerService.ResourcesSettled {
			result.Facts.NativeResources = "verified"
		}
		if result.Restart != nil && result.Restart.Takeover != nil {
			result.Facts.Compatibility = "verified"
		}
	}
	// Bind even the clean-stop path to the candidate actually selected for this
	// call. PowerShell checks this same digest before any subsequent replacement.
	if err = runnerclient.VerifyExecutable(runner, result.VerifierSHA256); err != nil {
		return fail(fmt.Errorf("staged Runner changed during preparation: %w", err))
	}
	result.Outcome, result.VerifiedAt = "prepared", time.Now().UTC()
	return result, nil
}
