//go:build e2e

package testing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	tm "github.com/erranliu/SteerLine/internal/testmanagement"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"github.com/google/uuid"
)

func TestMaterialGeneration(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "testing.material-generation")
	p := s.NewProject("materials")
	// Keep the shared minimal/L2 fixture unchanged. This scenario needs one
	// nonempty, self-contained arrangement to verify actual result consumption.
	definition := tm.Definition{ID: "fixture.materials", Name: "Fixture repository", Level: "L1", Environment: "isolated", Command: "git rev-parse --is-inside-work-tree", Check: "command succeeds and prints true for this fixture repository"}
	s.Must(controlled.Write(filepath.Join(p.Repo, "SteerlineFlow/testing/test-list.json"), tm.Catalogue{Tests: []tm.Definition{definition}}))
	s.Must(os.WriteFile(filepath.Join(p.Repo, "SteerlineFlow/testing.md"), []byte("Run fixture.materials to verify the fixture repository; no other tests are required.\n"), 0600))
	s.Must(os.WriteFile(filepath.Join(p.Repo, "SteerlineFlow/plans/task.plan.md"), []byte("# Material generation fixture\nRequire fixture.materials from the committed catalogue.\n"), 0600))
	s.Must(controlled.Write(filepath.Join(p.Repo, "SteerlineFlow/plans/task.tests.json"), tm.Arrangement{Tests: []string{definition.ID}}))
	git := func(args ...string) { _, err := controlled.Git(s.Context, p.Repo, args...); s.Must(err) }
	git("add", ".")
	git("commit", "--quiet", "-m", "Prepare nonempty material arrangement fixture")
	provider, err := controlled.Read(p.Provider)
	s.Must(err)
	git("push", "--quiet", provider.GitURL, "main")
	p.CreateTasks() // Preparation is registered explicitly through project maintenance.
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer project.Close()
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	prefix := "/projects/" + project.Identity().ProjectID
	material, err := driver.WaitFixtureMaterials(s.Context, p.Home, project.Identity(), "SteerlineFlow/plans/task.plan.md", filepath.Join(s.Output, "materials"))
	s.Must(err)
	if material.Status != "ready" || material.GenerationID == "" || material.Inputs == "" || material.Commit != p.Tasks["a"].Contract.PlanRef.Commit {
		t.Fatal("missing ready material binding", material)
	}
	for _, ref := range []struct{ Path, SHA string }{{material.Contract.SpecRef.Path, material.Contract.SpecRef.SHA256}, {material.Contract.PlanRef.Path, material.Contract.PlanRef.SHA256}} {
		body, err := os.ReadFile(filepath.Join(p.Repo, ref.Path))
		s.Must(err)
		if ref.SHA != tm.ContentDigest(body) {
			t.Fatal("material reference does not match committed document", ref.Path)
		}
	}
	var generated api.TestGeneration
	s.Must(client.Get(s.Context, prefix+"/tests/generations/"+material.GenerationID, &generated))
	if generated.ID != material.GenerationID || generated.SourceCommit != material.Commit || generated.Plan != material.Contract.PlanRef.Path || generated.State != "generated" || generated.StopStatus != "stopped" || generated.RunID == "" || len(generated.Result) == 0 {
		t.Fatal("automatic generation result is not bound and settled", generated)
	}
	var arrangement tm.Arrangement
	s.Must(json.Unmarshal(generated.Result, &arrangement))
	if len(arrangement.Tests) == 0 {
		t.Fatal("generated empty arrangement")
	}
	fixturePath := filepath.Join(runnerstatepath.Directory(p.Home), "caller-fixture.json")
	before, err := os.ReadFile(fixturePath)
	s.Must(err)
	var original controlled.RunnerFixture
	s.Must(json.Unmarshal(before, &original))
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	identity := project.Identity()
	in := httpcontract.CommandEnvelope{RequestID: id, ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: "maintenance_generate_all", TargetID: identity.ProjectID, ExpectedVersion: 1, Payload: json.RawMessage(`{}`)}
	for i := 0; i < 2; i++ {
		var response httpcontract.CommandResponse
		s.Must(client.Post(s.Context, prefix+"/commands", in, &response))
		s.Must(controlled.Write(filepath.Join(s.Output, "empty-batch-"+string(rune('a'+i))+".json"), response))
		var progress pm.GenerationProgress
		s.Must(json.Unmarshal(response.Maintenance, &progress))
		if response.Status != "applied" || progress.Status != "queued" && progress.Status != "completed" {
			t.Fatal("ready material preparation was not registered", response)
		}
	}
	again, err := driver.WaitFixtureMaterials(s.Context, p.Home, identity, material.Contract.PlanRef.Path, filepath.Join(s.Output, "rechecked"))
	s.Must(err)
	if !reflect.DeepEqual(material, again) {
		t.Fatal("no-op command changed original material", material, again)
	}
	var final api.TestGeneration
	s.Must(client.Get(s.Context, prefix+"/tests/generations/"+material.GenerationID, &final))
	if !reflect.DeepEqual(generated, final) {
		t.Fatal("no-op command changed generation")
	}
	after, err := os.ReadFile(fixturePath)
	s.Must(err)
	var observed controlled.RunnerFixture
	s.Must(json.Unmarshal(after, &observed))
	if !reflect.DeepEqual(original.Requests, observed.Requests) || !reflect.DeepEqual(original.Bodies, observed.Bodies) {
		t.Fatal("no-op/replay submitted an extra Runner request")
	}
	snapshot, err := p.Snapshot()
	s.Must(err)
	if len(snapshot.Runner.Candidates) != 0 || len(p.CallerRequests()) != 0 {
		t.Fatal("material preparation dispatched business execution")
	}
	p.Record("generated-without-business", snapshot)
	s.Must(controlled.Write(filepath.Join(s.Output, "generation.json"), generated))
	p.StopService()
	s.Must(s.FinishBody())
}
