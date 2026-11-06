package testenv

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// AsyncActivities uses real application HTTP/Git/SQLite and the public Runner
// fixture. The browser reads the same home; it never provides a business result.
func AsyncActivities(s *Scenario) {
	p := s.NewProject("activities")
	p.PrepareControlledEnvironment("controlled")
	p.CreateTasks()
	p.Claim("a")
	before := p.WaitPhase("waiting_pr_merge")
	candidate := before.Runner.Candidates[0]
	product, e := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(e)
	publicID := product.Identity().ProjectID
	product.Close()
	read := func(filter string) api.AsyncActivities {
		c, e := applicationclient.Open(s.Context, p.Home)
		s.Must(e)
		defer c.Close()
		var out api.AsyncActivities
		s.Must(c.Get(s.Context, "/projects/"+publicID+"/activities?filter="+filter, &out))
		return out
	}
	ongoing := read("ongoing")
	recent := read("recent")
	kinds := map[string]bool{}
	for _, a := range append(ongoing.Items, recent.Items...) {
		kinds[a.Kind] = true
	}
	if !kinds["detection"] || !kinds["test_preparation"] || !kinds["task_execution"] {
		s.t.Fatalf("three activity kinds missing: %+v %+v", ongoing, recent)
	}
	var originalID string
	for _, a := range ongoing.Items {
		if a.SourceID == candidate.ID {
			originalID = a.ID
			if a.State != "waiting" || a.Phase != "等待合并" {
				s.t.Fatalf("PR-ready was prematurely finished: %+v", a)
			}
		}
	}
	if originalID == "" {
		s.t.Fatal("original candidate absent")
	}
	requests, _ := json.Marshal(p.CallerRequests())
	associationClient, e := applicationclient.Open(s.Context, p.Home)
	s.Must(e)
	var associations map[string]json.RawMessage
	s.Must(associationClient.Get(s.Context, "/projects/"+publicID+"/activities?mode=links&request_ids="+candidate.ID, &associations))
	associationClient.Close()
	if _, exists := associations["counts"]; exists {
		s.t.Fatal("association-only HTTP response included counts")
	}
	if _, exists := associations["items"]; exists {
		s.t.Fatal("association-only HTTP response included list")
	}
	var links map[string]api.AsyncActivity
	s.Must(json.Unmarshal(associations["request_links"], &links))
	if links[candidate.ID].ID != originalID || links[candidate.ID].ProjectID != publicID {
		s.t.Fatalf("association-only HTTP scope/binding differs: %+v", links)
	}
	for n := 0; n < 4; n++ {
		read("ongoing")
		read("recent")
	}
	web := s.Run.Web
	if web == "" {
		s.t.Fatal("frozen Web binary required")
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	s.Must(e)
	address := listener.Addr().String()
	s.Must(listener.Close())
	s.Must(controlled.Write(filepath.Join(p.Home, "web.json"), map[string]any{"version": 1, "address": address}))
	s.start(p.Command(web, "serve"), "activities-web")
	s.Wait("isolated Web listener", func() (bool, error) {
		c, e := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if e == nil {
			c.Close()
		}
		return e == nil, e
	})
	node, e := exec.LookPath("node")
	s.Must(e)
	output := filepath.Join(s.Output, "browser")
	s.Must(os.MkdirAll(output, 0700))
	command := p.Command(node, filepath.Join(s.Run.Root, "web", "node_modules", "@playwright", "test", "cli.js"), "test", "tests/async-activities.spec.ts")
	command.Dir = filepath.Join(s.Run.Root, "web")
	command.Env = append(command.Env, "STEERLINE_ACTIVITIES_WEB="+web, "STEERLINE_ACTIVITIES_HOME="+p.Home, "STEERLINE_ACTIVITIES_PROJECT="+publicID, "STEERLINE_WEB_EVIDENCE="+output)
	_, e = s.RunCommand(command, "activities-browser")
	s.Must(e)
	afterRequests, _ := json.Marshal(p.CallerRequests())
	if string(requests) != string(afterRequests) {
		s.t.Fatal("activity observation or browser added execution inputs")
	}
	p.RestartService()
	s.Wait("original activity after service restart", func() (bool, error) {
		for _, a := range read("ongoing").Items {
			if a.ID == originalID && a.SourceID == candidate.ID {
				return true, nil
			}
		}
		return false, nil
	})
	after := p.WaitPhase("waiting_pr_merge")
	if after.Runner.Candidates[0].RunID != candidate.RunID {
		s.t.Fatal("observation replaced original Run")
	}
	p.Merge()
	p.WaitPhase("completed")
	p.WaitStoppedCandidates()
	p.StopService()
	s.Must(controlled.Write(filepath.Join(s.Output, "activity-proof.json"), map[string]any{"passed": true, "original_activity_id": originalID, "three_kinds": kinds, "no_extra_execution_inputs": true, "restarted_original_run": true, "resources_settled": true}))
	s.Must(s.FinishBody())
}
