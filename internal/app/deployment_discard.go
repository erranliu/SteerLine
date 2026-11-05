package app

import (
	"context"
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
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const discardMarker = "deployment-discard.json"

// Kept outside application-api, which itself is archived. An interrupted swap
// must not be mistaken for an empty installation by start or another deployment.
type DeploymentDiscard struct {
	Archive        string                            `json:"archive"`
	Phase          string                            `json:"phase"`
	Stops          []runnercontract.PendingStop      `json:"stops"`
	ServiceStop    *runnercontract.Request           `json:"service_stop,omitempty"`
	Resources      *runnerclient.StoppedVerification `json:"resources,omitempty"`
	Projects       []DiscardedProject                `json:"projects"`
	Moves          []discardMove                     `json:"moves"`
	StopDiagnostic string                            `json:"stop_diagnostic,omitempty"`
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
	if err := localservice.ReadPrivateJSON(filepath.Join(backend.dir, "shutdown.json"), &shutdown, 1<<20); err != nil {
		return fail(err)
	}
	if err := recheckableShutdown(shutdown, *b.Identity); err != nil {
		return fail(err)
	}
	if shutdown.Runner == nil || !runnerbinding.SamePath(shutdown.Runner.StateDir, pin.StateDir) ||
		shutdown.Runner.ExecutableSHA256 != pin.SHA256 || shutdown.Runner.ResourceDigest != pin.ResourceDigest || shutdown.Runner.Version != pin.Version {
		return fail(errors.New("original shutdown Runner differs from the home binding"))
	}
	catalog := loadApplicationCatalog(result.Home)
	if catalog.err != nil {
		return fail(catalog.err)
	}
	if len(catalog.entries) != len(shutdown.Projects) {
		return fail(errors.New("original project membership changed"))
	}
	byConfig := map[string]contract.ProjectShutdown{}
	for _, p := range shutdown.Projects {
		byConfig[p.ConfigID] = p
	}
	stateRoot := filepath.Join(result.Home, "state")
	var configs []Config
	stateNames := map[string]bool{}
	for _, entry := range catalog.entries {
		p, ok := byConfig[entry.view.ConfigID]
		if !ok {
			return fail(errors.New("original project membership changed"))
		}
		c, err := loadConfigSnapshot(entry.ref.ConfigPath, entry.data, catalog.global, nil, false)
		if err != nil {
			return fail(err)
		}
		if err := c.ResolveIdentity(ctx); err != nil {
			return fail(err)
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
		reader, err := OpenNativeTaskReader(ctx, &c)
		if err != nil {
			return fail(err)
		}
		snapshot, readErr := reader.ReadRunnerSnapshot(ctx, c.Identity())
		reader.Close()
		if readErr != nil {
			return fail(readErr)
		}
		if err := verifyStoppedCheckpoint(snapshot.Checkpoint, c, p, *b.Identity); err != nil {
			return fail(fmt.Errorf("%s: %w", p.ConfigID, err))
		}
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
	if stopErr != nil {
		journal.StopDiagnostic = stopErr.Error()
	}
	if err := localservice.WritePrivateFile(archive, "original-shutdown.json", shutdown); err != nil {
		return fail(err)
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
		return fail(err)
	}
	// Validate every caller-owned Run/input binding with the strict original
	// resource proof. Only report/business completion is waived for discarded work.
	for i, c := range configs {
		if _, err := verifyDiscardProject(ctx, c, byConfig[catalog.entries[i].view.ConfigID], *b.Identity, *journal.Resources); err != nil {
			return fail(err)
		}
	}
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

func prepareDiscardedTasks(ctx context.Context, c Config, dir string) (counts DiscardedProject, retErr error) {
	reader, err := OpenNativeTaskReader(ctx, &c)
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
	authority.DispatchMode, authority.DispatchPaused = "manual", true
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
		return err
	}
	for _, item := range inventory.Runs {
		view, cold, err := ops.inspectCold(ctx, item.RunID)
		if err != nil {
			return err
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
		if !r.Controls.CanStop {
			return fmt.Errorf("Run %s has an unresolved original stop/control; inspect its original request before retrying", item.RunID)
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
			return err
		}
		for _, stop := range journal.Stops {
			for {
				view, err := ops.inspectStop(ctx, stop.Request.RequestID)
				if err != nil {
					return err
				}
				if view.Run == nil || view.Run.RunID != stop.RunID || view.Command == nil || view.Command.RequestID != stop.Request.RequestID || view.Command.Command != "stop" {
					return errors.New("original stop receipt binding differs")
				}
				if view.Command.ActionStatus == "failed" || view.Command.ActionStatus == "rejected" {
					return errors.New("original Run stop failed")
				}
				if view.Command.ActionStatus == "completed" && view.Run.StopStatus == "stopped" {
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(200 * time.Millisecond):
				}
			}
		}
		body, _ := json.Marshal(map[string]string{"expected_instance_id": info.InstanceID})
		journal.ServiceStop = &runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: "sl-service-stop-" + info.InstanceID, Input: body}
		if err := persist(); err != nil {
			return err
		}
		if err := ops.stopService(ctx, info.InstanceID, journal.ServiceStop.RequestID); err != nil {
			return err
		}
	}
	proof, err := ops.verify(ctx)
	if err != nil {
		return err
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
