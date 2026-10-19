package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	executionprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
)

type component struct {
	dir, protocol, identity, shutdown, exe string
	args                                   []string
}

func components(home string) (component, component) {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	return component{dir: filepath.Join(home, "application-api"), protocol: contract.Protocol, identity: "/v2/application/identity", shutdown: "/v2/application/shutdown", exe: filepath.Join(home, "bin", "steerline"+suffix), args: []string{"serve", "--detached"}}, component{dir: filepath.Join(home, "ui", "web"), protocol: contract.WebProtocol, identity: "/native/identity", shutdown: "/native/shutdown", exe: filepath.Join(home, "bin", "steerline-web"+suffix), args: []string{"serve", "--detached"}}
}

type ComponentStatus struct {
	State      string             `json:"state"`
	Identity   *contract.Identity `json:"identity,omitempty"`
	Diagnostic string             `json:"diagnostic,omitempty"`
	LogPath    string             `json:"log_path"`
}
type ApplicationStatus struct {
	LastShutdown *contract.Shutdown `json:"last_shutdown,omitempty"`
	Backend      ComponentStatus    `json:"backend"`
	Web          ComponentStatus    `json:"web"`
	Projects     []contract.Project `json:"projects"`
	Degraded     bool               `json:"degraded"`
}
type launchRecord struct {
	Attempt string                        `json:"attempt"`
	Process *httpcontract.ProcessIdentity `json:"process,omitempty"`
}

func observeComponent(parent context.Context, home string, c component) (ComponentStatus, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	result := ComponentStatus{State: "unverified", LogPath: filepath.Join(home, "logs", filepath.Base(c.exe)+".log")}
	client, err := localservice.Open(ctx, home, c.dir, c.protocol, c.identity)
	if err == nil {
		defer client.Close()
		id := client.Identity
		result.Identity = &id
		result.State = "ready"
		return result, nil
	}
	var identities []httpcontract.ProcessIdentity
	id, instanceErr := localservice.ReadInstance(home, c.dir, c.protocol)
	if instanceErr == nil {
		result.Identity = &id
		identities = append(identities, id.Process)
	} else if !os.IsNotExist(instanceErr) {
		return result, instanceErr
	}
	var launch launchRecord
	launchErr := localservice.ReadPrivateJSON(filepath.Join(c.dir, "launch.json"), &launch, 64<<10)
	if launchErr == nil {
		if len(launch.Attempt) != 64 || launch.Process == nil {
			return result, errors.New("original process creation is unverified; refusing replacement")
		}
		identities = append(identities, *launch.Process)
	} else if !os.IsNotExist(launchErr) {
		return result, launchErr
	}
	for _, process := range identities {
		exited, probeErr := httpclient.ProcessExited(process)
		if probeErr != nil {
			return result, probeErr
		}
		if !exited {
			result.State = "starting"
			return result, err
		}
	}
	// A discovery file without its instance record is not an empty component.
	if len(identities) == 0 {
		if _, e := os.Stat(filepath.Join(c.dir, "discovery.json")); e == nil {
			return result, errors.New("discovery exists without verifiable creation identity")
		} else if !os.IsNotExist(e) {
			return result, e
		}
	}
	free, leaseErr := localservice.LeaseAvailable(c.dir)
	if leaseErr != nil {
		return result, leaseErr
	}
	if !free {
		return result, errors.New("resident lease is held; process ownership is unverified")
	}
	if len(identities) == 0 {
		result.State = "absent"
	} else {
		result.State = "exited"
	}
	return result, nil
}

