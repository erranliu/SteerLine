package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/host"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func fixtureAtomicWrite(path string, body []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".fixture-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

type fixtureResources struct{}

func (fixtureResources) WorkerTreeAbsent(id executionbackend.ProcessIdentity) bool {
	return resourceprocess.WorkerTreeAbsent(id)
}
func (fixtureResources) AtomicWrite(path string, value []byte) error {
	return fixtureAtomicWrite(path, value)
}

func fixtureStateKey(identity string) string {
	common := identity
	if value, err := filepath.EvalSymlinks(common); err == nil {
		common = value
	}
	name := filepath.Base(common)
	if strings.EqualFold(name, ".git") {
		name = filepath.Base(filepath.Dir(common))
	}
	name = strings.Trim(regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(name, "-"), "-.")
	if name == "" {
		name = "repo"
	}
	return name + "-" + shortHash(canonical(common))[:12]
}
func openFixtureProject(ctx context.Context, c *Config) (*sqlite.SQLiteStore, error) {
	*c = testConfigWithDirectory(*c)
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	target, err := c.TaskRepositoryTarget(ctx)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(c.RunDir, 0700); err != nil {
		return nil, err
	}
	db, err := sqlite.OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return nil, err
	}
	authority, err := db.NativeAuthority(ctx, c.Identity())
	if errors.Is(err, storageports.ErrNotFound) {
		authority = executionmodel.ProjectTaskAuthority{ProjectIdentity: c.Identity(), Authority: "native", AuthorityVersion: 1, Repository: target, Remote: c.Remote, Branch: c.Branch, DispatchMode: "manual", DispatchPaused: true, CreatedAt: now(), UpdatedAt: now()}
		if err = db.InitializeProject(ctx, authority); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	db.BindAuthority(authority)
	return db, nil
}
func openFixtureCommandStore(ctx context.Context, c *Config) (*sqlite.SQLiteStore, error) {
	return openFixtureProject(ctx, c)
}
func loadFixtureRuntime(c Config) (*contracts.Runtime, error) {
	db, err := sqlite.OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	snapshot, err := db.ReadExecutionStateSnapshot(context.Background(), c.Identity())
	if err != nil {
		return nil, err
	}
	if snapshot.Checkpoint == nil {
		return nil, storageports.ErrNotFound
	}
	runtime, err := reconstruction.BuildFromStorage(snapshot, *snapshot.Checkpoint)
	if err != nil {
		return nil, err
	}
	return &runtime, nil
}
func checkFixtureStoredExecutions(ctx context.Context, c Config) error {
	db, err := sqlite.OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	snapshot, err := db.ReadExecutionStateSnapshot(ctx, c.Identity())
	if err != nil {
		return err
	}
	return c.ValidateStoredExecutions(reconstruction.SnapshotFromStorage(snapshot))
}
func newFixtureTasks(reader *sqlite.SQLiteStore, c Config) taskapp.Service {
	commands := NewTaskCommands(reader, c)
	return taskapp.Service{Reader: reader, Input: commands.Input, Documents: commands.Documents}
}

// The fixture owns only physical OS/file transport. The production host loop
// drives the same execution actor; this creates no alternate business writer.
type fixtureHost struct{ config Config }

