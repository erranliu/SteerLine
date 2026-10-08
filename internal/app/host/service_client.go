package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/hostport"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

var errLockBusy = errors.New("service lock is held")

const (
	ConnectionNoRecord      = productapi.ConnectionNoRecord
	ConnectionCorrupt       = productapi.ConnectionCorrupt
	ConnectionOffline       = productapi.ConnectionOffline
	ConnectionUnknown       = productapi.ConnectionUnknown
	ConnectionIdentityError = productapi.ConnectionIdentityError
	ConnectionInitializing  = productapi.ConnectionInitializing
	ConnectionStale         = productapi.ConnectionStale
	ConnectionIncompatible  = productapi.ConnectionIncompatible
	ConnectionConfigChanged = productapi.ConnectionConfigChanged
	ConnectionConnected     = productapi.ConnectionConnected
	ProcessAbsent           = productapi.ProcessAbsent
	ProcessMatches          = productapi.ProcessMatches
	ProcessReused           = productapi.ProcessReused
	ProcessUnknown          = productapi.ProcessUnknown
)

// StartupRecord binds a startup attempt to the service process and its target
// configuration. It is diagnostic evidence, not runtime or task state.
type StartupRecord struct {
	Version            int                               `json:"version"`
	StartupID          string                            `json:"startup_id"`
	Status             string                            `json:"status"`
	StartedAt          time.Time                         `json:"started_at"`
	UpdatedAt          time.Time                         `json:"updated_at"`
	RepositoryIdentity string                            `json:"repository_identity"`
	RepositoryPath     string                            `json:"repository_path"`
	RunDir             string                            `json:"run_dir"`
	Remote             string                            `json:"remote"`
	Branch             string                            `json:"branch"`
	Claimant           string                            `json:"claimant"`
	ConfigDigest       string                            `json:"config_digest"`
	InstanceID         string                            `json:"instance_id,omitempty"`
	ProcessIdentity    *executionbackend.ProcessIdentity `json:"process_identity,omitempty"`
	Error              string                            `json:"error,omitempty"`
}

type StartServiceOptions struct {
	// Executable must be explicitly set by development and tests. Production
	// callers leave it empty and use the stable installed binary.
	Executable      string
	ApplicationHome string
	ApplicationBin  string
	WaitTimeout     time.Duration
	PollInterval    time.Duration
}

type ServiceStartResult struct {
	Status       string            `json:"status"` // connected, starting, pending, window-required, failed
	StartupID    string            `json:"startup_id,omitempty"`
	LogPath      string            `json:"log_path,omitempty"`
	LauncherPath string            `json:"launcher_path,omitempty"`
	ConfigPath   string            `json:"config_path,omitempty"`
	Connection   ServiceConnection `json:"connection"`
	Error        string            `json:"error,omitempty"`
}

func startupPath(runDir, id string) (string, error) {
	if !controlIDPattern.MatchString(id) {
		return "", errors.New("startup ID must be 32 lowercase hexadecimal characters")
	}
	runRoot, err := resolveProspectivePath(runDir)
	if err != nil {
		return "", err
	}
	root := filepath.Join(runRoot, "service", "startups")
	resolvedRoot, err := resolveProspectivePath(filepath.Join(runDir, "service", "startups"))
	if err != nil {
		return "", err
	}
	if canonical(resolvedRoot) != canonical(root) {
		return "", errors.New("startup root resolves outside the configured service directory")
	}
	dir := filepath.Join(root, id)
	if !inside(root, dir) {
		return "", errors.New("startup path escapes startup directory")
	}
	if resolvedDir, e := filepath.EvalSymlinks(dir); e == nil {
		if !inside(root, canonical(resolvedDir)) {
			return "", errors.New("startup directory resolves outside startup root")
		}
	} else if !os.IsNotExist(e) {
		return "", e
	}
	return filepath.Join(dir, "startup.json"), nil
}

func startupLogPath(runDir, id string) (string, error) {
	path, err := startupPath(runDir, id)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "service.log"), nil
}

func managedServiceDir(runDir string) (string, error) {
	runRoot, err := resolveProspectivePath(runDir)
	if err != nil {
		return "", err
	}
	expected := filepath.Join(runRoot, "service")
	actual, err := resolveProspectivePath(filepath.Join(runDir, "service"))
	if err != nil {
		return "", err
	}
	if canonical(actual) != canonical(expected) {
		return "", errors.New("service directory resolves outside run_dir")
	}
	return expected, nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed path must not be a symbolic link: %s", path)
	}
	return nil
}

func expectedClaimant(c Config) string {
	if c.Claimant != "" {
		return c.Claimant
	}
	return "steerline-" + shortHash(c.Identity())
}

func resolveRepositoryPath(ctx context.Context, c Config) (string, error) {
	if err := c.ResolveIdentity(ctx); err != nil {
		return "", err
	}
	if strings.TrimSpace(c.Repository) == "" {
		return "", errors.New("configured repository path is empty")
	}
	return canonical(c.Repository), nil
}

func expectedRunDir(c Config) (string, error) {
	return resolveProspectivePath(c.RunDir)
}

func resolveProspectivePath(input string) (string, error) {
	path, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	var tail []string
	for {
		resolved, e := filepath.EvalSymlinks(path)
		if e == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return canonical(resolved), nil
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", e
		}
		tail = append(tail, filepath.Base(path))
		path = parent
	}
}

