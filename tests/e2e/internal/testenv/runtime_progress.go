package testenv

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// RuntimeProgress verifies caller persistence and its public query independently of native final.
func RuntimeProgress(s *Scenario) {
	p := s.NewProject("progress")
	definition := testmanagement.Definition{ID: "fixture.progress", Name: "Controlled progress", Level: "L1", Environment: "isolated", Command: "fixture assertion", Check: "nonzero actual assertion"}
	definition.Note = strings.Repeat("完整固定入口约束；", 100)
	s.Must(os.MkdirAll(filepath.Join(p.Repo, "SteerlineFlow", "testing"), 0700))
	s.Must(controlled.Write(filepath.Join(p.Repo, "SteerlineFlow", "testing", "test-list.json"), testmanagement.Catalogue{Tests: []testmanagement.Definition{definition}}))
	s.Must(os.WriteFile(filepath.Join(p.Repo, "SteerlineFlow", "testing.md"), []byte("Run exactly fixture.progress; passing assertions complete its progress field.\n"), 0600))
	s.Must(os.WriteFile(filepath.Join(p.Repo, "SteerlineFlow", "plans", "task.plan.md"), []byte("# Progress plan\nRequires fixture.progress automatic assertions.\n"), 0600))
	p.git(p.Repo, "add", ".")
	p.git(p.Repo, "commit", "--quiet", "-m", "Progress fixture arrangement")
	pushProvider, pushErr := controlled.Read(p.Provider)
	s.Must(pushErr)
	p.git(p.Repo, "push", "--quiet", pushProvider.GitURL, "main")
	p.CreateTasks()
	provider, err := controlled.Read(p.Provider)
	s.Must(err)
	provider.Hold = true
	s.Must(controlled.Write(p.Provider, provider))
	p.Claim("a")
	var candidateID, runID string
	s.Wait("bound original progress Run", func() (bool, error) {
		snap, err := p.Snapshot()
		if err != nil {
			return false, err
		}
		if len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].RunID == "" {
			return false, nil
		}
		c := snap.Runner.Candidates[0]
		candidateID, runID = c.ID, c.RunID
		var input runnercontract.RunInput
		if err = json.Unmarshal(c.Submit.Input, &input); err != nil {
			return false, err
		}
		if input.ProgressFile == nil {
			return false, fmt.Errorf("missing runtime progress input")
		}
		var frozen testmanagement.Snapshot
		if err = json.Unmarshal(c.TestPlan, &frozen); err != nil {
			return false, err
		}
		referenced := false
		for _, ref := range input.Documents.References {
			referenced = referenced || ref.Path == testmodel.WriterPath
		}
		body := input.Workflow.PromptOverrides["implement.execute"]
		context, err := prompts.TestingContext(c.TestPlan, referenced)
		if err != nil {
			return false, err
		}
		if len(frozen.Definitions) != 1 || frozen.Definitions[0] != definition || !strings.Contains(body, context) || strings.Contains(body, definition.Note) {
			return false, fmt.Errorf("compact input lost frozen definition or step boundary")
		}
		// This controlled peer publishes implementation only. Full published
		// execute/report/publication isolation is exercised by the catalog owner.
		readOnly, err := prompts.TestingReadOnly()
		if err != nil {
			return false, err
		}
		if publication, exists := input.Workflow.PromptOverrides["publish"]; exists && !strings.Contains(publication, readOnly) {
			return false, fmt.Errorf("publication lost read-only boundary")
		}
		return true, nil
	})
	// The product's HTTP project identity is the hash, not the Git identity.
	projectClient, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	publicProject := projectClient.Identity().ProjectID
	projectClient.Close()
	read := func() (testmanagement.View, error) {
		client, err := applicationclient.Open(s.Context, p.Home)
		if err != nil {
			return testmanagement.View{}, err
		}
		defer client.Close()
		var view testmanagement.View
		err = client.Get(s.Context, "/projects/"+publicProject+"/runs/"+candidateID+"/tests", &view)
		return view, err
	}
	wait := func(version uint64, passed int, diagnostic bool) {
		s.Wait(fmt.Sprintf("progress %d/%d", version, passed), func() (bool, error) {
			v, err := read()
			if err != nil {
				return false, err
			}
			if v.ProgressVersion == version && v.Passed == passed && (v.Diagnostic != "") == diagnostic {
				s.Must(controlled.Write(filepath.Join(s.Output, fmt.Sprintf("snapshot-%d-%d-%t.json", version, passed, diagnostic)), v))
				return true, nil
			}
			return false, nil
		})
	}
	wait(1, 0, false)
	stateDir := runnerstatepath.Directory(p.Home)
	raw, err := os.ReadFile(filepath.Join(stateDir, "caller-fixture.json"))
	s.Must(err)
	var fixture controlled.RunnerFixture
	s.Must(json.Unmarshal(raw, &fixture))
	initialRequests := p.CallerRequests()
	initialRevision := fixture.Runs[runID].Revision
	set := func(body string, version uint64, status string) {
		snap := fixture.Progress[runID]
		encoded := base64.StdEncoding.EncodeToString([]byte(body))
		snap.ContentBase64 = &encoded
		snap.SizeBytes = len(body)
		snap.SHA256 = testmanagement.ContentDigest([]byte(body))
		snap.Version = version
		snap.CollectionStatus = status
		s.Must(controlled.Write(filepath.Join(stateDir, runID+"-progress.json"), snap))
	}
	set(`{"test.fixture.progress":true}`, 2, "observing")
	wait(2, 1, false)
	set(`{"test.fixture.progress":false}`, 3, "observing")
	wait(3, 0, false)
	set(`{"test.fixture.progress":true}`, 4, "observing")
	wait(4, 1, false)
	set(`{"test.fixture.progress":null}`, 5, "observing")
	wait(4, 1, true)
	set(`{"test.fixture.progress":true}`, 6, "unavailable")
	wait(6, 1, true)
	set(`{"test.fixture.progress":true}`, 6, "observing")
	wait(6, 1, false)
	p.RestartService()
	wait(6, 1, false)
	if !reflect.DeepEqual(initialRequests, p.CallerRequests()) {
		s.t.Fatal("progress observation/restart sent extra business inputs")
	}
	raw, err = os.ReadFile(filepath.Join(stateDir, "caller-fixture.json"))
	s.Must(err)
	s.Must(json.Unmarshal(raw, &fixture))
	if fixture.Runs[runID].Revision != initialRevision || fixture.Runs[runID].Final != nil {
		s.t.Fatal("progress changed business revision or required native final")
	}
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	var history struct {
		Records []any `json:"records"`
	}
	s.Must(client.Get(s.Context, "/projects/"+publicProject+"/tests/records", &history))
	if len(history.Records) != 0 {
		s.t.Fatal("progress created managed history records")
	}
	p.StopService()
	p.StartService()
	wait(6, 1, false)
	s.Wait("closed original Run retains its final progress", func() (bool, error) {
		v, err := read()
		return v.CollectionStatus == "closed" && v.ProgressVersion == 6 && v.Passed == 1 && v.Diagnostic == "", err
	})
	// Removing the peer's latest override must not erase the persisted terminal snapshot.
	s.Must(os.Remove(filepath.Join(stateDir, runID+"-progress.json")))
	p.RestartService()
	wait(6, 1, false)
}
