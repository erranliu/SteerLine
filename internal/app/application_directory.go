package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/applicationhttp"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/projectconnection"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

type catalogEntry struct {
	*catalogProject
	port        *runtimeProjectPort
	result      contract.ProjectShutdown
	resultReady bool
	runtime     *ProjectRuntime
}

// ApplicationDirectory owns the frozen configuration collection and independent
// project runtimes. Catalog and Connect never initialize storage.
type ApplicationDirectory struct {
	home              string
	mu                sync.RWMutex
	entries           []*catalogEntry
	global            []byte
	err               error
	ctx               context.Context
	cancel            context.CancelFunc
	done              chan struct{}
	started, stopping bool
	doneOnce          sync.Once
	runner            applicationRunner
}

func NewApplicationDirectory(home string) *ApplicationDirectory {
	ctx, cancel := context.WithCancel(context.Background())
	catalog := loadApplicationCatalog(home)
	d := &ApplicationDirectory{home: home, ctx: ctx, cancel: cancel, done: make(chan struct{}), global: catalog.global, err: catalog.err}
	if _, err := os.Lstat(runnerstatepath.Directory(home)); !os.IsNotExist(err) {
		d.runner.attempted = true
		d.runner.client.StateDir = runnerstatepath.Directory(home)
	}
	for _, p := range catalog.entries {
		d.entries = append(d.entries, &catalogEntry{catalogProject: p, resultReady: true, result: contract.ProjectShutdown{ConfigID: p.view.ConfigID, LocalClosed: true, LocksReleased: true}})
	}
	return d
}

func (d *ApplicationDirectory) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started || d.stopping {
		return
	}
	d.started = true
	go d.run()
}
func (d *ApplicationDirectory) run() {
	defer d.doneOnce.Do(func() { close(d.done) })
	census, cancel := context.WithTimeout(d.ctx, 30*time.Second)
	type resolved struct {
		entry *catalogEntry
		ref   api.ProjectRef
		err   error
	}
	results := make(chan resolved, len(d.entries))
	censusSlots := make(chan struct{}, 8)
	pending := 0
	for _, entry := range d.entries {
		if entry.view.RuntimeState == "failed" {
			continue
		}
		pending++
		go func(e *catalogEntry) {
			select {
			case censusSlots <- struct{}{}:
			case <-census.Done():
				results <- resolved{entry: e, err: census.Err()}
				return
			}
			defer func() { <-censusSlots }()
			ref := e.ref
			err := projectconnection.ResolveSnapshot(census, &ref, e.data)
			results <- resolved{entry: e, ref: ref, err: err}
		}(entry)
	}
	received := map[*catalogEntry]bool{}
	for pending > 0 {
		select {
		case result := <-results:
			pending--
			received[result.entry] = true
			d.mu.Lock()
			if result.err != nil {
				result.entry.view.RuntimeState = "failed"
				result.entry.view.Diagnostic = boundedDiagnostic(result.err.Error(), 4096)
			} else {
				e := result.entry
				e.ref = result.ref
				e.view.ProjectID = httpcontract.ProjectID(result.ref.Identity)
				e.result.ProjectID = e.view.ProjectID
				e.view.Repository = strings.TrimSuffix(result.ref.Identity, string(filepath.Separator)+".git")
			}
			d.mu.Unlock()
		case <-census.Done():
			d.mu.Lock()
			for _, e := range d.entries {
				if !received[e] && e.view.RuntimeState != "failed" {
					e.view.RuntimeState = "failed"
					e.view.Diagnostic = "project identity census: " + census.Err().Error()
				}
			}
			d.mu.Unlock()
			pending = 0
		}
	}
	cancel()
	d.mu.Lock()
	for i, a := range d.entries {
		if a.view.ProjectID == "" {
			continue
		}
		for _, b := range d.entries[i+1:] {
			if b.view.ProjectID == "" {
				continue
			}
			if a.ref.Identity == b.ref.Identity || withinResolved(a.ref.RunDir, b.ref.RunDir) || withinResolved(b.ref.RunDir, a.ref.RunDir) {
				reason := "project conflict: repository or overlapping run_dir is shared by " + a.view.ConfigID + " and " + b.view.ConfigID
				for _, e := range []*catalogEntry{a, b} {
					e.view.Conflict = reason
					e.view.Diagnostic = reason
					e.view.RuntimeState = "failed"
				}
			}
		}
	}
	d.mu.Unlock()
	d.mu.RLock()
	var eligible bool
	for _, e := range d.entries {
		if e.view.RuntimeState != "failed" {
			eligible = true
		}
	}
	d.mu.RUnlock()
	if eligible && d.ctx.Err() == nil {
		var global RunnerExecutionConfig
		err := decodeStrictJSON(d.global, &global)
		if err == nil {
			ctx, cancel := context.WithTimeout(d.ctx, 90*time.Second)
			err = d.runner.start(ctx, global, d.home, 30*time.Second, d.prepareRunnerStops)
			cancel()
		}
		if err != nil {
			d.mu.Lock()
			for _, e := range d.entries {
				if e.view.RuntimeState != "failed" {
					e.view.RuntimeState = "failed"
					e.view.Diagnostic = boundedDiagnostic(err.Error(), 4096)
				}
			}
			d.mu.Unlock()
		}
	}
	var wg sync.WaitGroup
	slots := make(chan struct{}, 8)
	for _, e := range d.entries {
		d.mu.RLock()
		failed := e.view.RuntimeState == "failed"
		d.mu.RUnlock()
		if failed {
			continue
		}
		d.mu.Lock()
		e.resultReady = false
		e.result = contract.ProjectShutdown{ConfigID: e.view.ConfigID, ProjectID: e.view.ProjectID}
		d.mu.Unlock()
		wg.Add(1)
		go func(e *catalogEntry) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-d.ctx.Done():
				d.finish(e, contract.ProjectShutdown{ConfigID: e.view.ConfigID, ProjectID: e.view.ProjectID, LocalClosed: true, LocksReleased: true}, nil)
				return
			}
			var releaseOnce sync.Once
			releaseSlot := func() { releaseOnce.Do(func() { <-slots }) }
			defer releaseSlot()
			runtime := &ProjectRuntime{configID: e.view.ConfigID}
			d.mu.Lock()
			e.runtime = runtime
			d.mu.Unlock()
			result, runErr := runtime.Run(d.ctx, d.home, e, d.global, &d.runner, func(port *runtimeProjectPort) {
				d.publish(e, port)
				releaseSlot()
			})
			d.finish(e, result, runErr)
		}(e)
	}
	wg.Wait()
}

