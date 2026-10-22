package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"path/filepath"
	"strings"
	"time"
)

type ServiceInfo struct {
	InstanceID       string           `json:"instance_id"`
	PID              int              `json:"pid"`
	ProcessCreated   string           `json:"process_created"`
	StateDir         string           `json:"state_dir"`
	Status           string           `json:"status"`
	Version          string           `json:"version"`
	Protocol         string           `json:"protocol"`
	ResourceDigest   string           `json:"resource_digest"`
	ExecutableSHA256 string           `json:"executable_sha256"`
	LastShutdown     *ServiceShutdown `json:"last_shutdown,omitempty"`
}

type ServiceShutdown struct {
	InstanceID       string `json:"instance_id"`
	RequestID        string `json:"request_id"`
	StateDir         string `json:"state_dir"`
	ExecutableSHA256 string `json:"executable_sha256"`
	ResourcesSettled bool   `json:"resources_settled"`
	Diagnostic       string `json:"diagnostic,omitempty"`
}

// VerifyServiceSettlement consumes the original Runner's post-shutdown proof.
// It never starts a service or substitutes a cached candidate projection.
func (c Client) VerifyServiceSettlement(ctx context.Context, instance, requestID string) error {
	info, err := c.ServiceStatus(ctx)
	if err != nil {
		return err
	}
	return c.verifyServiceSettlement(info, instance, requestID)
}

func (c Client) verifyServiceSettlement(info ServiceInfo, instance, requestID string) error {
	if err := c.verifyServiceStopped(info, instance, requestID); err != nil {
		return err
	}
	r := info.LastShutdown
	if !r.ResourcesSettled || r.Diagnostic != "" {
		return errors.New("original Runner native resource settlement remains unverified: " + r.Diagnostic)
	}
	return nil
}

// VerifyServiceStopped proves only that the original service exited and released
// its writer lease. Native executions may remain; this does not authorize a new build.
func (c Client) VerifyServiceStopped(ctx context.Context, instance, requestID string) (ServiceShutdown, error) {
	info, err := c.ServiceStatus(ctx)
	if err != nil {
		return ServiceShutdown{}, err
	}
	if err := c.verifyServiceStopped(info, instance, requestID); err != nil {
		return ServiceShutdown{}, err
	}
	return *info.LastShutdown, nil
}

// A selected compatible candidate can read the original service exit receipt.
// The receipt is still checked against its original executable, never the new one.
func (c Client) VerifyPreviousServiceStopped(ctx context.Context, sha, instance, requestID string) (ServiceShutdown, error) {
	info, err := c.ServiceStatus(ctx)
	if err != nil {
		return ServiceShutdown{}, err
	}
	original := c
	original.SHA256 = sha
	if err := original.verifyServiceStopped(info, instance, requestID); err != nil {
		return ServiceShutdown{}, err
	}
	return *info.LastShutdown, nil
}

func (c Client) verifyServiceStopped(info ServiceInfo, instance, requestID string) error {
	r := info.LastShutdown
	want, err := filepath.EvalSymlinks(c.StateDir)
	if err != nil {
		want = filepath.Clean(c.StateDir)
	}
	if info.Status != "not_running" || r == nil || instance == "" || requestID == "" || c.SHA256 == "" || r.InstanceID != instance || r.RequestID != requestID || !strings.EqualFold(filepath.Clean(r.StateDir), want) || !strings.EqualFold(r.ExecutableSHA256, c.SHA256) {
		return errors.New("original Runner resource settlement evidence is missing or identity differs")
	}
	return nil
}

func (c Client) ServiceStatus(ctx context.Context) (ServiceInfo, error) {
	return c.serviceInfo(ctx, "service status")
}
func (c Client) serviceInfo(ctx context.Context, command string) (ServiceInfo, error) {
	response, err := c.Call(ctx, command, nil, nil)
	if err != nil {
		return ServiceInfo{}, err
	}
	var info ServiceInfo
	if json.Unmarshal(response.Data, &info) != nil || info.Protocol != Protocol || info.Status == "" {
		return info, errors.New("Runner service identity is unavailable")
	}
	want, err := filepath.EvalSymlinks(c.StateDir)
	if err != nil {
		want = filepath.Clean(c.StateDir)
	}
	if !strings.EqualFold(filepath.Clean(info.StateDir), want) {
		return info, errors.New("Runner service belongs to another state directory")
	}
	return info, nil
}

// StartService is called only by the explicitly started SteerLine service.
// Queries use ServiceStatus, and never enter this startup path.
func (c Client) StartService(ctx context.Context, version Version) (ServiceInfo, error) {
	if len(c.UsagePolicy) > 0 {
		response, err := c.call(ctx, "service start", nil, nil, &runnercontract.ServiceStartInput{PendingStops: []runnercontract.PendingStop{}, UsagePolicy: c.UsagePolicy})
		var info ServiceInfo
		if err != nil {
			return info, err
		}
		if json.Unmarshal(response.Data, &info) != nil {
			return info, errors.New("Runner service identity is unavailable")
		}
		return info, c.validateStartedService(info, version)
	}
	info, err := c.serviceInfo(ctx, "service start")
	if err != nil {
		return info, err
	}
	return info, c.validateStartedService(info, version)
}

func (c Client) validateStartedService(info ServiceInfo, version Version) error {
	if info.Status != "ready" || info.InstanceID == "" || info.Protocol != Protocol || !strings.EqualFold(filepath.Clean(info.StateDir), filepath.Clean(c.StateDir)) || info.Version != version.Version || info.ResourceDigest != version.ResourceDigest || c.SHA256 == "" || !strings.EqualFold(info.ExecutableSHA256, c.SHA256) {
		return errors.New("Runner resident service differs from the selected release")
	}
	return nil
}

// StopService fixes one instance and request before invoking the CLI. A timeout
// never stops a replacement instance or kills a process by PID. The caller must
// retain requestID if it needs to reconcile an interrupted shutdown.
func (c Client) StopService(ctx context.Context, instance, requestID string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if instance == "" || requestID == "" {
		return errors.New("Runner shutdown requires its original instance and request ID")
	}
	body, _ := json.Marshal(map[string]string{"expected_instance_id": instance})
	request := Request{Protocol: Protocol, RequestID: requestID, Input: body}
	_, stopErr := c.Call(ctx, "service stop", nil, &request)
	for {
		if c.SHA256 != "" {
			if err := VerifyExecutable(c.Executable, c.SHA256); err != nil {
				return errors.Join(errors.New("Runner shutdown remains unverified; selected executable is unavailable"), stopErr, err)
			}
		}
		info, err := c.ServiceStatus(ctx)
		if err == nil && info.Status == "not_running" {
			return nil
		}
		if err == nil && info.InstanceID != "" && info.InstanceID != instance {
			return errors.New("Runner service changed during shutdown; replacement was not stopped")
		}
		if ctx.Err() != nil {
			return errors.Join(errors.New("Runner shutdown remains unverified"), stopErr, ctx.Err())
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}