func readStartupRecord(c Config, id string) (*StartupRecord, error) {
	path, err := startupPath(c.RunDir, id)
	if err != nil {
		return nil, err
	}
	b, err := readBoundedFile(path, 1<<20)
	if err != nil {
		return nil, err
	}
	var record StartupRecord
	if err = decode(b, &record); err != nil {
		return nil, err
	}
	if record.Version != 1 || record.StartupID != id || (record.Status != "starting" && record.Status != "started" && record.Status != "failed") {
		return nil, errors.New("invalid startup record version, ID or status")
	}
	if record.ProcessIdentity == nil {
		// The launcher owns a separate immutable process record. It never edits
		// the child's lifecycle record after process creation.
		b, e := readBoundedFile(filepath.Join(filepath.Dir(path), "process.json"), 1<<20)
		if e == nil {
			var process startupProcessRecord
			if e = decode(b, &process); e != nil || process.Version != 1 || process.StartupID != id {
				return nil, errors.New("invalid startup process record")
			}
			record.ProcessIdentity = &process.ProcessIdentity
		} else if !os.IsNotExist(e) {
			return nil, e
		}
	}
	return &record, nil
}

type startupProcessRecord struct {
	Version         int                              `json:"version"`
	StartupID       string                           `json:"startup_id"`
	ProcessIdentity executionbackend.ProcessIdentity `json:"process_identity"`
}

func recordStartupProcess(c Config, id string, identity executionbackend.ProcessIdentity) error {
	path, err := startupPath(c.RunDir, id)
	if err != nil {
		return err
	}
	b, err := json.Marshal(startupProcessRecord{Version: 1, StartupID: id, ProcessIdentity: identity})
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(filepath.Dir(path), "process.json"), append(b, '\n'))
}

func findLiveStartupAttempt(c Config) (*StartupRecord, productapi.ProcessProbe, string, error) {
	root, err := resolveProspectivePath(filepath.Join(c.RunDir, "service", "startups"))
	if err != nil {
		return nil, productapi.ProcessProbe{}, "", err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, productapi.ProcessProbe{}, "", nil
	}
	if err != nil {
		return nil, productapi.ProcessProbe{}, "", err
	}
	repositoryPath, err := resolveRepositoryPath(context.Background(), c)
	if err != nil {
		return nil, productapi.ProcessProbe{}, "", err
	}
	runDir, err := expectedRunDir(c)
	if err != nil {
		return nil, productapi.ProcessProbe{}, "", err
	}
	var found *StartupRecord
	var foundProbe productapi.ProcessProbe
	var foundDiagnostic string
	for _, entry := range entries {
		if !entry.IsDir() || !controlIDPattern.MatchString(entry.Name()) {
			continue
		}
		record, e := readStartupRecord(c, entry.Name())
		if e != nil {
			return nil, productapi.ProcessProbe{}, "", fmt.Errorf("read startup %s: %w", entry.Name(), e)
		}
		if record.Status != "starting" && record.Status != "started" {
			continue
		}
		if canonical(record.RunDir) != canonical(runDir) {
			continue
		}
		probe := publicProcessProbe(c.ProcessFacts, record.ProcessIdentity)
		if probe.State == ProcessAbsent || probe.State == ProcessReused {
			if record.Status == "starting" {
				record.Status = "failed"
				record.Error = "service process exited before the ready handshake; see startup log"
				_ = writeStartupRecord(c, *record)
			}
			continue
		}
		sameTarget := canonical(record.RepositoryIdentity) == canonical(c.Identity()) && canonical(record.RepositoryPath) == canonical(repositoryPath) && record.Remote == c.Remote && record.Branch == c.Branch && record.Claimant == expectedClaimant(c)
		diagnostic := "a matching startup attempt is still alive; resume observation with its existing startup_id"
		if !sameTarget {
			diagnostic = "a live startup attempt owns this run_dir with a different repository, remote/branch or claimant"
		} else if record.ConfigDigest != c.LoadedConfigDigest() {
			diagnostic = "a live startup attempt uses an older configuration digest; do not launch a replacement"
		}
		if found != nil {
			return nil, productapi.ProcessProbe{State: ProcessUnknown}, "", errors.New("multiple live startup attempts claim the same run_dir")
		}
		found, foundProbe, foundDiagnostic = record, probe, diagnostic
	}
	return found, foundProbe, foundDiagnostic, nil
}

func validateRecordForRuntime(c Config, rt *reconstructiontypes.Runtime, record *StartupRecord, repositoryPath, runDir string) error {
	if canonical(record.RepositoryIdentity) != canonical(rt.Repository.Identity) || canonical(record.RepositoryIdentity) != canonical(c.Identity()) ||
		canonical(record.RepositoryPath) != canonical(repositoryPath) || canonical(record.RunDir) != canonical(runDir) ||
		record.Remote != c.Remote || record.Branch != c.Branch || record.Claimant != expectedClaimant(c) ||
		record.ConfigDigest != rt.Service.ConfigDigest {
		return errors.New("startup record target identity does not match configuration and runtime")
	}
	if record.ProcessIdentity == nil || rt.Service.ProcessIdentity == nil || *record.ProcessIdentity != *rt.Service.ProcessIdentity {
		return errors.New("startup record process identity does not match runtime")
	}
	if record.InstanceID != "" && record.InstanceID != rt.Service.InstanceID {
		return errors.New("startup record belongs to another runtime instance")
	}
	if record.Status == "started" {
		if record.InstanceID == "" || record.InstanceID != rt.Service.InstanceID {
			return errors.New("startup record is not bound to the runtime instance")
		}
		if record.ProcessIdentity == nil || rt.Service.ProcessIdentity == nil || *record.ProcessIdentity != *rt.Service.ProcessIdentity {
			return errors.New("startup record process identity does not match runtime")
		}
	} else if record.Status == "starting" {
		if record.ProcessIdentity != nil && rt.Service.ProcessIdentity != nil && *record.ProcessIdentity != *rt.Service.ProcessIdentity {
			return errors.New("starting record process identity does not match runtime")
		}
	} else if record.Status != "failed" {
		return errors.New("startup record is not active")
	}
	return nil
}

func ConnectService(ctx context.Context, c Config) ServiceConnection {
	return connectServiceWithProbe(ctx, c, func(identity *executionbackend.ProcessIdentity) productapi.ProcessProbe {
		return publicProcessProbe(c.ProcessFacts, identity)
	})
}

