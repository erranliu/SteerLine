package orca_remote_worker

import (
	"errors"
	"strings"
	"time"
)

// Selection validates static options before any host probe or resource creation.
type Selection struct {
	Agent, Model, ReasoningEffort, Executable, Shell, WaitTimeout string
	Settings                                                      *SelectionSettings
}
type SelectionSettings struct {
	Settings
	RemoteCancelAvailable   bool
	CapabilityReportVersion int
}

func (c Selection) Validate() error {
	if c.Agent != "codex" {
		return errors.New("orca_supervised currently requires agent=codex")
	}
	if c.Model == "" || c.ReasoningEffort == "" {
		return errors.New("orca_supervised requires explicit model and reasoning_effort; the remote Orca default is never inherited")
	}
	if c.Settings == nil {
		return errors.New("orca_supervised requires an explicit orca.supervised configuration")
	}
	supervised := c.Settings
	if strings.TrimSpace(supervised.ConnectionID) == "" || strings.TrimSpace(supervised.EnvironmentID) == "" || strings.TrimSpace(supervised.RepositorySelector) == "" || strings.TrimSpace(supervised.CoordinatorTerminal) == "" {
		return errors.New("orca.supervised requires fixed connection_id, environment_id, repository_selector, and coordinator_terminal")
	}
	if supervised.RemoteCancelAvailable {
		return errors.New("orca_supervised remote cancellation is fixed unavailable")
	}
	if strings.TrimSpace(supervised.PollInterval) == "" {
		return errors.New("orca.supervised.poll_interval must be explicit")
	}
	if d, e := time.ParseDuration(supervised.PollInterval); e != nil || d <= 0 {
		return errors.New("orca.supervised.poll_interval must be a positive duration")
	}
	if supervised.CapabilityReportVersion != 1 {
		return errors.New("orca.supervised.capability_report_version must explicitly pin version 1")
	}
	if c.Executable == "" || c.Shell == "" {
		return errors.New("orca.executable and orca.shell are required")
	}
	if d, e := time.ParseDuration(c.WaitTimeout); e != nil || d <= 0 {
		return errors.New("orca.wait_timeout must be a positive duration")
	}
	return nil
}
