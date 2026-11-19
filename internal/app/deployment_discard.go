package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/pathguard"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const discardMarker = "deployment-discard.json"

func openDiscardTaskReader(ctx context.Context, c *Config) (*sqlite.SQLiteStore, error) {
	return openNativeTaskReader(ctx, c, sqlite.OpenSQLiteDiscardReader)
}

func validateDiscardApplicationPaths(c Config) error {
	if err := validateNativeStateLocation(c); err != nil {
		return err
	}
	return preflightNativeStatePathWithStore(c, func(ctx context.Context, _ host.Config) error {
		reader, err := openDiscardTaskReader(ctx, &c)
		if err != nil {
			return err
		}
		return reader.Close()
	})
}

// Kept outside application-api, which itself is archived. An interrupted swap
// must not be mistaken for an empty installation by start or another deployment.
type DeploymentDiscard struct {
	Archive         string                            `json:"archive"`
	Phase           string                            `json:"phase"`
	Stops           []runnercontract.PendingStop      `json:"stops"`
	ServiceStop     *runnercontract.Request           `json:"service_stop,omitempty"`
	Resources       *runnerclient.StoppedVerification `json:"resources,omitempty"`
	Projects        []DiscardedProject                `json:"projects"`
	Moves           []discardMove                     `json:"moves"`
	StopDiagnostic  string                            `json:"stop_diagnostic,omitempty"`
	InterruptedHost *contract.Identity                `json:"interrupted_host,omitempty"`
	UnstartedHost   *contract.Identity                `json:"unstarted_host,omitempty"`
}

type DiscardedProject struct {
	ConfigID     string `json:"config_id"`
	Tasks        int    `json:"tasks"`
	Dependencies int    `json:"dependencies"`
}

type discardMove struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Done   bool   `json:"done"`
}

func checkDeploymentDiscard(home string) error {
	if _, err := os.Lstat(filepath.Join(home, discardMarker)); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("an interrupted discard-runs deployment requires inspection of %s; startup and file replacement are blocked", filepath.Join(home, discardMarker))
}