func (d *ApplicationDirectory) finish(e *catalogEntry, result contract.ProjectShutdown, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "project %s failed: %v\n", e.view.ConfigID, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	e.result = result
	e.resultReady = true
	e.view.Online = false
	if err != nil {
		e.view.RuntimeState = "failed"
		e.view.Diagnostic = boundedDiagnostic(err.Error()+"; project unavailable; existing Runner executions may still continue", 4096)
		if !e.result.LocalClosed {
			e.result.Diagnostic = err.Error()
		}
	} else {
		e.view.RuntimeState = "stopped"
	}
}
func (d *ApplicationDirectory) Catalog(ctx context.Context) (contract.Catalog, error) {
	if err := ctx.Err(); err != nil {
		return contract.Catalog{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := contract.Catalog{Projects: []contract.Project{}}
	if len(d.global) > 0 {
		result.LoadedExecutionDigest = executionconfig.Digest(d.global)
	}
	for _, e := range d.entries {
		result.Projects = append(result.Projects, e.view)
	}
	return result, d.err
}
func (d *ApplicationDirectory) Connect(ctx context.Context, id string) (applicationhttp.ProjectPort, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, e := range d.entries {
		if e.view.ProjectID != id {
			continue
		}
		if e.view.Conflict != "" {
			return nil, &httpclient.ClientError{Status: 409, Code: "project_conflict", Reason: e.view.Conflict}
		}
		if e.port != nil && (e.view.RuntimeState == "ready" || e.view.RuntimeState == "stopping") {
			return e.port, nil
		}
		return nil, &httpclient.ClientError{Status: 503, Code: "project_unavailable", Reason: fmt.Sprintf("%s: %s", e.view.RuntimeState, e.view.Diagnostic)}
	}
	return nil, &httpclient.ClientError{Status: 503, Code: "project_unavailable", Reason: "project is absent or unresolved"}
}
func (d *ApplicationDirectory) BeginStop() {
	d.mu.Lock()
	d.stopping = true
	for _, e := range d.entries {
		if e.port != nil && !e.resultReady {
			e.port.StopAccepting()
			e.view.RuntimeState = "stopping"
			e.view.Online = false
		}
	}
	d.mu.Unlock()
	d.cancel()
}
func (d *ApplicationDirectory) Close() error {
	d.BeginStop()
	d.mu.Lock()
	if !d.started {
		for _, e := range d.entries {
			if e.view.RuntimeState == "starting" {
				e.view.RuntimeState = "stopped"
			}
		}
		d.doneOnce.Do(func() { close(d.done) })
	}
	d.mu.Unlock()
	<-d.done
	var errs []error
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_ = d.runner.shutdown(ctx)
	cancel()
	if d.runner.ResourceSettlementError() != nil && d.runner.stop != nil {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		_ = d.runner.shutdown(ctx)
		cancel()
	}
	errs = append(errs, d.runner.ResourceSettlementError())
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, e := range d.entries {
		if e.result.Diagnostic != "" {
			errs = append(errs, fmt.Errorf("%s: %s", e.view.ConfigID, e.result.Diagnostic))
		}
	}
	return errors.Join(errs...)
}
func (d *ApplicationDirectory) ShutdownResults() []contract.ProjectShutdown {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := []contract.ProjectShutdown{}
	for _, e := range d.entries {
		r := e.result
		if !e.resultReady {
			r.Diagnostic = "project shutdown is pending"
		}
		result = append(result, r)
	}
	return result
}

func withinResolved(root, path string) bool {
	relative, err := filepath.Rel(canonicalConfigPath(root), canonicalConfigPath(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func (d *ApplicationDirectory) publish(e *catalogEntry, port *runtimeProjectPort) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e.port = port
	e.view.InstanceID = port.instance
	if d.stopping || d.ctx.Err() != nil {
		port.StopAccepting()
		e.view.RuntimeState = "stopping"
		e.view.Online = false
	} else {
		e.view.RuntimeState = "ready"
		e.view.Online = true
	}
}