func ensureComponent(ctx context.Context, home string, c component) (ComponentStatus, error) {
	observed, err := observeComponent(ctx, home, c)
	if observed.State == "ready" {
		return observed, nil
	}
	if observed.State != "starting" && err != nil {
		return observed, err
	}
	if observed.State == "absent" || observed.State == "exited" {
		if _, err = os.Stat(c.exe); err != nil {
			return observed, fmt.Errorf("missing product executable %s: %w", c.exe, err)
		}
		if err = os.MkdirAll(c.dir, 0700); err != nil {
			return observed, err
		}
		if err = os.MkdirAll(filepath.Join(home, "logs"), 0700); err != nil {
			return observed, err
		}
		log, err := os.OpenFile(observed.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return observed, err
		}
		input, err := os.Open(os.DevNull)
		if err != nil {
			log.Close()
			return observed, err
		}
		command := exec.Command(c.exe, c.args...)
		command.Dir = home
		command.Env = applicationEnvironment(home)
		command.Stdin = input
		command.Stdout = log
		command.Stderr = log
		attempt := localservice.Token()
		if err = ctx.Err(); err == nil {
			err = localservice.Write(c.dir, "launch.json", launchRecord{Attempt: attempt})
		}
		if err != nil {
			log.Close()
			input.Close()
			return observed, err
		}
		identity, startErr := host.StartDetachedProduct(command, executionprocess.NewServiceFacts())
		log.Close()
		input.Close()
		if startErr != nil {
			os.Remove(filepath.Join(c.dir, "launch.json"))
			return observed, startErr
		}
		go command.Wait()
		process := httpcontract.ProcessIdentity{Kind: identity.Kind, PID: identity.PID, Created: identity.Created, Executable: identity.Executable}
		if err = localservice.Write(c.dir, "launch.json", launchRecord{Attempt: attempt, Process: &process}); err != nil {
			return observed, fmt.Errorf("process created but identity recording failed; result unverified: %w", err)
		}
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		observed, err = observeComponent(ctx, home, c)
		if observed.State == "ready" {
			return observed, nil
		}
		if observed.State == "exited" {
			return observed, fmt.Errorf("%s exited before API readiness; see %s", c.exe, observed.LogPath)
		}
		if observed.State == "unverified" {
			return observed, err
		}
		select {
		case <-ctx.Done():
			return observed, fmt.Errorf("%s startup unverified; see %s: %w", c.exe, observed.LogPath, ctx.Err())
		case <-ticker.C:
		}
	}
}

func StatusApplication(ctx context.Context) (ApplicationStatus, error) {
	home := HomeDir()
	backend, web := components(home)
	result := ApplicationStatus{Projects: []contract.Project{}}
	var backendErr, webErr error
	result.Backend, backendErr = observeComponent(ctx, home, backend)
	result.Web, webErr = observeComponent(ctx, home, web)
	if backendErr != nil {
		result.Backend.Diagnostic = backendErr.Error()
	}
	if webErr != nil {
		result.Web.Diagnostic = webErr.Error()
	}
	if result.Backend.State == "ready" {
		client, err := applicationclient.Open(ctx, home)
		if err != nil {
			return result, err
		}
		defer client.Close()
		var catalog contract.Catalog
		if err = client.Get(ctx, "/projects", &catalog); err != nil {
			return result, err
		}
		result.Projects = catalog.Projects
		for _, project := range catalog.Projects {
			if project.RuntimeState == "failed" {
				result.Degraded = true
			}
		}
	}
	if result.Backend.State == "exited" && result.Backend.Identity != nil {
		var shutdown contract.Shutdown
		if err := localservice.ReadPrivateJSON(filepath.Join(backend.dir, "shutdown.json"), &shutdown, 1<<20); err != nil {
			result.Backend.Diagnostic = "shutdown unverified: " + err.Error()
		} else if shutdown.Identity != *result.Backend.Identity {
			result.Backend.Diagnostic = "shutdown belongs to another host"
		} else {
			result.LastShutdown = &shutdown
			if !shutdown.Clean {
				result.Backend.Diagnostic = "shutdown unclean: " + shutdown.Diagnostic
			}
		}
	}
	return result, errors.Join(backendErr, webErr)
}

func StartApplication(ctx context.Context, noOpen bool) (ApplicationStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	release, err := localservice.Coordinate(ctx, HomeDir())
	if err != nil {
		return ApplicationStatus{}, err
	}
	defer release()
	backend, web := components(HomeDir())
	if _, err = ensureComponent(ctx, HomeDir(), backend); err != nil {
		status, _ := StatusApplication(ctx)
		return status, err
	}
	if _, err = ensureComponent(ctx, HomeDir(), web); err != nil {
		status, _ := StatusApplication(ctx)
		return status, fmt.Errorf("backend started; Web failed: %w", err)
	}
	result, err := StatusApplication(ctx)
	if err == nil {
		webClient, openErr := localservice.Open(ctx, HomeDir(), web.dir, web.protocol, web.identity)
		if openErr != nil {
			return result, openErr
		}
		var connected contract.Identity
		openErr = webClient.Request(ctx, "GET", "/native/backend", nil, &connected)
		webClient.Close()
		if openErr != nil {
			return result, openErr
		}
		if result.Backend.Identity == nil || connected != *result.Backend.Identity {
			return result, errors.New("Web is not connected to the verified host instance")
		}
	}
	if err != nil {
		return result, err
	}
	if !noOpen {
		command := exec.CommandContext(ctx, web.exe, "open")
		command.Dir = HomeDir()
		command.Env = applicationEnvironment(HomeDir())
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err = command.Run(); err != nil {
			return result, fmt.Errorf("services ready; browser opening failed: %w", err)
		}
	}
	return result, nil
}

