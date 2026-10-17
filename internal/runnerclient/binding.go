package runnerclient

import (
	"context"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
)

func (c Client) BuildPin() runnercontract.BuildPin {
	return runnercontract.BuildPin{Executable: c.Executable, Version: c.BuildVersion, SHA256: strings.ToLower(c.SHA256), ResourceDigest: c.ResourceDigest, StateDir: c.StateDir}
}

// ValidateInstance observes the original binding, never adopts a replacement.
func (c Client) ValidateInstance(ctx context.Context) error {
	if c.InstanceID == "" {
		return nil
	}
	info, err := c.ServiceStatus(ctx)
	if err != nil {
		return err
	}
	if info.Status != "ready" || info.InstanceID != c.InstanceID || !strings.EqualFold(info.ExecutableSHA256, c.SHA256) || info.ResourceDigest != c.ResourceDigest {
		return &Failure{Code: "runner_instance_changed", Message: "original Runner instance is unavailable; restart SteerLine explicitly to rebuild ProjectRuntime", Exit: 5}
	}
	return nil
}
