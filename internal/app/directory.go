package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/publication"
)

const executionDirectoryVersion = "builtin-execution-v7"

// NewBuiltinDirectory constructs independent catalogs. The instance is bound
// once, before any registration callback can run.
func NewBuiltinDirectory(processHost executionbackend.ManagedProcessHost) (*execution.ExecutionDirectory, error) {
	if processHost == nil {
		return nil, errors.New("managed process host is required")
	}
	var directory *execution.ExecutionDirectory
	bound := func() *execution.ExecutionDirectory { return directory }
	localCodec, local := localExecutionRegistration(processHost, bound)
	terminalCodec, terminal := terminalExecutionRegistration(processHost, bound)
	var err error
	directory, err = execution.NewExecutionDirectory(executionDirectoryVersion,
		[]execution.ImplementationProfileCodec{localCodec, terminalCodec},
		[]execution.ExecutionRegistration{local, terminal})
	return directory, err
}

func validateWorkerPublicationPolicy(policy executionbackend.PublicationPolicy, provider publication.Config) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if err := provider.ValidateRemote(strings.TrimSuffix(provider.BaseURL, "/") + "/" + provider.Repository + ".git"); err != nil {
		return errors.New("publication provider requires a valid kind/base_url/repository")
	}
	queryAvailable := provider.Kind == "gitea" || provider.Kind == "command" && provider.Query.Executable != ""
	if policy.Publisher == executionbackend.PublisherWorker && queryAvailable {
		return nil
	}
	return executionbackend.UnsupportedAdapterOperation("PublicationPolicy", "publication_policy is unsupported by the selected implementation/provider")
}

func shortDigest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func positiveDuration(value string, fallback time.Duration) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func validateCatalogRef(ref executionbackend.ExecutionRef) error {
	switch ref.Protocol {
	case executionbackend.ProtocolLocalCodexAppServer, executionbackend.ProtocolLocalProcess:
		if ref.Backend != executionbackend.BackendLocal || ref.External.Kind != "os" || ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "" {
			return errors.New("local resource lacks its fixed process identity")
		}
	case executionbackend.ProtocolOrcaTerminalV3:
		if ref.Backend != executionbackend.BackendOrca || ref.External.Kind != "orca_terminal" || ref.External.Terminal == "" || ref.External.RuntimeID == "" || ref.External.Incarnation == "" || ref.External.WorktreeID == "" || ref.External.ExecutionHost != ref.Target.ExecutionHost {
			return errors.New("terminal resource lacks its exact original binding")
		}

	default:
		return errors.New("unsupported resource protocol")
	}
	return nil
}