func connectServiceWithProbe(ctx context.Context, c Config, probe func(*executionbackend.ProcessIdentity) productapi.ProcessProbe) ServiceConnection {
	connection := ServiceConnection{State: ConnectionNoRecord, Capabilities: []string{}, CanStart: true}
	if err := c.ResolveIdentity(ctx); err != nil {
		connection.State, connection.Diagnostic, connection.CanStart = ConnectionIdentityError, "resolve repository identity: "+err.Error(), false
		return connection
	}
	obs := ReadObservationSnapshot(c, time.Now)
	return connectServiceFromSnapshot(ctx, c, obs, probe)
}

// ConnectServiceFromSnapshot checks process and configuration identity against
// the supplied committed observation, without sampling the database again.
// Control commands still perform their own admission checks.
func ConnectServiceFromSnapshot(ctx context.Context, c Config, obs ObservationSnapshot) ServiceConnection {
	if err := c.ResolveIdentity(ctx); err != nil {
		return ServiceConnection{State: ConnectionIdentityError, Snapshot: obs, Diagnostic: "resolve repository identity: " + err.Error()}
	}
	return connectServiceFromSnapshot(ctx, c, obs, func(identity *executionbackend.ProcessIdentity) productapi.ProcessProbe {
		return publicProcessProbe(c.ProcessFacts, identity)
	})
}

func connectServiceFromSnapshot(ctx context.Context, c Config, obs ObservationSnapshot, probe func(*executionbackend.ProcessIdentity) productapi.ProcessProbe) ServiceConnection {
	connection := ServiceConnection{State: ConnectionNoRecord, Capabilities: []string{}, CanStart: true}
	connection.Snapshot = obs
	if obs.State == SnapshotNone {
		return connection
	}
	if obs.State == SnapshotReadFailed || obs.State == SnapshotUnknownVersion || obs.Runtime == nil {
		connection.State = ConnectionCorrupt
		connection.Diagnostic = obs.Error
		connection.CanStart = false
		return connection
	}
	rt := obs.Runtime
	connection.Protocol = rt.Service.ControlProtocol
	connection.Capabilities = append(connection.Capabilities, rt.Service.Capabilities...)
	if rt.Service.StartupID != nil {
		connection.StartupID = *rt.Service.StartupID
	}
	repositoryPath, err := resolveRepositoryPath(ctx, c)
	if err != nil {
		connection.State, connection.Diagnostic, connection.CanStart = ConnectionIdentityError, "resolve configured repository: "+err.Error(), false
		return connection
	}
	runDir, err := expectedRunDir(c)
	if err != nil {
		connection.State, connection.Diagnostic, connection.CanStart = ConnectionIdentityError, "resolve configured run_dir: "+err.Error(), false
		return connection
	}
	snapshotRepositoryPath, pathErr := filepath.EvalSymlinks(rt.Repository.Path)
	if pathErr != nil || canonical(rt.Repository.Identity) != canonical(c.Identity()) || canonical(snapshotRepositoryPath) != canonical(repositoryPath) || rt.Repository.Remote != c.Remote || rt.Repository.Branch != c.Branch || rt.Service.Claimant != expectedClaimant(c) {
		connection.State = ConnectionIdentityError
		connection.Diagnostic = "runtime repository, remote/branch or claimant does not match this configuration"
		connection.CanStart = false
		return connection
	}
	connection.ConfigMatched = c.LoadedConfigDigest() != "" && rt.Service.ConfigDigest == c.LoadedConfigDigest()
	var record *StartupRecord
	recordTargetsRuntime := false
	if rt.Service.StartupID != nil {
		record, err = readStartupRecord(c, *rt.Service.StartupID)
		if err == nil && validateRecordForRuntime(c, rt, record, repositoryPath, runDir) == nil {
			recordTargetsRuntime = true
		}
	}
	instanceBound := (rt.Service.StartupID != nil && recordTargetsRuntime) || (rt.Service.StartupID == nil && connection.ConfigMatched)
	if rt.Service.ProcessIdentity == nil || rt.Service.PID <= 0 || rt.Service.ProcessIdentity.PID != rt.Service.PID {
		connection.Process = productapi.ProcessProbe{State: ProcessUnknown, Reason: "runtime has no internally consistent service process identity"}
	} else {
		connection.Process = probe(rt.Service.ProcessIdentity)
	}
	// Database sampling can finish long after it began, or the caller can reuse
	// an older observation. Freshness belongs to this connection check, while
	// SampledAt continues to identify the supplied database sample.
	if obs.State == SnapshotValid || obs.State == SnapshotStale {
		sampledAt := obs.SampledAt
		obs = ObserveRuntimeSnapshot(c, rt, nil, time.Now())
		obs.SampledAt = sampledAt
		connection.Snapshot = obs
	}
	if connection.Process.State == ProcessAbsent || connection.Process.State == ProcessReused {
		if !instanceBound {
			connection.State = ConnectionIdentityError
			connection.Diagnostic = "cannot verify that the recorded service instance owns the configured run_dir"
			connection.CanStart = false
			return connection
		}
		connection.State = ConnectionOffline
		connection.Diagnostic = connection.Process.Reason
		connection.CanStart = true
		for _, path := range []string{filepath.Join(c.Identity(), "steerline.lock"), filepath.Join(c.RunDir, "steerline.lock")} {
			held, e := probeLockFile(path)
			if e != nil || held {
				connection.CanStart = false
				connection.Diagnostic = "original process exited, but service lock release is not confirmed"
				if e != nil {
					connection.Diagnostic += ": " + e.Error()
				}
				break
			}
		}
		return connection
	}
	if connection.Process.State == ProcessUnknown {
		connection.State = ConnectionUnknown
		connection.Diagnostic = connection.Process.Reason
		connection.CanStart = false
		return connection
	}
	if !instanceBound {
		connection.State = ConnectionIdentityError
		connection.Diagnostic = "cannot verify that this process instance owns the configured run_dir"
		connection.CanStart = false
		return connection
	}
	connection.IdentityMatched = true
	connection.CanStop = rt.Service.ControlProtocol == ControlProtocolVersion && serviceHasCapability(rt, "stop_service")
	if !connection.ConfigMatched {
		connection.State = ConnectionConfigChanged
		connection.Diagnostic = "configuration changed since this service started; observation and explicit stop are available"
		if !connection.CanStop {
			connection.Diagnostic += "; this service does not advertise a compatible stop capability"
		}
		connection.CanStart = false
		return connection
	}
	if record != nil && record.Status == "failed" {
		connection.State = ConnectionInitializing
		connection.Diagnostic = record.Error
		if connection.Diagnostic == "" {
			connection.Diagnostic = "startup failed; the matching process is still present"
		}
		connection.CanStart = false
		return connection
	}
	if record != nil && record.Status == "starting" {
		connection.State = ConnectionInitializing
		connection.Diagnostic = "startup handshake has not been published"
		connection.CanStart = false
		return connection
	}
	if obs.State == SnapshotStale {
		connection.State = ConnectionStale
		connection.Diagnostic = "service heartbeat is stale; the matching process is still present"
		connection.CanStart = false
		return connection
	}
	if rt.Service.Status != "running" && rt.Service.Status != "error" || !rt.Service.Ready {
		connection.State = ConnectionInitializing
		connection.Diagnostic = "service process is alive but has not published a ready snapshot"
		connection.CanStart = false
		return connection
	}
	if rt.Service.ControlProtocol != ControlProtocolVersion {
		connection.State = ConnectionIncompatible
		connection.Diagnostic = fmt.Sprintf("service control protocol %d is incompatible with client protocol %d", rt.Service.ControlProtocol, ControlProtocolVersion)
		connection.CanStop = false
		connection.CanStart = false
		return connection
	}
	connection.State = ConnectionConnected
	connection.CanControl = true
	connection.CanStart = false
	if rt.Service.Status == "error" && rt.Service.Error != nil {
		connection.Diagnostic = *rt.Service.Error
	}
	return connection
}

