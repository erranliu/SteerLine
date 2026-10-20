package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

// A missing/invalid optional testing document is a display diagnostic, never a
// new Task eligibility rule. The immutable Plan commit supplies both paths.
func (c Config) prepareTestPlan(ctx context.Context, work taskmodel.FixedWork) (json.RawMessage, []runnerclient.Document) {
	snapshot := testmanagement.Snapshot{Status: "missing", PlanPath: work.Contract.PlanRef.Path, Commit: work.Contract.PlanRef.Commit, Definitions: []testmanagement.Definition{}}
	refs := []runnerclient.Document{}
	finish := func() (json.RawMessage, []runnerclient.Document) { raw, _ := json.Marshal(snapshot); return raw, refs }
	name, err := testmanagement.ArrangementPath(snapshot.PlanPath)
	if err != nil {
		snapshot.Status = "invalid"
		snapshot.Diagnostic = err.Error()
		return finish()
	}
	reader := taskDocuments{repository: c.Repository, git: c.Git, timeout: positiveDuration(c.CommandTimeout, 30*time.Second), processHost: c.ProcessHost}
	read := func(path string) ([]byte, error) {
		body, e := reader.readCommittedFile(ctx, snapshot.Commit, path)
		if e != nil {
			return nil, e
		}
		if len(body) > testmanagement.MaxDocumentBytes {
			return nil, errors.New("test document exceeds 1 MiB")
		}
		refs = append(refs, runnerclient.Document{Kind: "git", Path: path, Commit: snapshot.Commit})
		return body, nil
	}
	arrangement, err := read(name)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			snapshot.Status = "invalid"
		}
		snapshot.Diagnostic = err.Error()
		return finish()
	}
	snapshot.Status = "invalid"
	catalogue, err := read(testmanagement.CataloguePath)
	if err != nil {
		snapshot.Diagnostic = err.Error()
		return finish()
	}
	catalog, err := testmanagement.ParseCatalogue(catalogue)
	if err != nil {
		snapshot.Diagnostic = err.Error()
		return finish()
	}
	snapshot.Definitions, err = testmanagement.Select(arrangement, catalog)
	if err != nil {
		snapshot.Diagnostic = err.Error()
		return finish()
	}
	instructions := testmodel.TestingInstructions
	refs = append(refs, runnerclient.Document{Kind: "inline", Path: "steerline-testing.md", Content: &instructions})
	snapshot.Status = "ready"
	snapshot.Digest = testmanagement.Digest(snapshot.Definitions)
	return finish()
}