// This is an explicitly destructive deployment goal. Ordinary update/replace
// never calls it. Native absence is still proved by the original Runner.
func prepareApplicationDiscard(ctx context.Context, runner, invocation string, verifyPackage func() error) (DeploymentPreparation, error) {
	result := DeploymentPreparation{Contract: DeploymentPreparationContract, Home: HomeDir(), Goal: DeploymentDiscardRuns, Outcome: "blocked",
		Facts: DeploymentFacts{"unverified", "unverified", "unverified", "not_applicable", "unverified"}}
	fail := func(err error) (DeploymentPreparation, error) { result.Diagnostic = err.Error(); return result, err }
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	release, err := localservice.Coordinate(ctx, result.Home)
	if err != nil {
		return fail(err)
	}
	defer release()
	if err := checkDeploymentDiscard(result.Home); err != nil {
		return fail(err)
	}
	if err := runnerstatepath.Validate(result.Home); err != nil {
		return fail(err)
	}
	data, err := os.ReadFile(runner)
	if err != nil {
		return fail(err)
	}
	hash := taskPayloadDigest(data)
	result.VerifierSHA256 = hash
	candidate := runnerclient.Client{Executable: runner, SHA256: hash, StateDir: runnerstatepath.Directory(result.Home)}
	if err := verifyRunnerUpgradeCapabilities(ctx, candidate); err != nil {
		return fail(err)
	}
	var pin runnercontract.BuildPin
	if err := localservice.ReadPrivateJSON(filepath.Join(result.Home, "application-api", "runner-build.json"), &pin, 64<<10); err != nil {
		return fail(err)
	}
	if !runnerbinding.SamePath(pin.StateDir, candidate.StateDir) || !runnerbinding.SamePath(pin.Executable, filepath.Join(result.Home, "bin", "steerrunner.exe")) {
		return fail(errors.New("discard-runs requires the current namespace and original installed Runner binding; no legacy migration"))
	}
	original := runnerclient.Client{Executable: pin.Executable, SHA256: pin.SHA256, StateDir: pin.StateDir, BuildVersion: pin.Version, ResourceDigest: pin.ResourceDigest}
	if err := runnerclient.VerifyExecutable(pin.Executable, pin.SHA256); err != nil {
		return fail(err)
	}
	archive := filepath.Join(result.Home, "maintenance", "discard-runs-"+invocation)
	if err := discardPlainPath(archive); err != nil {
		return fail(err)
	}
	if _, err := os.Lstat(archive); !os.IsNotExist(err) {
		return fail(errors.New("discard archive must be new; never replay a destructive operation with a different stop identity"))
	}
	// Preserve the failed receipt, not just its diagnostic string.
	stopCtx, stopCancel := context.WithTimeout(ctx, 120*time.Second)
	stopErr := stopAllCoordinated(stopCtx, result.Home)
	stopCancel()
	backend, web := components(result.Home)
	b, err := observeComponent(ctx, result.Home, backend)
	if err != nil || b.State != "exited" || b.Identity == nil {
		return fail(errors.Join(stopErr, err, errors.New("discard-runs requires verified original host exit")))
	}
	w, err := observeComponent(ctx, result.Home, web)
	if err != nil || w.State != "absent" && w.State != "exited" {
		return fail(errors.Join(err, errors.New("original Web exit is unverified")))
	}
	var shutdown contract.Shutdown
	err = localservice.ReadPrivateJSON(filepath.Join(backend.dir, "shutdown.json"), &shutdown, 1<<20)
	interrupted := os.IsNotExist(err)
	if err != nil && !interrupted {
		return fail(err)
	}
	unstarted := false
	if !interrupted {
		unstarted, err = recheckableDiscardShutdown(shutdown, *b.Identity, pin)
		if err != nil {
			return fail(err)
		}
		if !unstarted && (!runnerbinding.SamePath(shutdown.Runner.StateDir, pin.StateDir) ||
			shutdown.Runner.ExecutableSHA256 != pin.SHA256 || shutdown.Runner.ResourceDigest != pin.ResourceDigest || shutdown.Runner.Version != pin.Version) {
			return fail(errors.New("original shutdown Runner differs from the home binding"))
		}
	}
	catalog := loadApplicationCatalog(result.Home)
	if catalog.err != nil {
		return fail(catalog.err)
	}
	if !interrupted && len(catalog.entries) != len(shutdown.Projects) {
		return fail(errors.New("original project membership changed"))
	}
	byConfig := map[string]contract.ProjectShutdown{}
	for _, p := range shutdown.Projects {
		byConfig[p.ConfigID] = p
	}
	stateRoot := filepath.Join(result.Home, "state")
	var configs []Config
	stateNames := map[string]bool{}
	owners := map[string]*httpcontract.ProcessIdentity{}
	drainedBuilds := map[string]bool{}
	for _, entry := range catalog.entries {
		p, ok := byConfig[entry.view.ConfigID]
		if !ok && !interrupted {
			return fail(errors.New("original project membership changed"))
		}
		c, err := loadConfigSnapshotWithValidation(entry.ref.ConfigPath, entry.data, catalog.global, nil, false, nil, validateDiscardApplicationPaths)
		if err != nil {
			return fail(err)
		}
		if err := c.ResolveIdentity(ctx); err != nil {
			return fail(err)
		}
		if !interrupted && p.ProjectID != httpcontract.ProjectID(c.Identity()) {
			return fail(errors.New("original project configuration identity differs"))
		}
		if interrupted {
			p = contract.ProjectShutdown{ConfigID: entry.view.ConfigID, ProjectID: httpcontract.ProjectID(c.Identity())}
		}
		if !runnerbinding.SamePath(filepath.Dir(c.RunDir), stateRoot) {
			return fail(errors.New("project state is outside this installation"))
		}
		if err := discardPlainPath(c.RunDir); err != nil {
			return fail(err)
		}
		// Verify lock availability before issuing any new native stop.
		repoLock, err := host.LockRepository(c.Identity())
		if err != nil {
			return fail(err)
		}
		defer repoLock.Close()
		runLock, err := host.LockRepository(c.RunDir)
		if err != nil {
			return fail(err)
		}
		runLock.Close() // Directory rename cannot retain an open Windows lock file.
		reader, err := openDiscardTaskReader(ctx, &c)
		if err != nil {
			return fail(err)
		}
		snapshot, readErr := reader.ReadRunnerSnapshot(ctx, c.Identity())
		reader.Close()
		if readErr != nil {
			return fail(readErr)
		}
		if interrupted && snapshot.Checkpoint != nil {
			p.InstanceID = snapshot.Checkpoint.Service.InstanceID
		}
		if unstarted {
			cp := snapshot.Checkpoint
			if cp == nil || cp.Service.InstanceID == "" || cp.Service.ProcessIdentity == nil || cp.Service.RunnerBuild == nil {
				return fail(errors.New("previous project checkpoint ownership is missing"))
			}
			previous := cp.Service.RunnerBuild
			if sha, err := hex.DecodeString(previous.SHA256); err != nil || len(sha) != 32 || previous.Version == "" || previous.ResourceDigest == "" {
				return fail(errors.New("previous project Runner build is unverified"))
			}
			selected := c.RunnerExecution.Runner
			if !runnerbinding.SamePath(selected.Executable, pin.Executable) || selected.SHA256 != pin.SHA256 || selected.Version != pin.Version ||
				!runnerbinding.SamePath(previous.Executable, pin.Executable) || !runnerbinding.SamePath(previous.StateDir, pin.StateDir) ||
				previous.SHA256 == pin.SHA256 && !runnerbinding.SameBuild(*previous, pin) {
				return fail(errors.New("previous project or saved configuration Runner binding differs from the installation"))
			}
			process := cp.Service.ProcessIdentity
			owner := httpcontract.ProcessIdentity{Kind: process.Kind, PID: process.PID, Created: process.Created, Executable: process.Executable}
			owners[p.ConfigID] = &owner
			p.InstanceID = cp.Service.InstanceID
			// Only the in-memory verifier uses the checkpoint's original build.
			// The saved selection and the old database are never rewritten.
			c.RunnerExecution.Runner.SHA256 = previous.SHA256
			c.RunnerExecution.Runner.Version = previous.Version
		}
		if err := verifyDiscardCheckpoint(snapshot.Checkpoint, c, p, *b.Identity, interrupted, owners[p.ConfigID]); err != nil {
			return fail(fmt.Errorf("%s: %w", p.ConfigID, err))
		}
		if !runnerbinding.SameBuild(*snapshot.Checkpoint.Service.RunnerBuild, pin) {
			if !unstarted {
				return fail(errors.New("original project checkpoint Runner differs from the home binding"))
			}
			sha := snapshot.Checkpoint.Service.RunnerBuild.SHA256
			if !drainedBuilds[sha] {
				// A prior deployment may already have installed a new Runner.
				// Never start that build on undrained old Runs or controls.
				if err := original.VerifyInstalledUpgradeDrained(ctx, sha); err != nil {
					return fail(err)
				}
				drainedBuilds[sha] = true
			}
		}
		byConfig[p.ConfigID] = p
		configs = append(configs, c)
		stateNames[filepath.Base(c.RunDir)] = true
	}
	children, err := os.ReadDir(stateRoot)
	if err != nil && !os.IsNotExist(err) {
		return fail(err)
	}
	for _, child := range children {
		if !stateNames[child.Name()] {
			return fail(errors.New("unconfigured project state would be lost; discard-runs requires the complete original project census"))
		}
	}
	// All replacement task graphs are built and validated before native mutation.
	// No live service or task redispatch is needed to restore them.
	if err := os.MkdirAll(archive, 0700); err != nil {
		return fail(err)
	}
	journal := &DeploymentDiscard{Archive: archive, Phase: "preparing", Stops: []runnercontract.PendingStop{}, Projects: []DiscardedProject{}, Moves: []discardMove{}}
	result.Discard = journal
	if interrupted {
		journal.InterruptedHost = b.Identity
	}
	if unstarted {
		journal.UnstartedHost = b.Identity
	}
	if stopErr != nil {
		journal.StopDiagnostic = stopErr.Error()
	}
	if !interrupted {
		if err := localservice.WritePrivateFile(archive, "original-shutdown.json", shutdown); err != nil {
			return fail(err)
		}
	}
	prepared := filepath.Join(archive, "prepared-state")
	if err := os.Mkdir(prepared, 0700); err != nil {
		return fail(err)
	}
	for i, c := range configs {
		counts, err := prepareDiscardedTasks(ctx, c, filepath.Join(prepared, filepath.Base(c.RunDir)))
		if err != nil {
			return fail(fmt.Errorf("%s: %w", catalog.entries[i].view.ConfigID, err))
		}
		counts.ConfigID = catalog.entries[i].view.ConfigID
		journal.Projects = append(journal.Projects, counts)
	}
	persist := func() error { return localservice.WritePrivateFile(archive, "discard.json", journal) }
	if err := discardOriginalRuns(ctx, original, journal, persist); err != nil {
		return fail(fmt.Errorf("discard-runs Runner settlement (journal %s): %w", filepath.Join(archive, "discard.json"), err))
	}
	// Validate every caller-owned Run/input binding with the strict original
	// resource proof. Only report/business completion is waived for discarded work.
	proof := &ApplicationStopVerification{OriginalHost: *b.Identity, VerifierSHA256: hash, Runner: *journal.Resources, Projects: []ProjectStopVerification{}}
	for i, c := range configs {
		p := byConfig[catalog.entries[i].view.ConfigID]
		project, err := verifyDiscardProject(ctx, c, p, *b.Identity, *journal.Resources, original, interrupted, owners[p.ConfigID])
		if err != nil {
			return fail(fmt.Errorf("%s: %w", p.ConfigID, err))
		}
		proof.Projects = append(proof.Projects, project)
	}
	proof.VerifiedAt = time.Now().UTC()
	result.Stopped = proof
	if err := verifyPackage(); err != nil {
		return fail(err)
	}
	current := loadApplicationCatalog(result.Home)
	if !sameApplicationCatalog(catalog, current) {
		return fail(errors.New("project or execution configuration changed during discard preparation"))
	}
	journal.Moves = []discardMove{
		{Source: backend.dir, Target: filepath.Join(archive, "application-api")},
		{Source: pin.StateDir, Target: filepath.Join(archive, "runner-state")},
	}
	if _, err := os.Lstat(stateRoot); err == nil {
		journal.Moves = append([]discardMove{{Source: stateRoot, Target: filepath.Join(archive, "project-state")}}, journal.Moves...)
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	if _, err := os.Lstat(web.dir); err == nil {
		journal.Moves = append(journal.Moves, discardMove{Source: web.dir, Target: filepath.Join(archive, "web-state")})
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	journal.Moves = append(journal.Moves, discardMove{Source: prepared, Target: stateRoot})
	if err := commitDiscard(result.Home, journal); err != nil {
		return fail(err)
	}
	result.Outcome, result.Source, result.VerifiedAt = "prepared", "discarded_runs", time.Now().UTC()
	result.Facts = DeploymentFacts{"verified", "verified", "verified", "not_applicable", "verified"}
	return result, nil
}

func discardPlainPath(path string) error {
	resolved, err := pathguard.ResolveProspective(path)
	if err != nil {
		return err
	}
	if !runnerbinding.SamePath(resolved, path) {
		return fmt.Errorf("linked discard path: %s", path)
	}
	return nil
}

// A host that failed before opening any project cannot republish the previous
// projects' instance IDs. Its matching receipt proves only its own local exit;
// each old checkpoint owner and all native resources are independently checked.
func recheckableDiscardShutdown(r contract.Shutdown, id contract.Identity, pin runnercontract.BuildPin) (bool, error) {
	if err := recheckableShutdown(r, id); err == nil {
		return false, nil
	}
	if r.Version != 2 || r.Identity != id || r.Runner == nil || len(r.Projects) == 0 || len(r.Projects) > 512 {
		return false, errors.New("original host shutdown identity or membership is unverified")
	}
	seen := map[string]bool{}
	for _, p := range r.Projects {
		if p.ConfigID == "" || seen[p.ConfigID] || p.InstanceID != "" || p.ProjectID == "" || !p.LocalClosed || !p.LocksReleased {
			return false, errors.New("unstarted host project closure or membership is unverified")
		}
		seen[p.ConfigID] = true
	}
	runner := r.Runner
	if runner.StateDir != "" && !runnerbinding.SamePath(runner.StateDir, pin.StateDir) ||
		runner.ExecutableSHA256 != "" && runner.ExecutableSHA256 != pin.SHA256 ||
		runner.Version != "" && runner.Version != pin.Version || runner.ResourceDigest != "" && runner.ResourceDigest != pin.ResourceDigest ||
		runner.ExecutableSHA256 == "" && (runner.InstanceID != "" || runner.RequestID != "") {
		return false, errors.New("unstarted host Runner differs from the home binding")
	}
	return true, nil
}

func prepareDiscardedTasks(ctx context.Context, c Config, dir string) (counts DiscardedProject, retErr error) {
	reader, err := openDiscardTaskReader(ctx, &c)
	if err != nil {
		return counts, err
	}
	defer reader.Close()
	graph, err := reader.TaskGraphSnapshot(ctx, c.Identity())
	if err != nil {
		return counts, err
	}
	// Enforce the same repository/remote/branch binding as normal task commands.
	input := NewTaskCommands(reader, c).Input
	input.Repository, err = c.TaskRepositoryTarget(ctx)
	if err != nil {
		return counts, err
	}
	if _, err := (taskCommandStore{reader}).ReadGraph(ctx, input); err != nil {
		return counts, err
	}
	batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: c.Identity(), Tasks: []taskmodel.TaskBatchTask{}, Dependencies: []taskmodel.TaskBatchEdge{}}
	for _, task := range graph.Tasks {
		id, err := strconv.ParseInt(task.TaskID, 10, 64)
		if err != nil {
			return counts, err
		}
		contract, err := reader.TaskContract(ctx, c.Identity(), task)
		if err != nil {
			return counts, err
		}
		priority := task.Priority
		batch.Tasks = append(batch.Tasks, taskmodel.TaskBatchTask{LocalKey: task.TaskID, TaskID: id, Title: task.Title, Type: task.Type, Priority: &priority, GoalIDs: task.GoalIDs, Contract: contract})
	}
	for _, edge := range graph.Edges {
		batch.Dependencies = append(batch.Dependencies, taskmodel.TaskBatchEdge{Predecessor: taskmodel.TaskBatchReference{LocalKey: edge.PredecessorID}, Successor: taskmodel.TaskBatchReference{LocalKey: edge.SuccessorID}})
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return counts, err
	}
	fresh, err := sqlite.OpenSQLiteStoreWithAdmissionPolicy(filepath.Join(dir, "steerline.db"), c.environmentPolicy)
	if err != nil {
		return counts, err
	}
	defer func() { retErr = errors.Join(retErr, fresh.Close()) }()
	authority := graph.Authority
	authority.AuthorityVersion, authority.GraphRevision = 1, 0
	authority.DispatchMode = "manual"
	authority.CreatedAt, authority.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if err := fresh.InitializeProject(ctx, authority); err != nil {
		return counts, err
	}
	if len(batch.Tasks) != 0 {
		raw, err := json.Marshal(batch)
		if err != nil {
			return counts, err
		}
		commands := NewTaskCommands(fresh, c)
		valid, err := commands.ValidateTaskBatch(ctx, raw)
		if err != nil {
			return counts, err
		}
		if !valid.Valid {
			return counts, taskmodel.BatchValidationError(valid.Issues, valid.Cycle)
		}
		if _, err := commands.CreateTaskBatch(ctx, raw, productclient.NewRequestID(), valid.AuthorityVersion, "deployment-discard-runs"); err != nil {
			return counts, err
		}
	}
	return DiscardedProject{Tasks: len(batch.Tasks), Dependencies: len(batch.Dependencies)}, nil
}

type discardRunOperations struct {
	inventory   func(context.Context) (runnerclient.StoppedVerification, error)
	inspectCold func(context.Context, string) (runnerclient.Inspection, bool, error)
	start       func(context.Context, []runnercontract.PendingStop) (runnerclient.ServiceInfo, error)
	inspectStop func(context.Context, string) (runnerclient.Inspection, error)
	stopService func(context.Context, string, string) error
	verify      func(context.Context) (runnerclient.StoppedVerification, error)
}

func discardOriginalRuns(ctx context.Context, cli runnerclient.Client, journal *DeploymentDiscard, persist func() error) error {
	ops := discardRunOperations{
		inventory: cli.StoppedResourceInventory,
		inspectCold: func(ctx context.Context, id string) (runnerclient.Inspection, bool, error) {
			return cli.InspectBeforeStart(ctx, "run-id", id)
		},
		start: func(ctx context.Context, stops []runnercontract.PendingStop) (runnerclient.ServiceInfo, error) {
			version, err := cli.Version(ctx)
			if err != nil {
				return runnerclient.ServiceInfo{}, err
			}
			info, err := cli.StartServiceWithStops(ctx, version, stops)
			if err == nil {
				cli.InstanceID = info.InstanceID
			}
			return info, err
		},
		inspectStop: func(ctx context.Context, id string) (runnerclient.Inspection, error) {
			return cli.Inspect(ctx, "request-id", id)
		},
		stopService: cli.StopService,
		verify:      cli.VerifyStopped,
	}
	return discardRuns(ctx, ops, journal, persist)
}

func discardRuns(ctx context.Context, ops discardRunOperations, journal *DeploymentDiscard, persist func() error) error {
	inventory, err := ops.inventory(ctx)
	if err != nil {
		return fmt.Errorf("read original Runner's stopped inventory: %w", err)
	}
	for _, item := range inventory.Runs {
		view, cold, err := ops.inspectCold(ctx, item.RunID)
		if err != nil {
			return fmt.Errorf("inspect original Run %s before stopping: %w", item.RunID, err)
		}
		r := view.Run
		if !cold || r == nil || r.InputDigest != item.InputDigest {
			return errors.New("original offline Run/input binding differs")
		}
		if r.StopStatus == "stopped" {
			// A stopped Run can still have an accepted resume awaiting startup.
			// Do not reopen the service for other stops while that authority is
			// unresolved. A dormant CanResume is only an offer for a new request.
			if r.NewInputsAllowed || r.Controls.CanReply || r.Controls.CanMessage || r.Controls.CanInterrupt ||
				!r.Closed && r.ResumeRequired && !r.Controls.CanResume {
				return fmt.Errorf("Run %s has unresolved input/resume authority despite stopped resources; preserve its original control", item.RunID)
			}
			continue
		}
		if r.StopStatus == "pending" || r.StopStatus == "unknown" || !r.Controls.CanStop {
			return discardStopDiagnostic(fmt.Errorf("Run %s has an unresolved original stop/control; inspect its original request before retrying", item.RunID), r)
		}
		body, _ := json.Marshal(map[string]uint64{"expected_revision": r.Revision})
		journal.Stops = append(journal.Stops, runnercontract.PendingStop{RunID: item.RunID, Request: runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: productclient.NewRequestID(), Input: body}})
	}
	if err := persist(); err != nil {
		return err
	}
	if len(journal.Stops) != 0 {
		info, err := ops.start(ctx, journal.Stops)
		if err != nil {
			return fmt.Errorf("start original Runner with recorded stop requests: %w", err)
		}
		var waitErr error
		for _, stop := range journal.Stops {
			if waitErr = waitDiscardRunStop(ctx, ops.inspectStop, stop); waitErr != nil {
				break
			}
		}
		body, _ := json.Marshal(map[string]string{"expected_instance_id": info.InstanceID})
		journal.ServiceStop = &runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: "sl-service-stop-" + info.InstanceID, Input: body}
		if err := persist(); err != nil {
			return errors.Join(waitErr, err)
		}
		// The verified writer that we started still needs to exit if waiting for
		// its original stops fails. Closing it does not prove resource settlement.
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		stopErr := ops.stopService(stopCtx, info.InstanceID, journal.ServiceStop.RequestID)
		cancel()
		if stopErr != nil {
			stopErr = fmt.Errorf("close original Runner service %s: %w", info.InstanceID, stopErr)
		}
		if err := errors.Join(waitErr, stopErr, ctx.Err()); err != nil {
			return err
		}
	}
	proof, err := ops.verify(ctx)
	if err != nil {
		return fmt.Errorf("verify original Runner resources after writer exit: %w", err)
	}
	if len(proof.Runs) != len(inventory.Runs) {
		return errors.New("Runner inventory changed while discarding Runs")
	}
	want := map[string]string{}
	for _, run := range inventory.Runs {
		want[run.RunID] = run.InputDigest
	}
	for _, run := range proof.Runs {
		if want[run.RunID] != run.InputDigest {
			return errors.New("Runner inventory changed while discarding Runs")
		}
	}
	journal.Resources, journal.Phase = &proof, "settled"
	return persist()
}

