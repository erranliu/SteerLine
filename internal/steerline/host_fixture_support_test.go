package steerline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const workerFlowMockSession = "00000000-0000-0000-0000-000000000008"

func workspacePolicyFixture() executionbackend.ExecutionPolicy {
	return executionbackend.ExecutionPolicy{Filesystem: executionbackend.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
}
func executionPublicationFixture() executionbackend.PublicationPolicy {
	return executionbackend.PublicationPolicy{Publisher: "worker"}
}
func hostPublicationProviderFixture() publication.Config {
	return publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: executionbackend.Command{Executable: os.Args[0], Args: []string{"api", "repos/{repository}/pulls/{number}"}}, Close: executionbackend.Command{Executable: os.Args[0], Args: []string{"pr", "close", "{pr_url}"}}}
}
func fullFixtureTaskService(reader persistence.CommandStore, c execution.Config) taskapp.Service {
	commands := NewTaskCommands(reader, c)
	return taskapp.Service{Reader: reader, Input: commands.Input, Documents: commands.Documents}
}
func putFixtureImplementationEnvelope(record *executionmodel.ExecutionRecord, envelope executionbackend.ImplementationDataEnvelope) error {
	if record == nil {
		return errors.New("record required")
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	var values []executionbackend.ImplementationDataEnvelope
	for _, value := range record.ImplementationData {
		if value.ImplementationKey != envelope.ImplementationKey {
			values = append(values, value.Clone())
		}
	}
	record.ImplementationData = append(values, envelope.Clone())
	return nil
}
func fixtureStoreFor(t *testing.T, service *execution.Service) *sqliteFixture {
	store, err := openSQLiteFixture(filepath.Join(service.Config.RunDir, "steerline.db"))
	if err != nil {
		panic(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
func publishFixtureCheckpoint(service *execution.Service) error {
	db, err := service.Config.Persistence.OpenWriter(filepath.Join(service.Config.RunDir, "steerline.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	checkpoint, err := db.SaveCheckpoint(context.Background(), reconstruction.Checkpoint(service.State))
	if err != nil {
		return err
	}
	service.State.Revision = checkpoint.Revision
	service.State.UpdatedAt = checkpoint.UpdatedAt
	service.State.HeartbeatAt = checkpoint.HeartbeatAt
	return nil
}
func controlReviewService(t *testing.T) *execution.Service {
	t.Helper()
	service := newFixture(t).service(t)
	service.Config.BindApplicationConfig(service.Config.ExecutionDirectory, "", "", "lifecycle-review-config", false)
	service.State.Service.Mode = "scheduler"
	service.State.Service.Status = "running"
	service.State.Service.Ready = true
	identity, err := resourceprocess.CurrentIdentity()
	if err != nil {
		t.Fatal(err)
	}
	startupID := newID()
	if _, _, err := createStartupAttempt(service.Config, startupID, true); err != nil {
		t.Fatal(err)
	}
	record := initialStartupRecord(service.Config, startupID)
	record.Status = "started"
	record.InstanceID = service.State.Service.InstanceID
	record.ProcessIdentity = identity
	if err := writeStartupRecord(service.Config, record); err != nil {
		t.Fatal(err)
	}
	service.State.Service.StartupID = &startupID
	service.State.Service.PID = identity.PID
	service.State.Service.ProcessIdentity = identity
	service.State.Service.ConfigDigest = service.Config.LoadedConfigDigest()
	if err := publishFixtureCheckpoint(service); err != nil {
		t.Fatal(err)
	}
	return service
}

func newHostFixtureService(c execution.Config, host execution.HostPort) (*execution.Service, error) {
	c = testConfigWithDirectory(c)
	var err error
	c, err = PrepareServiceConfig(c)
	if err != nil {
		return nil, err
	}
	db, err := OpenNativeTaskCommandStore(context.Background(), &c)
	if err != nil {
		return nil, err
	}
	tasks := fullFixtureTaskService(db, c)
	return execution.NewService(c, c.ExecutionDirectory, c.ExecutionDirectory, neutralConversationSources{}, c.ProcessHost, db, tasks, tasks, host)
}
func isNativeErrorCode(err error, codes ...string) bool {
	var value *taskmodel.Error
	if errors.As(err, &value) {
		for _, code := range codes {
			if value.Code == code {
				return true
			}
		}
	}
	return false
}
func findPublishedRun(service *execution.Service, id string) *contracts.Run {
	for _, run := range service.State.Runs {
		if run.RunID == id || run.TaskID == id {
			return run
		}
	}
	return nil
}

var _ = strings.TrimSpace
var _ = time.Second

func sameFixtureRepositoryTarget(a, b string) bool { return a == b }