func createStartupAttempt(c Config, id string, createLog bool) (string, string, error) {
	jsonPath, err := startupPath(c.RunDir, id)
	if err != nil {
		return "", "", err
	}
	logPath, err := startupLogPath(c.RunDir, id)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Dir(jsonPath)
	if err = os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return "", "", err
	}
	if err = os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", "", err
	}
	if resolved, e := filepath.EvalSymlinks(dir); e != nil || !inside(filepath.Dir(filepath.Dir(dir)), resolved) {
		if e != nil {
			return "", "", e
		}
		return "", "", errors.New("startup directory resolves outside its managed root")
	}
	if createLog {
		f, e := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return "", "", e
		}
		if e = f.Close(); e != nil {
			return "", "", e
		}
	}
	return jsonPath, logPath, nil
}

func initialStartupRecord(c Config, id string) StartupRecord {
	return StartupRecord{Version: 1, StartupID: id, Status: "starting", StartedAt: now(), UpdatedAt: now(), RepositoryIdentity: c.Identity(), RepositoryPath: c.Repository, RunDir: c.RunDir, Remote: c.Remote, Branch: c.Branch, Claimant: expectedClaimant(c), ConfigDigest: c.LoadedConfigDigest()}
}

func writeStartupRecord(c Config, record StartupRecord) error {
	path, err := startupPath(c.RunDir, record.StartupID)
	if err != nil {
		return err
	}
	if record.Version != 1 || record.Status != "starting" && record.Status != "started" && record.Status != "failed" {
		return errors.New("invalid startup record")
	}
	record.UpdatedAt = now()
	b, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(b, '\n'))
}

func failStartup(c Config, id string, cause error) error {
	if id == "" {
		return cause
	}
	if _, _, err := createStartupAttempt(c, id, false); err != nil {
		return errors.Join(cause, fmt.Errorf("prepare startup diagnostic: %w", err))
	}
	record := initialStartupRecord(c, id)
	if old, err := readStartupRecord(c, id); err == nil {
		record = *old
	}
	record.Status = "failed"
	if cause != nil {
		record.Error = clip(cause.Error(), 4000)
	}
	return errors.Join(cause, writeStartupRecord(c, record))
}

func windowsServiceWindowStart(ctx context.Context, c Config, absoluteConfig string, opts StartServiceOptions) (ServiceStartResult, error) {
	connection := ConnectService(ctx, c)
	switch connection.State {
	case ConnectionConnected:
		return ServiceStartResult{Status: "connected", Connection: connection}, nil
	case ConnectionInitializing:
		logPath, _ := startupLogPath(c.RunDir, connection.StartupID)
		return ServiceStartResult{Status: "pending", StartupID: connection.StartupID, LogPath: logPath, Connection: connection, Error: connection.Diagnostic}, nil
	case ConnectionStale, ConnectionUnknown, ConnectionIdentityError, ConnectionCorrupt, ConnectionIncompatible, ConnectionConfigChanged:
		return ServiceStartResult{}, fmt.Errorf("refusing to replace a service with uncertain or incompatible state: %s: %s", connection.State, connection.Diagnostic)
	case ConnectionOffline, ConnectionNoRecord:
		if !connection.CanStart {
			return ServiceStartResult{Status: "pending", Connection: connection, Error: connection.Diagnostic}, nil
		}
	default:
		return ServiceStartResult{}, fmt.Errorf("cannot offer service window from connection state %q", connection.State)
	}
	pending, probe, diagnostic, err := findLiveStartupAttempt(c)
	if err != nil {
		return ServiceStartResult{}, fmt.Errorf("inspect prior service startup attempts: %w", err)
	}
	if pending != nil {
		logPath, _ := startupLogPath(c.RunDir, pending.StartupID)
		connection.CanStart = false
		connection.Process = probe
		connection.Diagnostic = diagnostic
		connection.StartupID = pending.StartupID
		connection.ConfigMatched = pending.ConfigDigest == c.LoadedConfigDigest()
		switch {
		case probe.State == ProcessUnknown:
			connection.State = ConnectionUnknown
			connection.Diagnostic += ": " + probe.Reason
		case canonical(pending.RepositoryIdentity) != canonical(c.Identity()) ||
			canonical(pending.RepositoryPath) != canonical(c.Repository) ||
			pending.Remote != c.Remote || pending.Branch != c.Branch ||
			pending.Claimant != expectedClaimant(c):
			connection.State = ConnectionIdentityError
		case !connection.ConfigMatched:
			connection.State = ConnectionConfigChanged
		default:
			connection.State = ConnectionInitializing
		}
		return ServiceStartResult{Status: "pending", StartupID: pending.StartupID, LogPath: logPath, Connection: connection, Error: connection.Diagnostic}, nil
	}
	result := ServiceStartResult{
		Status:       "window-required",
		LauncherPath: filepath.Join(opts.ApplicationHome, "service-window.bat"),
		ConfigPath:   absoluteConfig,
		Connection:   connection,
	}
	for _, candidate := range []struct {
		path    string
		install string
	}{
		{result.LauncherPath, "steerline launchers"},
		{filepath.Join(opts.ApplicationBin, "steerline.exe"), "steerline install"},
	} {
		info, statErr := os.Stat(candidate.path)
		if statErr != nil || info.IsDir() {
			if statErr == nil {
				statErr = errors.New("path is a directory")
			}
			result.Status = "failed"
			result.Error = fmt.Sprintf("service window installation is unavailable at %s: %v; run %s", candidate.path, statErr, candidate.install)
			return result, nil
		}
	}
	return result, nil
}

