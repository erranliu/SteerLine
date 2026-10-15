package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

type ServiceInfo struct {
	InstanceID       string `json:"instance_id"`
	StateDir         string `json:"state_dir"`
	Status           string `json:"status"`
	Version          string `json:"version"`
	Protocol         string `json:"protocol"`
	ResourceDigest   string `json:"resource_digest"`
	ExecutableSHA256 string `json:"executable_sha256"`
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
	info, err := c.serviceInfo(ctx, "service start")
	if err != nil {
		return info, err
	}
	if info.Status != "ready" || info.InstanceID == "" || info.Version != version.Version || info.ResourceDigest != version.ResourceDigest || c.SHA256 == "" || !strings.EqualFold(info.ExecutableSHA256, c.SHA256) {
		return info, errors.New("Runner resident service differs from the selected release")
	}
	return info, nil
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
