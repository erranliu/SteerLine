package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/applicationhttp"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/projectconnection"
)

type catalogEntry struct {
	view    contract.Project
	ref     api.ProjectRef
	data    []byte
	port    *runtimeProjectPort
	result  contract.ProjectShutdown
	runtime *ProjectRuntime
}

// ApplicationDirectory owns the frozen configuration collection and independent
// project runtimes. Catalog and Connect never initialize storage.
type ApplicationDirectory struct {
	home      string
	mu        sync.RWMutex
	entries   []*catalogEntry
	global    []byte
	err       error
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
}

func NewApplicationDirectory(home string) *ApplicationDirectory {
	ctx, cancel := context.WithCancel(context.Background())
	d := &ApplicationDirectory{home: home, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	d.global, _ = readConfiguration(filepath.Join(home, "execution.json"))
	f, err := os.Open(filepath.Join(home, "etc"))
	if os.IsNotExist(err) {
		return d
	}
	if err != nil {
		d.err = err
		return d
	}
	defer f.Close()
	files, err := f.ReadDir(513)
	if err != nil && err != io.EOF {
		d.err = err
		return d
	}
	if len(files) > 512 {
		d.err = errors.New("project directory exceeds 512 entries")
		return d
	}
	for _, file := range files {
		if file.IsDir() || !strings.EqualFold(filepath.Ext(file.Name()), ".json") {
			continue
		}
		entry := &catalogEntry{view: contract.Project{ConfigID: file.Name(), Name: strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())), RuntimeState: "starting"}, ref: api.ProjectRef{ConfigPath: filepath.Join(home, "etc", file.Name()), ApplicationHome: home}}
		entry.data, err = readConfiguration(entry.ref.ConfigPath)
		if err != nil {
			entry.view.RuntimeState = "failed"
			entry.view.Diagnostic = err.Error()
		}
		d.entries = append(d.entries, entry)
	}
	sort.Slice(d.entries, func(i, j int) bool { return d.entries[i].view.ConfigID < d.entries[j].view.ConfigID })
	return d
}

func readConfiguration(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("configuration exceeds 1 MiB")
	}
	return data, nil
}

func (d *ApplicationDirectory) Start() { d.startOnce.Do(func() { go d.run() }) }
func (d *ApplicationDirectory) run() {
	defer close(d.done)
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
				result.entry.view.Diagnostic = result.err.Error()
			} else {
				e := result.entry
				e.ref = result.ref
				e.view.ProjectID = httpcontract.ProjectID(result.ref.Identity)
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
	var wg sync.WaitGroup
	slots := make(chan struct{}, 8)
	for _, e := range d.entries {
		d.mu.RLock()
		failed := e.view.RuntimeState == "failed"
		d.mu.RUnlock()
		if failed {
			continue
		}
		wg.Add(1)
		go func(e *catalogEntry) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-d.ctx.Done():
				d.finish(e, contract.ProjectShutdown{ConfigID: e.view.ConfigID, ProjectID: e.view.ProjectID, LocalClosed: true, LocksReleased: true, ResourcesSettled: true}, nil)
				return
			}
			var releaseOnce sync.Once
			releaseSlot := func() { releaseOnce.Do(func() { <-slots }) }
			defer releaseSlot()
			runtime, err := d.prepareProject(e)
			if err != nil {
				d.finish(e, contract.ProjectShutdown{ConfigID: e.view.ConfigID, ProjectID: e.view.ProjectID, LocalClosed: true, LocksReleased: true, ResourcesSettled: true}, err)
				return
			}
			d.mu.Lock()
			e.runtime = runtime
			d.mu.Unlock()
			result, runErr := runtime.Run(d.ctx, func(port *runtimeProjectPort) {
				d.mu.Lock()
				if d.ctx.Err() != nil {
					port.StopAccepting()
				}
				e.port = port
				e.view.RuntimeState = "ready"
				e.view.Online = true
				if d.ctx.Err() != nil {
					e.view.RuntimeState = "stopping"
					e.view.Online = false
				}
				e.view.InstanceID = port.instance
				releaseSlot()
				d.mu.Unlock()
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
	e.view.Online = false
	if err != nil {
		e.view.RuntimeState = "failed"
		e.view.Diagnostic = err.Error()
		if !e.result.LocalClosed || !e.result.ResourcesSettled {
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
	for _, e := range d.entries {
		if e.port != nil {
			e.port.StopAccepting()
			e.view.RuntimeState = "stopping"
			e.view.Online = false
		}
	}
	d.cancel()
	d.mu.Unlock()
}
func (d *ApplicationDirectory) Close() error {
	d.Start()
	d.BeginStop()
	<-d.done
	var errs []error
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
		if r.ConfigID == "" {
			r = contract.ProjectShutdown{ConfigID: e.view.ConfigID, ProjectID: e.view.ProjectID, LocalClosed: true, LocksReleased: true, ResourcesSettled: true}
		}
		result = append(result, r)
	}
	return result
}

func withinResolved(root, path string) bool {
	relative, err := filepath.Rel(canonicalConfigPath(root), canonicalConfigPath(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
