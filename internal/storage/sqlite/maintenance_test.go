package sqlite

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	task "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPMSCandidateSnapshotCASVersionsAndReplay(t *testing.T) {
	ctx := context.Background()
	s, project, tasks := nativeAuditStore(t)
	graph, e := s.TaskGraphSnapshot(ctx, project)
	require(t, e)
	original, e := s.TaskContract(ctx, project, tasks[0])
	require(t, e)
	record := tasks[0]
	var first runnerstate.Candidate
	var firstWork task.FixedWork
	var latest runnerstate.Candidate
	var latestWork task.FixedWork
	for i := 0; i < 7; i++ {
		graph, e = s.TaskGraphSnapshot(ctx, project)
		require(t, e)
		record = graph.ByID[record.TaskID]
		c := original
		commit := strings.Repeat(string("abcdef0"[i]), 40)
		c.SpecRef.Commit = commit
		c.PlanRef.Commit = commit
		c.Digest, e = task.ContractDigest(c)
		require(t, e)
		material := pm.Material{TaskID: record.TaskID, Commit: commit, Digest: strings.Repeat(string("abcdef0"[i]), 64), Status: "ready", Contract: c}
		definition := testmanagement.Definition{ID: "one", Name: "One", Level: "L1", Environment: "isolated", Command: "test", Check: "pass"}
		definitions := []testmanagement.Definition{definition}
		work := task.FixedWork{Record: record, Contract: c, Authority: graph.TaskGraph().Authority, ConfirmedDigest: original.Digest, MaterialDigest: material.Digest}
		input := runnerclient.RunInput{Title: record.Title, Repository: runnerclient.Repository{URL: c.Repository, Remote: c.Remote, BaseBranch: c.Branch, BaseRevision: commit}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Path: c.SpecRef.Path}, Plan: runnerclient.Document{Path: c.PlanRef.Path}}, Publication: runnerclient.Publication{BaseBranch: c.Branch}}
		raw, _ := json.Marshal(input)
		candidate := runnerstate.Candidate{Project: project, TaskID: record.TaskID, ID: strings.Repeat(string("abcdef0"[i]), 32), Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "test", ContractDigest: c.Digest, MaterialDigest: material.Digest, BaseRevision: commit, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "submit-" + commit, Input: raw}}
		candidate.TestPlan, _ = json.Marshal(testmanagement.Snapshot{Project: testmodel.ProjectEntry{Version: 1, Requirements: []string{"SteerlineFlow/testing.md"}, TestingCatalogue: "SteerlineFlow/testing/test-list.json"}, Status: "ready", PlanPath: c.PlanRef.Path, Commit: commit, Definitions: definitions, Digest: testmanagement.Digest(definitions)})
		material.TestPlan = candidate.TestPlan
		require(t, s.SaveMaterial(ctx, project, material))
		for _, sibling := range tasks[1:] {
			other := material
			other.TaskID = sibling.TaskID
			require(t, s.SaveMaterial(ctx, project, other))
		}
		input.Workflow.PromptOverrides = map[string]string{"implement.execute": testPromptContext(t, candidate.TestPlan)}
		input.ResultFile = &runnercontract.ResultFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString([]byte(`{"tests":["one"]}`))}
		candidate.Submit.Input, _ = json.Marshal(input)
		if i < 2 {
			// The chosen task is ready, but another unfinished task changes
			// after preparation and before this reservation transaction.
			other, err := s.Material(ctx, project, tasks[1].TaskID)
			require(t, err)
			bad := other
			bad.Status = "failed"
			require(t, s.SaveMaterial(ctx, project, bad))
			err = reserveFixtureCandidate(t, s, ctx, work, candidate, i > 0)
			requireNativeAuditCode(t, err, "project_not_ready")
			g, err := s.TaskGraphSnapshot(ctx, project)
			require(t, err)
			if g.ByID[record.TaskID].Version != record.Version {
				t.Fatal("project-wide rejection changed the chosen task")
			}
			// Repeated rejection keeps the original task version unchanged.

			require(t, s.SaveMaterial(ctx, project, bad))
			err = reserveFixtureCandidate(t, s, ctx, work, candidate, i > 0)
			requireNativeAuditCode(t, err, "project_not_ready")
			require(t, s.SaveMaterial(ctx, project, other))
		}
		if i == 0 {
			bad := material
			bad.Status = "pending"
			require(t, s.SaveMaterial(ctx, project, bad))
			if e = reserveFixtureCandidate(t, s, ctx, work, candidate, false); e == nil {
				t.Fatal("unready generation reserved")
			}
			g, e := s.TaskGraphSnapshot(ctx, project)
			require(t, e)
			if g.ByID[record.TaskID].Status != "open" {
				t.Fatal("failed reserve claimed task")
			}
			require(t, s.SaveMaterial(ctx, project, material))
			first, firstWork = candidate, work
		}
		require(t, reserveFixtureCandidate(t, s, ctx, work, candidate, i > 0))
		latest, latestWork = candidate, work
	}
	// Four further candidates on the latest version are allowed; the sixth is not.
	for i := 0; i < 5; i++ {
		next := latest
		next.ID = fmt.Sprintf("%032x", 100+i)
		next.Submit.RequestID = "submit-" + next.ID
		e := reserveFixtureCandidate(t, s, ctx, latestWork, next, true)
		if i < 4 {
			require(t, e)
		} else if e == nil {
			t.Fatal("sixth current-version candidate accepted")
		}
	}

	// Seven historical versions do not consume the five-current-version limit.
	other, err := s.Material(ctx, project, tasks[1].TaskID)
	require(t, err)
	other.Status = "failed"
	require(t, s.SaveMaterial(ctx, project, other))
	// An unready project must not invalidate an already reserved request.
	require(t, reserveFixtureCandidate(t, s, ctx, firstWork, first, false))
	saved, e := s.Candidate(ctx, project, first.ID)
	require(t, e)
	if saved.Contract.PlanRef.Commit != first.BaseRevision || saved.MaterialDigest != first.MaterialDigest || saved.Number != 1 {
		t.Fatal("original replay upgraded candidate", saved)
	}
}

func TestPMSMaterialObservationSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	ctx := context.Background()
	store, err := OpenSQLiteStore(path)
	require(t, err)
	original := pm.Observation{Commit: strings.Repeat("a", 40), CheckedAt: time.Now().UTC(), LastSuccess: time.Now().UTC(), Diagnostic: "original check failed"}
	require(t, store.SaveMaterialObservation(ctx, "project", original))
	require(t, store.Close())
	store, err = OpenSQLiteStore(path)
	require(t, err)
	defer store.Close()
	got, err := store.MaterialObservation(ctx, "project")
	require(t, err)
	if !reflect.DeepEqual(got, original) {
		t.Fatal("observation lost across reopen", got, original)
	}
	original.Diagnostic = ""
	require(t, store.SaveMaterialObservation(ctx, "project", original))
	got, err = store.MaterialObservation(ctx, "project")
	require(t, err)
	if !reflect.DeepEqual(got, original) {
		t.Fatal("observation upsert failed", got)
	}
}
