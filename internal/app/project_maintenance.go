package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pmm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type maintenanceStore interface {
	MaintenanceReceipt(context.Context, string, string) (json.RawMessage, error)
	TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error)
	TaskContract(context.Context, string, taskmodel.TaskRecord) (taskmodel.TaskContract, error)
}

// committedMaterialReader reads a regular Git file at the exact supplied commit.
type committedMaterialReader interface {
	readCommittedMaterial(ctx context.Context, commit, path string) ([]byte, error)
}

type projectMaintenance struct {
	environmentObservation func(context.Context, string) error
	commitCache            map[string]cachedBaselineCommit
	config                 Config
	store                  maintenanceStore
	mu                     sync.Mutex
	observation            pmm.Observation
	checking               chan struct{}
}

func newProjectMaintenance(c Config, s maintenanceStore) *projectMaintenance {
	m := &projectMaintenance{config: c, store: s}
	if persistence, ok := s.(observationStore); ok {
		m.observation, _ = persistence.MaterialObservation(context.Background(), c.Identity())
	}
	return m
}
func (m *projectMaintenance) git(ctx context.Context, args ...string) ([]byte, error) {
	return m.config.repositoryGit(ctx, args...)
}

func (m *projectMaintenance) readCommittedMaterial(ctx context.Context, commit, path string) ([]byte, error) {
	return m.config.committedDocuments().readCommittedMaterial(ctx, commit, path)
}
func (m *projectMaintenance) current() pmm.Observation {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.observation
}

// readObservedBaseline can wait for the current observation, including its
// environment revocation. An empty commit means no observation is available yet.
func (m *projectMaintenance) readObservedBaseline(ctx context.Context, waitForCheck bool) (string, error) {
	for {
		m.mu.Lock()
		observation, checking := m.observation, m.checking
		m.mu.Unlock()
		if checking == nil {
			if observation.Diagnostic != "" {
				return "", errors.New(observation.Diagnostic)
			}
			return observation.Commit, nil
		}
		if !waitForCheck {
			return "", errors.New("主分支环境依据观察处理中")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-checking:
		}
	}
}

func (m *projectMaintenance) refreshBaseline(ctx context.Context) (string, error) {
	observation, err := m.check(ctx)
	return observation.Commit, err
}

func (m *projectMaintenance) refreshBaselineWithRewrite(ctx context.Context) (commit string, rewritten bool, err error) {
	observation, err := m.check(ctx)
	return observation.Commit, observation.Rewritten, err
}

func (m *projectMaintenance) verifyAncestor(ctx context.Context, ancestor, descendant string) error {
	_, err := m.git(ctx, "merge-base", "--is-ancestor", ancestor, descendant)
	return err
}

