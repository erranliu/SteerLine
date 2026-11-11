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
	"github.com/erranliu/SteerLine/internal/roadmap"
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
	if !kinds["detection"] || !kinds["task_execution"] || kinds["test_preparation"] {
		s.t.Fatalf("current activity kinds differ: %+v %+v", ongoing, recent)
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
	s.Must(controlled.Write(filepath.Join(s.Output, "activity-proof.json"), map[string]any{"passed": true, "original_activity_id": originalID, "observed_kinds": kinds, "no_extra_execution_inputs": true, "restarted_original_run": true, "resources_settled": true}))
	s.activityFeedback(web, node)
	s.Must(s.FinishBody())
}

// Keep explicit user submissions in a separate project and browser phase so
// the original observation-only zero-input assertion remains unchanged.
func (s *Scenario) activityFeedback(web, node string) {
	p := s.NewProject("feedback")
	_, err := roadmap.PrepareStatic(p.Repo, false)
	s.Must(err)
	draft := filepath.Join(p.Repo, "SteerlineFlow", "Roadmap", "drafts", "feedback.md")
	s.Must(os.MkdirAll(filepath.Dir(draft), 0700))
	s.Must(os.WriteFile(draft, []byte("# Feedback draft\n\nT1: Verify immediate feedback.\n"), 0600))
	p.PrepareControlledEnvironment("controlled")
	p.CreateTasks()
	product, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	project := product.Identity().ProjectID
	product.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	s.Must(err)
	address := listener.Addr().String()
	s.Must(listener.Close())
	s.Must(controlled.Write(filepath.Join(p.Home, "web.json"), map[string]any{"version": 1, "address": address}))
	s.start(p.Command(web, "serve"), "feedback-web")
	s.Wait("feedback Web listener", func() (bool, error) {
		connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			connection.Close()
		}
		return err == nil, err
	})
	output := filepath.Join(s.Output, "feedback-browser")
	s.Must(os.MkdirAll(output, 0700))
	command := p.Command(node, filepath.Join(s.Run.Root, "web", "node_modules", "@playwright", "test", "cli.js"), "test", "tests/async-feedback.spec.ts")
	command.Dir = filepath.Join(s.Run.Root, "web")
	command.Env = append(command.Env, "STEERLINE_ACTIVITIES_WEB="+web, "STEERLINE_ACTIVITIES_HOME="+p.Home, "STEERLINE_ACTIVITIES_PROJECT="+project, "STEERLINE_WEB_EVIDENCE="+output, "STEERLINE_FEEDBACK_DRAFT="+draft)
	_, err = s.RunCommand(command, "feedback-browser")
	s.Must(err)
	proof, err := os.ReadFile(filepath.Join(output, "feedback-browser-proof.json"))
	s.Must(err)
	var evidence struct {
		Passed   bool `json:"passed"`
		Commands []struct {
			Action    string `json:"action"`
			RequestID string `json:"request_id"`
		} `json:"commands"`
	}
	s.Must(json.Unmarshal(proof, &evidence))
	expected := map[string]int{"roadmap_parse": 1, "maintenance_check": 1, "start_task": 1, "start_candidate": 1, "environment_start": 1}
	seen := map[string]bool{}
	for _, command := range evidence.Commands {
		if command.RequestID == "" || seen[command.RequestID] {
			s.t.Fatal("feedback repeated or missing original request ID")
		}
		seen[command.RequestID] = true
		expected[command.Action]--
	}
	if !evidence.Passed || len(evidence.Commands) != len(expected) {
		s.t.Fatal("feedback browser evidence incomplete")
	}
	for action, remaining := range expected {
		if remaining != 0 {
			s.t.Fatalf("feedback command %s count differs", action)
		}
	}
	if len(p.CallerRequests()) != 2 {
		s.t.Fatal("explicit feedback phase must create exactly two candidate inputs")
	}
	s.Wait("both explicit feedback candidates observed", func() (bool, error) {
		snapshot, err := p.Snapshot()
		if err != nil || len(snapshot.Runner.Candidates) != 2 {
			return false, err
		}
		for _, candidate := range snapshot.Runner.Candidates {
			if candidate.SubmitState == "rejected" {
				s.t.Fatal("explicit feedback candidate rejected: ", candidate.Diagnostic)
			}
			if candidate.RunID == "" || candidate.Projection == nil {
				return false, nil
			}
		}
		p.Record("parallel-observed", snapshot)
		return true, nil
	})
	// The observation-only project above proves delivery and merge. This
	// project's two parallel Runs are fenced by the existing owned cleanup.
	p.StopService()
}