func StartService(ctx context.Context, c Config, configPath string, opts StartServiceOptions) (ServiceStartResult, error) {
	if err := ctx.Err(); err != nil {
		return ServiceStartResult{}, err
	}
	if err := c.ValidateTaskCommandConfiguration(); err != nil {
		return ServiceStartResult{}, err
	}
	if c.ProcessFacts == nil {
		return ServiceStartResult{}, errors.New("application process-facts port is required")
	}
	var err error
	if err = c.ResolveIdentity(ctx); err != nil {
		return ServiceStartResult{}, fmt.Errorf("resolve service target: %w", err)
	}
	if c.Claimant == "" {
		c.Claimant = expectedClaimant(c)
	}
	c.Repository, err = resolveRepositoryPath(ctx, c)
	if err != nil {
		return ServiceStartResult{}, fmt.Errorf("resolve configured repository: %w", err)
	}
	c.RunDir, err = expectedRunDir(c)
	if err != nil {
		return ServiceStartResult{}, fmt.Errorf("resolve configured run_dir: %w", err)
	}
	if c.LoadedConfigDigest() == "" {
		return ServiceStartResult{}, errors.New("start-service requires a config loaded from a file")
	}
	absoluteConfig, err := filepath.Abs(configPath)
	if err != nil {
		return ServiceStartResult{}, err
	}
	if inside(c.Repository, c.RunDir) {
		return ServiceStartResult{}, errors.New("run_dir must be outside the configured repository")
	}
	if runtimeGOOSWindows() {
		if opts.ApplicationHome == "" || opts.ApplicationBin == "" {
			return ServiceStartResult{}, errors.New("application home and binary directories are required")
		}
		return windowsServiceWindowStart(ctx, c, absoluteConfig, opts)
	}
	if opts.WaitTimeout <= 0 || opts.WaitTimeout > 30*time.Second {
		opts.WaitTimeout = 30 * time.Second
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 100 * time.Millisecond
	}
	executable := opts.Executable
	if executable == "" {
		if opts.ApplicationBin == "" {
			return ServiceStartResult{}, errors.New("application binary directory is required")
		}
		executable = filepath.Join(opts.ApplicationBin, "steerline")
		if runtimeGOOSWindows() {
			executable += ".exe"
		}
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return ServiceStartResult{}, err
	}
	serviceDir, err := managedServiceDir(c.RunDir)
	if err != nil {
		return ServiceStartResult{}, err
	}
	if err = os.MkdirAll(serviceDir, 0700); err != nil {
		return ServiceStartResult{}, err
	}
	if resolved, e := filepath.EvalSymlinks(serviceDir); e != nil || canonical(resolved) != canonical(serviceDir) {
		if e != nil {
			return ServiceStartResult{}, e
		}
		return ServiceStartResult{}, errors.New("service directory changed while preparing its launch lock")
	}
	launchLockPath := filepath.Join(serviceDir, "launch.lock")
	if err = rejectSymlink(launchLockPath); err != nil {
		return ServiceStartResult{}, err
	}
	launchLock, err := acquireLaunchLock(ctx, launchLockPath)
	if err != nil {
		return ServiceStartResult{}, err
	}
	defer launchLock.Close()
	connection := ConnectService(ctx, c)
	switch connection.State {
	case ConnectionConnected:
		return ServiceStartResult{Status: "connected", Connection: connection}, nil
	case ConnectionInitializing:
		if connection.StartupID == "" {
			return ServiceStartResult{Status: "pending", Connection: connection, Error: connection.Diagnostic}, nil
		}
		logPath, _ := startupLogPath(c.RunDir, connection.StartupID)
		return waitForStartup(ctx, c, connection.StartupID, logPath, opts)
	case ConnectionStale, ConnectionUnknown, ConnectionIdentityError, ConnectionCorrupt, ConnectionIncompatible, ConnectionConfigChanged:
		return ServiceStartResult{}, fmt.Errorf("refusing to start a replacement service: %s: %s", connection.State, connection.Diagnostic)
	case ConnectionOffline, ConnectionNoRecord:
		if !connection.CanStart {
			return ServiceStartResult{Status: "pending", Connection: connection, Error: connection.Diagnostic}, nil
		}
	default:
		return ServiceStartResult{}, fmt.Errorf("cannot start service from connection state %q", connection.State)
	}
	if connection.State == ConnectionOffline || connection.State == ConnectionNoRecord {
		pending, probe, diagnostic, scanErr := findLiveStartupAttempt(c)
		if scanErr != nil {
			return ServiceStartResult{}, fmt.Errorf("inspect prior service startup attempts: %w", scanErr)
		}
		if pending != nil {
			logPath, _ := startupLogPath(c.RunDir, pending.StartupID)
			if probe.State == ProcessUnknown {
				connection.State = ConnectionUnknown
				connection.Process = probe
				connection.Diagnostic = diagnostic + ": " + probe.Reason
				connection.CanStart = false
				return ServiceStartResult{Status: "pending", StartupID: pending.StartupID, LogPath: logPath, Connection: connection, Error: connection.Diagnostic}, nil
			}
			if pending.ConfigDigest != c.LoadedConfigDigest() {
				connection.State = ConnectionConfigChanged
				connection.Process = probe
				connection.Diagnostic = diagnostic
				connection.CanStart = false
				return ServiceStartResult{Status: "pending", StartupID: pending.StartupID, LogPath: logPath, Connection: connection, Error: diagnostic}, nil
			}
			if canonical(pending.RepositoryIdentity) != canonical(c.Identity()) || pending.Remote != c.Remote || pending.Branch != c.Branch || pending.Claimant != expectedClaimant(c) {
				connection.State = ConnectionIdentityError
				connection.Process = probe
				connection.Diagnostic = diagnostic
				connection.CanStart = false
				return ServiceStartResult{Status: "pending", StartupID: pending.StartupID, LogPath: logPath, Connection: connection, Error: diagnostic}, nil
			}
			return waitForStartup(ctx, c, pending.StartupID, logPath, opts)
		}
	}
	if err = ctx.Err(); err != nil {
		return ServiceStartResult{}, err
	}
	if info, e := os.Stat(executable); e != nil || info.IsDir() {
		if e == nil {
			e = errors.New("path is a directory")
		}
		return ServiceStartResult{}, fmt.Errorf("installed service executable %s is unavailable: %w; run steerline install", executable, e)
	}
	startupID := newID()
	_, logPath, err := createStartupAttempt(c, startupID, true)
	if err != nil {
		return ServiceStartResult{}, err
	}
	if err = writeStartupRecord(c, initialStartupRecord(c, startupID)); err != nil {
		return ServiceStartResult{}, err
	}
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		_ = failStartup(c, startupID, err)
		return ServiceStartResult{}, err
	}
	defer stdin.Close()
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		_ = failStartup(c, startupID, err)
		return ServiceStartResult{}, err
	}
	defer logFile.Close()
	cmd := exec.Command(executable, "run", "-config", absoluteConfig, "-dispatch", "manual", "-console", "off", "-startup-id", startupID)
	cmd.Dir = c.Repository
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, logFile, logFile
	if err = ctx.Err(); err != nil {
		_ = failStartup(c, startupID, err)
		return ServiceStartResult{Status: "failed", StartupID: startupID, LogPath: logPath, Error: err.Error()}, nil
	}
	identity, err := startDetachedProcess(cmd, c.ProcessFacts)
	if err != nil {
		_ = failStartup(c, startupID, err)
		return ServiceStartResult{Status: "failed", StartupID: startupID, LogPath: logPath, Error: err.Error()}, nil
	}
	err = recordStartupProcess(c, startupID, identity)
	// Reap a child that exits while this UI remains open (notably on Linux).
	// Waiting does not bind its lifetime to ours or kill it when this UI exits.
	go func() { _ = cmd.Wait() }()
	if err != nil {
		return ServiceStartResult{Status: "pending", StartupID: startupID, LogPath: logPath, Error: "process created, but recording its identity failed: " + err.Error()}, nil
	}
	return waitForStartup(ctx, c, startupID, logPath, opts)
}