// RestartApplication retains the fixed Runner and its original Runs. A service
// stop may leave remote native executions alive; their status stays unverified.
func RestartApplication(ctx context.Context, noOpen bool) (ApplicationStatus, error) {
	data, err := readConfiguration(GlobalExecutionConfigPath())
	if err != nil {
		return ApplicationStatus{}, err
	}
	var global RunnerExecutionConfig
	if err := decodeStrictJSON(data, &global); err != nil {
		return ApplicationStatus{}, err
	}
	if err := global.Validate(); err != nil {
		return ApplicationStatus{}, err
	}
	if stopErr := StopAll(ctx); stopErr != nil {
		if _, err := VerifyApplicationRestartable(ctx, global.Runner.Executable); err != nil {
			return ApplicationStatus{}, errors.Join(stopErr, err)
		}
	}
	return StartApplication(ctx, noOpen)
}

func requestStop(ctx context.Context, home string, c component) (*contract.Identity, error) {
	status, err := observeComponent(ctx, home, c)
	if status.State == "absent" {
		return nil, nil
	}
	if status.State == "exited" {
		if status.Identity == nil {
			return nil, errors.New("original process exited without a verifiable resident instance")
		}
		return status.Identity, nil
	}
	if status.State != "ready" {
		return status.Identity, fmt.Errorf("%s stop ownership unverified: %w", c.exe, err)
	}
	client, err := localservice.Open(ctx, home, c.dir, c.protocol, c.identity)
	if err != nil {
		return status.Identity, err
	}
	defer client.Close()
	err = client.Request(ctx, "POST", c.shutdown, nil, nil)
	return status.Identity, err
}
func verifyBackendStop(ctx context.Context, home string, c component, id *contract.Identity) error {
	if id == nil {
		return nil
	}
	if err := localservice.WaitExited(ctx, c.dir, *id); err != nil {
		return err
	}
	var result contract.Shutdown
	if err := localservice.ReadPrivateJSON(filepath.Join(c.dir, "shutdown.json"), &result, 1<<20); err != nil {
		return fmt.Errorf("project shutdown result unverified: %w", err)
	}
	if result.Version != 2 || result.Identity != *id || !result.Clean || !result.Runner.Settled() || result.Projects == nil || len(result.Projects) > 512 {
		return fmt.Errorf("original host shutdown is not clean or identity differs: %s", result.Diagnostic)
	}
	seen := map[string]bool{}
	for _, project := range result.Projects {
		if project.ConfigID == "" || seen[project.ConfigID] {
			return errors.New("project shutdown membership is invalid")
		}
		seen[project.ConfigID] = true
		if !project.LocalClosed || !project.LocksReleased || project.Diagnostic != "" {
			return fmt.Errorf("project %s shutdown unverified: %s", project.ConfigID, project.Diagnostic)
		}
	}
	return nil
}
func stopBackend(ctx context.Context, home string) error {
	backend, _ := components(home)
	id, err := requestStop(ctx, home, backend)
	if err != nil {
		return err
	}
	return verifyBackendStop(ctx, home, backend, id)
}
func StopAll(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	home := HomeDir()
	release, err := localservice.Coordinate(ctx, home)
	if err != nil {
		return err
	}
	defer release()
	backend, web := components(home)
	// Receiving the host acknowledgement follows BeginStop's closure of every
	// project's input gate. Independently attempt Web and all project cleanup.
	backendID, backendErr := requestStop(ctx, home, backend)
	webID, webErr := requestStop(ctx, home, web)
	if webErr == nil && webID != nil {
		webErr = localservice.WaitExited(ctx, web.dir, *webID)
	}
	if backendErr == nil {
		backendErr = verifyBackendStop(ctx, home, backend, backendID)
	}
	return errors.Join(backendErr, webErr)
}

func applicationEnvironment(home string) []string {
	env := []string{}
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.EqualFold(key, "STEERLINE_HOME") {
			env = append(env, value)
		}
	}
	return append(env, "STEERLINE_HOME="+home)
}
