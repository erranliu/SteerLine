package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	privatefs "github.com/erranliu/SteerLine/internal/product/httpfs"
	"github.com/erranliu/SteerLine/internal/projectconnection"
)

type catalogEntry struct {
	view        contract.Project
	fingerprint string
	ref         api.ProjectRef
	resolving   bool
	sampled     time.Time
	client      *httpclient.Client
	discovery   httpcontract.Discovery
	connecting  chan struct{}
}

// ApplicationDirectory owns only read-only connection facts. Each routed call
// borrows a verified HTTP client. Retiring its idle transport never cancels an active call.
type ApplicationDirectory struct {
	home    string
	mu      sync.Mutex
	entries map[string]*catalogEntry
	slots   chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewApplicationDirectory(home string) *ApplicationDirectory {
	ctx, cancel := context.WithCancel(context.Background())
	return &ApplicationDirectory{home: home, entries: map[string]*catalogEntry{}, slots: make(chan struct{}, 8), ctx: ctx, cancel: cancel}
}
func (d *ApplicationDirectory) Close() error {
	d.cancel()
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.entries {
		if e.client != nil {
			e.client.Close()
		}
	}
	return nil
}
func fingerprint(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "error:" + err.Error()
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return "error:" + err.Error()
	}
	if len(data) > 1<<20 {
		return "error:configuration too large"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (d *ApplicationDirectory) Catalog(ctx context.Context) (contract.Catalog, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := contract.Catalog{Projects: []contract.Project{}}
	f, err := os.Open(filepath.Join(d.home, "etc"))
	if os.IsNotExist(err) {
		for _, e := range d.entries {
			if e.client != nil {
				e.client.Close()
			}
		}
		d.entries = map[string]*catalogEntry{}
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer f.Close()
	files, err := f.ReadDir(513)
	if err != nil && err != io.EOF {
		return result, err
	}
	if len(files) > 512 {
		return result, errors.New("project directory exceeds 512 entries")
	}
	seen := map[string]bool{}
	for _, file := range files {
		if file.IsDir() || !strings.EqualFold(filepath.Ext(file.Name()), ".json") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		key := file.Name()
		seen[key] = true
		path := filepath.Join(d.home, "etc", key)
		stamp := fingerprint(path)
		e := d.entries[key]
		if e == nil || e.fingerprint != stamp {
			if e != nil && e.client != nil {
				e.client.Close()
			}
			e = &catalogEntry{view: contract.Project{ConfigID: key, Name: strings.TrimSuffix(key, filepath.Ext(key)), Diagnostic: "正在解析项目连接"}, fingerprint: stamp}
			d.entries[key] = e
		}
		if !e.resolving && time.Since(e.sampled) > 10*time.Second {
			select {
			case d.slots <- struct{}{}:
				e.resolving = true
				go d.resolve(key, path, e)
			default:
			}
		}
	}
	for key := range d.entries {
		if !seen[key] {
			if d.entries[key].client != nil {
				d.entries[key].client.Close()
			}
			delete(d.entries, key)
		}
	}
	duplicates := map[string]int{}
	for _, e := range d.entries {
		if e.view.ProjectID != "" {
			duplicates[e.view.ProjectID]++
		}
	}
	for _, e := range d.entries {
		view := e.view
		if duplicates[view.ProjectID] > 1 {
			view.Conflict = "同仓库配置关联/冲突；控制操作已禁用"
			view.Online = false
		}
		result.Projects = append(result.Projects, view)
	}
	sort.Slice(result.Projects, func(i, j int) bool { return result.Projects[i].ConfigID < result.Projects[j].ConfigID })
	return result, nil
}
func (d *ApplicationDirectory) resolve(key, path string, e *catalogEntry) {
	defer func() { <-d.slots }()
	ctx, cancel := context.WithTimeout(d.ctx, 15*time.Second)
	defer cancel()
	ref := api.ProjectRef{ConfigPath: path, ApplicationHome: d.home}
	err := projectconnection.Resolve(ctx, &ref)
	view := contract.Project{ConfigID: key, Name: strings.TrimSuffix(key, filepath.Ext(key))}

	if err == nil {
		view.ProjectID = httpcontract.ProjectID(ref.Identity)
		view.Repository = strings.TrimSuffix(ref.Identity, string(filepath.Separator)+".git")
		view.Name = filepath.Base(view.Repository)
		d.mu.Lock()
		if d.entries[key] != e {
			d.mu.Unlock()
			return
		}
		e.ref = ref
		e.view = view
		d.mu.Unlock()
		client, openErr := d.nativeClient(ctx, key, e, ref)
		if openErr == nil {
			view.Online = client.Identity().Ready
			client.Close()
		}
		err = openErr
	}
	view.Diagnostic = ""
	if err != nil {
		view.Diagnostic = err.Error()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.entries[key] != e {
		return
	}
	e.ref = ref
	e.view = view
	e.sampled = time.Now()
	e.resolving = false
}
func (d *ApplicationDirectory) Connect(ctx context.Context, id string) (*httpclient.Client, error) {
	catalog, err := d.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	var key string
	for _, p := range catalog.Projects {
		if p.ProjectID == id {
			if p.Conflict != "" {
				return nil, &httpclient.ClientError{Status: 409, Code: "project_conflict", Reason: p.Conflict}
			}
			key = p.ConfigID
		}
	}
	if key == "" {
		return nil, &httpclient.ClientError{Status: 503, Code: "project_unavailable", Reason: "project is absent, changed, or unresolved"}
	}
	d.mu.Lock()
	e := d.entries[key]
	if e == nil || e.view.ProjectID != id {
		d.mu.Unlock()
		return nil, errors.New("project mapping changed")
	}
	ref := e.ref
	stamp := e.fingerprint
	d.mu.Unlock()
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := d.nativeClient(connectCtx, key, e, ref)
	if err != nil {
		return nil, fmt.Errorf("project offline: %w", err)
	}
	// A config replaced during discovery cannot redirect a request to a new target.
	d.mu.Lock()
	current := d.entries[key] == e && e.ref.Identity == ref.Identity && e.ref.RunDir == ref.RunDir
	d.mu.Unlock()
	if !current || fingerprint(ref.ConfigPath) != stamp {
		client.Close()
		return nil, errors.New("project mapping changed during connection")
	}
	return client, nil
}

// A live transport is reused only while private discovery still names the same
// repository, native instance, OS generation, address, and credential location.
// Close only retires idle connections; already dispatched calls keep their owner.
func (d *ApplicationDirectory) nativeClient(ctx context.Context, key string, e *catalogEntry, ref api.ProjectRef) (*httpclient.Client, error) {
	dir := filepath.Join(ref.RunDir, "local-api")
	if err := privatefs.VerifyPrivateDirectory(dir); err != nil {
		return nil, err
	}
	discovery, err := privatefs.ReadDiscoveryFile(filepath.Join(dir, "discovery.json"))
	if err != nil {
		return nil, err
	}
	if discovery.RepositoryIdentity != ref.Identity || discovery.ProjectID != httpcontract.ProjectID(ref.Identity) || !privatefs.PrivatePath(filepath.Join(dir, "credentials"), discovery.CredentialPath) {
		return nil, errors.New("native discovery project or credential ownership differs")
	}
	if err = privatefs.VerifyPrivateDirectory(filepath.Join(dir, "credentials")); err != nil {
		return nil, err
	}
	if err = privatefs.VerifyPrivateFile(discovery.CredentialPath); err != nil {
		return nil, err
	}
	exited, err := httpclient.ProcessExited(discovery.Process)
	if err != nil || exited {
		return nil, errors.New("native process identity is unavailable")
	}
	d.mu.Lock()
	if d.entries[key] != e {
		d.mu.Unlock()
		return nil, errors.New("project mapping changed")
	}
	previous := e.discovery
	if e.client != nil && previous.InstanceID == discovery.InstanceID && previous.Process == discovery.Process && previous.Address == discovery.Address && previous.CredentialPath == discovery.CredentialPath && previous.LeaseID == discovery.LeaseID {
		c := e.client
		d.mu.Unlock()
		return c, nil
	}
	if pending := e.connecting; pending != nil {
		d.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-pending:
			return d.nativeClient(ctx, key, e, ref)
		}
	}
	pending := make(chan struct{})
	e.connecting = pending
	d.mu.Unlock()
	c, err := httpclient.OpenWithTimeout(ctx, ref.RunDir, ref.Identity, 2*time.Minute)
	d.mu.Lock()
	defer d.mu.Unlock()
	defer close(pending)
	e.connecting = nil
	if d.entries[key] != e || d.ctx.Err() != nil {
		if c != nil {
			c.Close()
		}
		return nil, errors.New("project mapping closed during connection")
	}
	if err != nil {
		return nil, err
	}
	if e.client != nil {
		e.client.Close()
	}
	e.client = c
	e.discovery = discovery
	return c, nil
}