func waitForStartup(ctx context.Context, c Config, id, logPath string, opts StartServiceOptions) (ServiceStartResult, error) {
	if opts.WaitTimeout <= 0 || opts.WaitTimeout > 30*time.Second {
		opts.WaitTimeout = 30 * time.Second
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 100 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, opts.WaitTimeout)
	defer cancel()
	tick := time.NewTicker(opts.PollInterval)
	defer tick.Stop()
	result := ServiceStartResult{Status: "starting", StartupID: id, LogPath: logPath}
	for {
		connection := ConnectService(ctx, c)
		result.Connection = connection
		if connection.State == ConnectionConnected && connection.StartupID == id {
			result.Status = "connected"
			return result, nil
		}
		if record, err := readStartupRecord(c, id); err == nil && record.Status == "failed" {
			result.Status, result.Error = "failed", record.Error
			if result.Error == "" {
				result.Error = "service startup failed; see startup log"
			}
			return result, nil
		} else if err == nil && record.ProcessIdentity != nil {
			probe := publicProcessProbe(c.ProcessFacts, record.ProcessIdentity)
			if probe.State == ProcessAbsent || probe.State == ProcessReused {
				record.Status = "failed"
				record.Error = "service process exited before the client confirmed the ready handshake; see startup log"
				_ = writeStartupRecord(c, *record)
				result.Status, result.Error = "failed", record.Error
				return result, nil
			}
		}
		select {
		case <-ctx.Done():
			result.Status = "pending"
			if result.Error == "" {
				result.Error = "startup result is pending after cancellation or wait timeout; observe this startup_id before requesting another start"
			}
			return result, nil
		case <-tick.C:
		}
	}
}

