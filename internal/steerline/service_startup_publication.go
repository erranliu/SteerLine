package steerline

import (
	"errors"
	"fmt"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (s *ServiceHost) PublishReady(instanceID string, processIdentity *executionbackend.ProcessIdentity, dispatchMode string) error {
	if s.startupID == "" {
		return nil
	}
	record, err := readStartupRecord(s.Config, s.startupID)
	if err != nil {
		return fmt.Errorf("read startup attempt before readiness publication: %w", err)
	}
	if record.Status == "started" && record.InstanceID == instanceID {
		return nil
	}
	if record.Status != "starting" {
		return errors.New("startup attempt is no longer pending")
	}
	record.Status = "started"
	record.RepositoryIdentity = s.Config.Identity()
	record.RepositoryPath = s.Config.Repository
	record.RunDir = s.Config.RunDir
	record.Remote = s.Config.Remote
	record.Branch = s.Config.Branch
	record.Claimant = s.Config.Claimant
	record.ConfigDigest = s.Config.LoadedConfigDigest()
	record.InstanceID = instanceID
	record.ProcessIdentity = processIdentity
	if err = writeStartupRecord(s.Config, *record); err != nil {
		return fmt.Errorf("publish startup readiness record: %w", err)
	}
	s.Emit(ActivityEvent{
		Source: "steer", Kind: "service", Status: "ready",
		Title: "service ready", Detail: "dispatch " + dispatchMode,
	})
	return nil
}
