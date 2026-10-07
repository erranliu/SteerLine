package host

import (
	"context"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// Config contains only the values the process host needs after app composition.
// Application storage, adapters, task readers, and actor construction stay in app.
type Config struct {
	Repository         string
	RepositoryIdentity string
	Remote             string
	Branch             string
	RunDir             string
	Claimant           string
	ConfigDigest       string
	TaskAuthority      string
	WorkflowProtocol   string
	TaskPoll           string
	PRPoll             string
	Heartbeat          string
	CommandTimeout     string
	ShutdownGrace      string
	ProcessFacts       executionbackend.ProcessFactsPort

	RuntimeReader           func(context.Context, Config) (*reconstruction.Runtime, error)
	ValidateNativeStore     func(context.Context, Config) error
	EnsureRunDirOutsideRepo func(context.Context, Config) error
	ControlValidation       ControlValidationPort
}

func (c Config) Identity() string           { return c.RepositoryIdentity }
func (c Config) LoadedConfigDigest() string { return c.ConfigDigest }

func (c Config) RequireNativeTaskAuthority() error {
	if c.TaskAuthority != "native" {
		return nativeErr("authority_mismatch", "task_authority must be explicitly set to native")
	}
	return nil
}

func (c Config) ValidateTaskCommandConfiguration() error {
	if c.Repository == "" || c.Remote == "" || c.Branch == "" {
		return nativeErr("authority_mismatch", "repository, remote and branch are required")
	}
	return nil
}

func (c Config) ResolveIdentity(context.Context) error {
	if c.RepositoryIdentity == "" {
		return nativeErr("authority_mismatch", "resolved repository identity is required")
	}
	return nil
}

// ControlValidationPort lets the host reject malformed wire requests while
// execution remains the owner of action-specific payload semantics.
type ControlValidationPort interface {
	ValidAction(string) bool
	DecodePayload(productapi.ControlRequest) (productapi.ControlPayload, error)
	ValidateRequest(productapi.ControlRequest, string, string, uint64, time.Time) (string, error)
	RequestDigest(productapi.ControlRequest) string
	TerminalStatus(string) bool
}