func StopService(ctx context.Context, c Config, requestedBy string, timeout time.Duration) (productapi.ServiceStopResult, error) {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return productapi.ServiceStopResult{}, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return productapi.ServiceStopResult{}, fmt.Errorf("resolve service repository identity: %w", err)
	}
	var err error
	c.Repository, err = resolveRepositoryPath(ctx, c)
	if err != nil {
		return productapi.ServiceStopResult{}, fmt.Errorf("resolve configured repository: %w", err)
	}
	c.RunDir, err = expectedRunDir(c)
	if err != nil {
		return productapi.ServiceStopResult{}, fmt.Errorf("resolve configured run_dir: %w", err)
	}
	connection := ConnectService(ctx, c)
	if !connection.CanStop {
		return productapi.ServiceStopResult{}, fmt.Errorf("cannot stop service in state %q: %s", connection.State, connection.Diagnostic)
	}
	if err := ctx.Err(); err != nil {
		return productapi.ServiceStopResult{}, err
	}
	request, err := submitControlSnapshot(c, connection.Snapshot.Runtime, "stop_service", nil, requestedBy)
	if err != nil {
		return productapi.ServiceStopResult{}, err
	}
	result := productapi.ServiceStopResult{Status: "stopping", RequestID: request.RequestID, InstanceID: request.InstanceID}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		receipt, e := ReadControlReceipt(c, request.RequestID)
		if e == nil {
			if receipt.RequestID != request.RequestID || receipt.InstanceID != request.InstanceID || receipt.Action != "stop_service" || receipt.RequestDigest != rawRequestDigest(controlRequestPath(c.RunDir, request.RequestID)) {
				result.Status = "failed"
				result.Diagnostic = "stop receipt does not match the submitted request and service instance"
				return result, nil
			}
			result.Receipt = receipt
			if c.ControlValidation != nil && c.ControlValidation.TerminalStatus(receipt.Status) {
				switch receipt.Status {
				case "applied":
					result.Status = "stopping"
				case "rejected":
					result.Status = "rejected"
					result.Diagnostic = receipt.Message
					return result, nil
				default:
					result.Status = "failed"
					result.Diagnostic = receipt.Message
					return result, nil
				}
			}
		} else if !os.IsNotExist(e) {
			result.Diagnostic = e.Error()
		}
		{
			probe := publicProcessProbe(c.ProcessFacts, connection.Snapshot.Runtime.Service.ProcessIdentity)
			if probe.State == ProcessAbsent || probe.State == ProcessReused {
				result.ProcessExited = true
				locks, lockErr := serviceLocksReleased(c)
				result.LocksReleased = locks
				if lockErr != nil {
					result.Diagnostic = lockErr.Error()
				}
				if result.LocksReleased && result.Receipt != nil && result.Receipt.Status == "applied" {
					result.Status = "stopped"
					return result, nil
				}
				if result.Receipt == nil || result.Receipt.Status != "applied" {
					result.Diagnostic = "original service process exited; stop receipt is not confirmed"
				}
			} else if probe.State == ProcessUnknown {
				result.Diagnostic = "original service process exit is unverifiable: " + probe.Reason
			}
		}
		select {
		case <-ctx.Done():
			result.Status = "pending"
			if result.Diagnostic == "" {
				result.Diagnostic = "stop result remains pending; UI exit or timeout does not stop the service"
			}
			return result, nil
		case <-deadline.C:
			result.Status = "pending"
			if result.Diagnostic == "" {
				result.Diagnostic = "stop receipt applied or submitted, but process exit and lock release are still being confirmed"
			}
			return result, nil
		case <-tick.C:
		}
	}
}

func serviceLocksReleased(c Config) (bool, error) {
	repoLock, err := tryExistingLock(filepath.Join(c.Identity(), "steerline.lock"))
	if err != nil {
		if errors.Is(err, errLockBusy) {
			return false, nil
		}
		return false, err
	}
	if repoLock == nil {
		return false, nil
	}
	defer repoLock.Close()
	runLock, err := tryExistingLock(filepath.Join(c.RunDir, "steerline.lock"))
	if err != nil {
		if errors.Is(err, errLockBusy) {
			return false, nil
		}
		return false, err
	}
	if runLock == nil {
		return false, nil
	}
	defer runLock.Close()
	return true, nil
}

func runtimeGOOSWindows() bool { return os.PathSeparator == '\\' }

func acquireLaunchLock(ctx context.Context, path string) (*RepoLock, error) {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lock, err := lockPath(path)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, errLockBusy) {
			return nil, fmt.Errorf("acquire service launch lock: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for service launch lock: %w", ctx.Err())
		case <-deadline.C:
			return nil, errors.New("timed out waiting for service launch lock")
		case <-tick.C:
		}
	}
}

// RunConfiguredService is the lock-owning service entry point used by the CLI.
// It records preflight and lock failures under startup_id before returning.
type ServiceLifetime interface {
	Run(context.Context) error
	Close() error
}

type ServiceFactory func(Config, hostport.HostPort, string) (ServiceLifetime, error)

func runServiceLifetime(ctx context.Context, lifetime ServiceLifetime) error {
	runErr := lifetime.Run(ctx)
	return errors.Join(runErr, lifetime.Close())
}