func (m *projectMaintenance) check(ctx context.Context) (pmm.Observation, error) {
	m.mu.Lock()
	if done := m.checking; done != nil {
		m.mu.Unlock()
		select {
		case <-done:
			o := m.current()
			if o.Diagnostic != "" {
				return o, errors.New(o.Diagnostic)
			}
			return o, nil
		case <-ctx.Done():
			return pmm.Observation{}, ctx.Err()
		}
	}

	before := m.observation
	m.checking = make(chan struct{})
	done := m.checking
	m.mu.Unlock()
	o := before
	o.CheckedAt = time.Now().UTC()
	o.Diagnostic = ""
	ref := "refs/remotes/" + m.config.Remote + "/" + m.config.Branch
	_, e := m.git(ctx, "fetch", "--no-tags", "--no-write-fetch-head", m.config.Remote, "+refs/heads/"+m.config.Branch+":"+ref)
	if e == nil {
		var b []byte
		b, e = m.git(ctx, "rev-parse", "--verify", ref+"^{commit}")
		if e == nil {
			o.Commit = strings.TrimSpace(string(b))
			o.Previous = before.Commit
			o.LastSuccess = o.CheckedAt
			o.Rewritten = false
			if before.Commit != "" && before.Commit != o.Commit {
				_, a := m.git(ctx, "merge-base", "--is-ancestor", before.Commit, o.Commit)
				o.Rewritten = a != nil
			}
		}
	}
	if e == nil && m.environmentObservation != nil {
		e = m.environmentObservation(ctx, o.Commit)
	}
	if e != nil {
		o.Diagnostic = e.Error()
	}
	m.mu.Lock()
	if persistence, ok := m.store.(observationStore); ok {
		if saveErr := persistence.SaveMaterialObservation(context.WithoutCancel(ctx), m.config.Identity(), o); saveErr != nil {
			e = errors.Join(e, saveErr)
			o.Diagnostic = e.Error()
		}
	}
	m.observation = o
	m.checking = nil
	close(done)
	m.mu.Unlock()
	return o, e
}
func (m *projectMaintenance) observe(ctx context.Context) {
	// A bounded background reader never holds the project actor or a SQL transaction.
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		c, cancel := context.WithTimeout(ctx, 35*time.Second)
		_, _ = m.check(c)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (m *projectMaintenance) localWorkspace(ctx context.Context) (string, error) {
	head, e := m.git(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if e != nil {
		return "", e
	}
	branch, e := m.git(ctx, "symbolic-ref", "--quiet", "HEAD")
	if e != nil {
		return "", errors.New("需要正常检出的分支，请人工核对Git状态")
	}
	return strings.TrimSpace(string(head)) + "\n" + strings.TrimSpace(string(branch)), nil
}

var errGitOperationInProgress = errors.New("Git operation in progress")

func (m *projectMaintenance) workspace(ctx context.Context) (string, error) {
	snapshot, e := m.localWorkspace(ctx)
	if e != nil {
		return "", e
	}
	// No writes during an active merge or rebase. Git reports state paths, not guesses.
	for _, state := range []string{"MERGE_HEAD", "REBASE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		if _, e := m.git(ctx, "rev-parse", "--verify", state); e == nil {
			return "", errGitOperationInProgress
		}
	}
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		path, err := m.git(ctx, "rev-parse", "--git-path", state)
		if err != nil {
			return "", err
		}
		name := strings.TrimSpace(string(path))
		if !filepath.IsAbs(name) {
			name = filepath.Join(m.config.Repository, name)
		}
		if _, err = os.Stat(name); err == nil {
			return "", errGitOperationInProgress
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	remote, err := m.git(ctx, "remote", "get-url", m.config.Remote)
	if err != nil {
		return "", err
	}
	target := pmm.Hash([]string{m.config.Identity(), m.config.Remote, m.config.Branch, strings.TrimSpace(string(remote))})
	return snapshot + "\n" + target, nil
}
func (m *projectMaintenance) resolved(ctx context.Context, c taskmodel.TaskContract, commit string) (taskmodel.TaskContract, error) {
	resolve := func(ref taskmodel.FileReference) (taskmodel.FileReference, error) {
		b, e := m.readCommittedMaterial(ctx, commit, ref.Path)
		if e != nil {
			return ref, e
		}
		ref.Commit = commit
		ref.SHA256 = taskPayloadDigest(b)
		if e = taskmodel.ValidateDocumentContent(ref, b, taskmodel.IsAcceptanceReference(c.AcceptanceRefs, ref)); e != nil {
			return ref, e
		}
		return ref, nil
	}
	var e error
	if c.SpecRef, e = resolve(c.SpecRef); e != nil {
		return c, e
	}
	if c.PlanRef, e = resolve(c.PlanRef); e != nil {
		return c, e
	}
	c.CommonRefs = append([]taskmodel.FileReference(nil), c.CommonRefs...)
	c.AcceptanceRefs = append([]taskmodel.FileReference(nil), c.AcceptanceRefs...)
	for i := range c.CommonRefs {
		if c.CommonRefs[i], e = resolve(c.CommonRefs[i]); e != nil {
			return c, e
		}
	}
	for i := range c.AcceptanceRefs {
		if c.AcceptanceRefs[i], e = resolve(c.AcceptanceRefs[i]); e != nil {
			return c, e
		}
	}
	c.Digest, e = taskmodel.ContractDigest(c)
	return c, e
}
func (m *projectMaintenance) prepare(ctx context.Context, w taskmodel.FixedWork, candidate bool, base func(context.Context, taskmodel.FixedWork, bool) (string, error)) (taskmodel.FixedWork, string, error) {
	return m.prepareLatest(ctx, w, candidate, base)
}

func (m *projectMaintenance) view(ctx context.Context, mode, plan string) (any, error) {
	return m.baselineView(ctx)
}

// reserveBaseline serializes final admission against the entire observation,
// including environment-basis revocation before the new baseline is published.
func (m *projectMaintenance) reserveBaseline(commit string, save func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.checking != nil || m.observation.Commit != commit || m.observation.Diagnostic != "" {
		return errors.New("baseline observation changed or pending before reservation; review again")
	}
	return save()
}
