package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/pathguard"
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
	Version            int                           `json:"version"`
	StartupID          string                        `json:"startup_id"`
	Status             string                        `json:"status"`
	StartedAt          time.Time                     `json:"started_at"`
	UpdatedAt          time.Time                     `json:"updated_at"`
	RepositoryIdentity string                        `json:"repository_identity"`
	RepositoryPath     string                        `json:"repository_path"`
	RunDir             string                        `json:"run_dir"`
	Remote             string                        `json:"remote"`
	Branch             string                        `json:"branch"`
	Claimant           string                        `json:"claimant"`
	ConfigDigest       string                        `json:"config_digest"`
	InstanceID         string                        `json:"instance_id,omitempty"`
	ProcessIdentity    *hostcontract.ProcessIdentity `json:"process_identity,omitempty"`
	Error              string                        `json:"error,omitempty"`
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
	return pathguard.ResolveProspective(input)
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
	Version         int                          `json:"version"`
	StartupID       string                       `json:"startup_id"`
	ProcessIdentity hostcontract.ProcessIdentity `json:"process_identity"`
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
	return connectServiceWithProbe(ctx, c, func(identity *hostcontract.ProcessIdentity) productapi.ProcessProbe {
		return publicProcessProbe(c.ProcessFacts, identity)
	})
}

func connectServiceWithProbe(ctx context.Context, c Config, probe func(*hostcontract.ProcessIdentity) productapi.ProcessProbe) ServiceConnection {
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
	return connectServiceFromSnapshot(ctx, c, obs, func(identity *hostcontract.ProcessIdentity) productapi.ProcessProbe {
		return publicProcessProbe(c.ProcessFacts, identity)
	})
}

func connectServiceFromSnapshot(ctx context.Context, c Config, obs ObservationSnapshot, probe func(*hostcontract.ProcessIdentity) productapi.ProcessProbe) ServiceConnection {
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