func (h fixtureHost) ValidateStateLocation(c Config) error {
	if c.StateRoot == "" || c.RepositoryStateKey == "" || c.RepositoryStateKey != fixtureStateKey(c.Identity()) || canonical(c.RunDir) != canonical(filepath.Join(c.StateRoot, c.RepositoryStateKey)) {
		return errors.New("fixture state identity differs")
	}
	return nil
}
func (h fixtureHost) ControlRequestIDs() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(h.config.RunDir, "control", "requests"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() && controlIDPattern.MatchString(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}
func controlRequestPath(dir, id string) string {
	return filepath.Join(dir, "control", "requests", id, "request.json")
}
func readControlRequest(path string) (productapi.ControlRequest, error) {
	body, err := os.ReadFile(path)
	var request productapi.ControlRequest
	if err != nil {
		return request, err
	}
	if len(body) > 3<<20 {
		return request, errors.New("oversize fixture request")
	}
	return request, contracts.DecodeJSON(body, &request)
}
func (h fixtureHost) ReadControlRequest(id string) (productapi.ControlRequest, error) {
	return readControlRequest(controlRequestPath(h.config.RunDir, id))
}
func rawRequestDigest(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(body) > 3<<20 {
		body = body[:(3<<20)+1]
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func (h fixtureHost) ControlRequestDigest(id string) string {
	return rawRequestDigest(controlRequestPath(h.config.RunDir, id))
}
func ReadControlReceipt(c Config, id string) (*productapi.ControlReceipt, error) {
	body, err := os.ReadFile(filepath.Join(c.RunDir, "control", "receipts", id+".json"))
	if err != nil {
		return nil, err
	}
	var value productapi.ControlReceipt
	if err = contracts.DecodeJSON(body, &value); err != nil {
		return nil, err
	}
	return &value, nil
}
func (h fixtureHost) ReadControlReceipt(id string) (*productapi.ControlReceipt, error) {
	return ReadControlReceipt(h.config, id)
}
func (h fixtureHost) WriteControlReceipt(value productapi.ControlReceipt) error {
	path := filepath.Join(h.config.RunDir, "control", "receipts", value.RequestID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return fixtureAtomicWrite(path, append(body, '\n'))
}
func publishControlRequest(dir string, value productapi.ControlRequest) error {
	path := controlRequestPath(dir, value.RequestID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if old, err := readControlRequest(path); err == nil {
		if RequestDigest(old) != RequestDigest(value) {
			return errors.New("fixture request identity conflict")
		}
		return nil
	}
	return fixtureAtomicWrite(path, append(body, '\n'))
}
func (h fixtureHost) FindStopRequest(instance string, seen map[string]bool) (*productapi.ControlRequest, string, time.Time) {
	ids, _ := h.ControlRequestIDs()
	for _, id := range ids {
		if seen[id] {
			continue
		}
		req, err := h.ReadControlRequest(id)
		if err != nil {
			continue
		}
		if seen != nil {
			seen[id] = true
		}
		if req.Action != "stop_service" || req.RequestID != id || req.InstanceID != instance {
			continue
		}
		checked := now()
		if _, err = DecodeControlPayload(req); err != nil {
			continue
		}
		if _, err = ValidateControlRequest(req, instance, h.config.Identity(), 0, checked); err != nil {
			continue
		}
		if _, err = h.ReadControlReceipt(id); err == nil {
			continue
		}
		return &req, h.ControlRequestDigest(id), checked
	}
	return nil, "", time.Time{}
}
func (fixtureHost) PublishReady(string, *executionbackend.ProcessIdentity, string) error { return nil }
func (fixtureHost) Emit(contracts.ActivityEvent)                                         {}
func (fixtureHost) CurrentProcessIdentity() (*executionbackend.ProcessIdentity, error) {
	return resourceprocess.CurrentIdentity()
}
func runTestService(ctx context.Context, s *Service, mode string) error {
	return host.Run(ctx, s, host.Schedule{TaskPoll: duration(s.Config.TaskPoll), PRPoll: duration(s.Config.PRPoll), Heartbeat: duration(s.Config.Heartbeat), FindStop: s.host.FindStopRequest}, mode)
}

var _ HostPort = fixtureHost{}

func newFixtureConversationQuery(store storageports.ConversationReader, project string) query.ConversationService {
	return query.ConversationService{Project: project, Events: store, Bodies: store, Raw: store, Capabilities: store}
}
func taskDependencyState(record taskmodel.TaskRecord, graph storageports.TaskGraphSnapshot) (bool, []taskmodel.TaskDependencyStatus) {
	return taskmodel.DependencyStateForTask(record, graph.TaskGraph())
}