func waitDiscardRunStop(ctx context.Context, inspect func(context.Context, string) (runnerclient.Inspection, error), stop runnercontract.PendingStop) error {
	var last *runnerclient.Run
	fail := func(err error) error {
		return discardStopDiagnostic(fmt.Errorf("Run %s stop request %s: %w", stop.RunID, stop.Request.RequestID, err), last)
	}
	for {
		view, err := inspect(ctx, stop.Request.RequestID)
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		if err != nil {
			return fail(fmt.Errorf("inspect original stop receipt: %w", err))
		}
		if view.Run == nil || view.Run.RunID != stop.RunID || view.Command == nil || view.Command.RequestID != stop.Request.RequestID || view.Command.Command != "stop" {
			return fail(errors.New("original stop receipt binding differs"))
		}
		last = view.Run
		switch view.Command.ActionStatus {
		case "failed", "rejected":
			if view.Command.Error != nil {
				return fail(view.Command.Error)
			}
			return fail(fmt.Errorf("original stop action %s", view.Command.ActionStatus))
		case "completed":
			if last.StopStatus == "stopped" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func discardStopDiagnostic(err error, run *runnerclient.Run) error {
	if run == nil {
		return err
	}
	for _, op := range run.Operations {
		if op.Status == "outcome_unknown" && op.Diagnostic != "" {
			return fmt.Errorf("%w (stop_status=%q, original %s operation %s: %s)", err, run.StopStatus, op.Action, op.ID, op.Diagnostic)
		}
	}
	if run.Diagnostic != "" {
		return fmt.Errorf("%w (stop_status=%q: %s)", err, run.StopStatus, run.Diagnostic)
	}
	return fmt.Errorf("%w (stop_status=%q)", err, run.StopStatus)
}

func commitDiscard(home string, journal *DeploymentDiscard) error {
	archive := journal.Archive
	if !pathguard.Within(filepath.Join(home, "maintenance"), archive) || runnerbinding.SamePath(filepath.Join(home, "maintenance"), archive) {
		return errors.New("discard archive escaped the installation")
	}
	// Validate all moves before the first rename. Move directories as entries;
	// never traverse or delete their contents, including archived Git workspaces.
	allowed := map[string]string{
		canonicalPath(filepath.Join(home, "state")):             filepath.Join(archive, "project-state"),
		canonicalPath(filepath.Join(home, "application-api")):   filepath.Join(archive, "application-api"),
		canonicalPath(filepath.Join(home, "ui", "web")):         filepath.Join(archive, "web-state"),
		canonicalPath(runnerstatepath.Directory(home)):          filepath.Join(archive, "runner-state"),
		canonicalPath(filepath.Join(archive, "prepared-state")): filepath.Join(home, "state"),
	}
	if journal.Resources == nil || !journal.Resources.ResourcesSettled {
		return errors.New("discard commit requires original native resource settlement")
	}
	for _, move := range journal.Moves {
		target, ok := allowed[canonicalPath(move.Source)]
		if !ok || !runnerbinding.SamePath(target, move.Target) {
			return errors.New("discard move escaped its fixed installation boundaries")
		}
		if err := discardPlainPath(move.Source); err != nil {
			return err
		}
		if err := discardPlainPath(move.Target); err != nil {
			return err
		}
		if _, err := os.Lstat(move.Source); err != nil {
			return err
		}
		if _, err := os.Lstat(move.Target); err == nil {
			// The replacement state target is vacated by the first move.
			if !runnerbinding.SamePath(move.Target, filepath.Join(home, "state")) {
				return errors.New("discard archive target already exists")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	journal.Phase = "committing"
	if err := localservice.WritePrivateFile(home, discardMarker, journal); err != nil {
		return err
	}
	for i := range journal.Moves {
		move := &journal.Moves[i]
		if err := os.Rename(move.Source, move.Target); err != nil {
			return err
		}
		move.Done = true
		if err := localservice.WritePrivateFile(home, discardMarker, journal); err != nil {
			return err
		}
	}
	journal.Phase = "complete"
	if err := localservice.WritePrivateFile(archive, "discard.json", journal); err != nil {
		return err
	}
	return os.Remove(filepath.Join(home, discardMarker))
}