func RunConfiguredService(ctx context.Context, c Config, mode, startupID, console string, construct ServiceFactory, admission ApplicationAdmission, formatter ActivityFormatter) (retErr error) {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return err
	}
	if construct == nil {
		return errors.New("application service constructor is required")
	}
	if admission == nil {
		return errors.New("application service admission is required")
	}
	if err := c.ValidateTaskCommandConfiguration(); err != nil {
		return err
	}
	if c.ProcessFacts == nil {
		return errors.New("application process-facts port is required")
	}
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return err
	}
	if c.WorkflowProtocol != executionmodel.Version {
		return nativeErr("unsupported_schema", "only worker-flow-v1 is supported")
	}
	if mode != "manual" && mode != "auto" {
		return errors.New("dispatch mode must be manual or auto")
	}
	windowGeneratedID := startupID == ""
	if startupID == "" {
		startupID = newID()
	}
	if !controlIDPattern.MatchString(startupID) {
		return errors.New("startup ID must be 32 lowercase hexadecimal characters")
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return err
	}
	c.Repository, retErr = resolveRepositoryPath(ctx, c)
	if retErr != nil {
		return retErr
	}
	if c.Claimant == "" {
		c.Claimant = expectedClaimant(c)
	}
	c.RunDir, retErr = expectedRunDir(c)
	if retErr != nil {
		return retErr
	}
	if inside(c.Repository, c.RunDir) {
		return errors.New("run_dir must be outside the configured repository")
	}
	if err := admission.ValidateStateLocation(c); err != nil {
		return fmt.Errorf("native state location: %w", err)
	}
	_, logPath, retErr := createStartupAttempt(c, startupID, windowGeneratedID)
	if retErr != nil {
		return retErr
	}
	processIdentity, retErr := c.ProcessFacts.CurrentIdentity()
	if retErr != nil {
		// Without our own identity, we cannot claim or modify an existing attempt.
		return fmt.Errorf("identify service process: %w", retErr)
	}
	record := initialStartupRecord(c, startupID)
	record.ProcessIdentity = processIdentity
	if previous, err := readStartupRecord(c, startupID); err == nil {
		if previous.Status != "starting" || canonical(previous.RepositoryIdentity) != canonical(record.RepositoryIdentity) || canonical(previous.RunDir) != canonical(record.RunDir) || previous.Remote != record.Remote || previous.Branch != record.Branch || previous.Claimant != record.Claimant || previous.ConfigDigest != record.ConfigDigest {
			return errors.New("startup ID already belongs to a different or completed attempt")
		}
		record = *previous
		if record.ProcessIdentity != nil && *record.ProcessIdentity != *processIdentity {
			return errors.New("startup record is bound to a different OS process")
		}
		record.ProcessIdentity = processIdentity
	} else if !os.IsNotExist(err) {
		return err
	}
	// Publish the child's identity before slow preflight. The launcher only
	// writes process.json, so it cannot roll back a later started/failed state.
	if err := writeStartupRecord(c, record); err != nil {
		return err
	}
	// Only an attempt bound to this process may append diagnostics or be marked
	// failed. Opening the log earlier could corrupt another instance's evidence.
	if err := rejectSymlink(logPath); err != nil {
		return failStartup(c, startupID, err)
	}
	logFile, retErr := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0600)
	if retErr != nil {
		return failStartup(c, startupID, fmt.Errorf("open service startup log: %w", retErr))
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "startup_id=%s dispatch=%s pid=%d console_window=0x%x\n", startupID, mode, os.Getpid(), serviceConsoleWindow())
	fmt.Fprintf(logFile, "runtime executable=%s\n", diagnosticValue(c.RuntimeExecutable))
	fmt.Fprintf(logFile, "runtime home=%s\n", diagnosticValue(c.RuntimeHome))
	fmt.Fprintf(logFile, "runtime config=%s\n", diagnosticValue(c.RuntimeConfig))
	fmt.Fprintf(logFile, "runtime source_commit=%s source_dirty=%s\n", diagnosticValue(c.SourceCommit), diagnosticValue(c.SourceDirty))
	fmt.Fprintf(logFile, "runtime repository=%s repository_identity=%s run_dir=%s\n", c.Repository, c.Identity(), c.RunDir)
	if console != "off" && console != "json" && (console != "" || ConsoleDefault() != "") {
		fmt.Fprintf(os.Stdout, "SteerLine starting · dispatch %s · startup %s\n", mode, startupID)
	}
	fail := func(err error) error {
		fmt.Fprintf(logFile, "startup failed: %v\n", err)
		failed := record
		failed.Status = "failed"
		failed.Error = clip(err.Error(), 4000)
		return errors.Join(err, writeStartupRecord(c, failed))
	}
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return fail(err)
	}
	if c, retErr = admission.Preflight(ctx, c, true); retErr != nil {
		return fail(fmt.Errorf("service preflight: %w", retErr))
	}
	if initErr := admission.InitializeNativeProject(ctx, c); initErr != nil {
		return fail(fmt.Errorf("initialize native project authority: %w", initErr))
	}
	// Preflight canonicalizes paths and applies the same defaults used by the
	// service. Rewrite only this attempt's non-authoritative startup record.
	processIdentity, retErr = c.ProcessFacts.CurrentIdentity()
	if retErr != nil {
		return fail(fmt.Errorf("identify service process: %w", retErr))
	}
	record = initialStartupRecord(c, startupID)
	record.ProcessIdentity = processIdentity
	if retErr = writeStartupRecord(c, record); retErr != nil {
		return retErr
	}
	repoLock, err := LockRepository(c.Identity())
	if err != nil {
		return fail(err)
	}
	defer repoLock.Close()
	runLock, err := LockRepository(c.RunDir)
	if err != nil {
		return fail(err)
	}
	defer runLock.Close()
	selectedConsole := console
	if selectedConsole == "" {
		selectedConsole = ConsoleDefault()
	}
	host := NewServiceHost(c, startupID, selectedConsole, io.MultiWriter(logFile, os.Stdout), admission, formatter)
	runtime, err := construct(c, host, startupID)
	if err != nil {
		return fail(err)
	}
	if runtime == nil {
		return fail(errors.New("application service factory returned nil lifetime"))
	}
	closed := false
	defer func() {
		if !closed {
			retErr = errors.Join(retErr, runtime.Close())
		}
	}()
	runErr := runServiceLifetime(ctx, runtime)
	closed = true
	retErr = runErr
	if retErr != nil {
		fmt.Fprintf(logFile, "service exited: %v\n", retErr)
	} else {
		fmt.Fprintln(logFile, "service exited")
	}
	if rec, e := readStartupRecord(c, startupID); e == nil && rec.Status == "starting" {
		rec.Status = "failed"
		if retErr != nil {
			rec.Error = clip(retErr.Error(), 4000)
		} else {
			rec.Error = "service exited before publishing a ready runtime snapshot"
		}
		retErr = errors.Join(retErr, writeStartupRecord(c, *rec))
	}
	return retErr
}

func diagnosticValue(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
