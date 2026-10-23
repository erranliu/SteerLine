package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	"strings"
	"time"
)

// The cache contains only one immutable Git tree, never database history or
// mutable worktree files. A changed main-branch commit replaces it entirely.
func (p *testManagementPort) recordSnapshot(ctx context.Context) (testmanagement.RecordSnapshot, error) {
	if p.maintenance == nil {
		return testmanagement.RecordSnapshot{}, errors.New("主分支基线尚未核实")
	}
	commit := p.maintenance.current().Commit
	if commit == "" {
		return testmanagement.RecordSnapshot{}, errors.New("主分支基线尚未核实")
	}
	p.historyMu.Lock()
	defer p.historyMu.Unlock()
	if p.history != nil && p.history.Commit == commit {
		return *p.history, nil
	}
	tree, err := p.maintenance.git(ctx, "ls-tree", "-r", "-z", commit, "--", testmanagement.RecordsPath+"/")
	if err != nil {
		return testmanagement.RecordSnapshot{}, err
	}
	snapshot := testmanagement.RecordSnapshot{Commit: commit, Records: []testmanagement.RecordView{}}
	for _, entry := range bytes.Split(tree, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		parts := bytes.SplitN(entry, []byte{'\t'}, 2)
		if len(parts) != 2 {
			return snapshot, errors.New("invalid Git history tree")
		}
		name := string(parts[1])
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		meta := strings.Fields(string(parts[0]))
		if len(meta) != 3 || meta[1] != "blob" || (meta[0] != "100644" && meta[0] != "100755") {
			return snapshot, fmt.Errorf("%s: history requires regular Git files", name)
		}
		sha, err := testmanagement.RecordCommit(strings.TrimPrefix(name, testmanagement.RecordsPath+"/"))
		if err != nil {
			return snapshot, fmt.Errorf("%s: %w", name, err)
		}
		raw, err := p.maintenance.read(ctx, commit, name)
		if err != nil {
			return snapshot, err
		}
		file, err := testmanagement.ParseRecordFile(raw)
		if err != nil {
			return snapshot, fmt.Errorf("%s: %w", name, err)
		}
		snapshot.Records = append(snapshot.Records, testmanagement.RecordViews(sha, file)...)
	}
	p.history = &snapshot
	return snapshot, nil
}
func (p *testManagementPort) TestRecords(ctx context.Context, in api.TestRecordFilter) (json.RawMessage, error) {
	snapshot, err := p.recordSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	page, err := snapshot.List(p.service.Project, testmanagement.RecordFilter(in))
	if errors.Is(err, testmanagement.ErrInvalidRecordFilter) {
		return nil, &api.Error{Kind: api.InvalidInput, Message: err.Error()}
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(page)
}
func (p *testManagementPort) TestRecord(ctx context.Context, id string) (json.RawMessage, error) {
	snapshot, err := p.recordSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range snapshot.Records {
		if r.ID == id {
			return json.Marshal(r)
		}
	}
	return nil, &api.Error{Kind: api.UnknownRecord, Message: "Git test record not found"}
}
func (p *testManagementPort) TestCatalogue(ctx context.Context) (json.RawMessage, error) {
	type entry struct {
		testmanagement.Definition
		Current bool `json:"current"`
	}
	out := struct {
		Tests      []entry   `json:"tests"`
		Commit     string    `json:"commit"`
		Path       string    `json:"path"`
		ObservedAt time.Time `json:"observed_at"`
		Diagnostic string    `json:"diagnostic,omitempty"`
	}{Tests: []entry{}, Path: testmanagement.CataloguePath, ObservedAt: time.Now().UTC()}
	if p.maintenance == nil {
		out.Diagnostic = "主分支基线尚未核实"
		return json.Marshal(out)
	}
	out.Commit = p.maintenance.current().Commit
	seen := map[string]bool{}
	raw, err := p.maintenance.read(ctx, out.Commit, testmanagement.CataloguePath)
	if err != nil {
		out.Diagnostic = err.Error()
	} else {
		catalogue, e := testmanagement.ParseCatalogue(raw)
		if e != nil {
			out.Diagnostic = e.Error()
		} else {
			for _, d := range catalogue.Tests {
				out.Tests = append(out.Tests, entry{d, true})
				seen[d.ID] = true
			}
		}
	}
	snapshot, e := p.recordSnapshot(ctx)
	if e != nil {
		if out.Diagnostic != "" {
			out.Diagnostic += "; "
		}
		out.Diagnostic += e.Error()
	} else {
		for _, r := range snapshot.Records {
			if !seen[r.TestID] {
				out.Tests = append(out.Tests, entry{testmanagement.Definition{ID: r.TestID, Name: r.TestID, Level: r.Level, Environment: r.Environment}, false})
				seen[r.TestID] = true
			}
		}
	}
	return json.Marshal(out)
}
