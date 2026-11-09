package testenv

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/mainagent"
	"github.com/erranliu/SteerLine/internal/mainagent/contract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

type adviceView struct {
	ID       string            `json:"id"`
	Revision uint64            `json:"revision"`
	State    string            `json:"state"`
	Settled  bool              `json:"settled"`
	Prompt   string            `json:"prompt"`
	Context  *contract.Context `json:"context"`
}

// Only the public peer is scripted. Production APIs, owner observation, storage,
// admission, asynchronous work and browser code are unchanged.
func MainAgentAdvisory(s *Scenario) {
	p := s.NewProject("advisory")
	var config map[string]any
	raw, e := os.ReadFile(filepath.Join(p.Home, "execution.json"))
	s.Must(e)
	s.Must(json.Unmarshal(raw, &config))
	config["max_generate_concurrency"] = 1
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), config))
	provider, e := controlled.Read(p.Provider)
	s.Must(e)
	provider.Hold = true
	s.Must(controlled.Write(p.Provider, provider))
	script := controlled.AdvisoryScript{Results: map[string]controlled.AdvisoryResponse{}}
	write := func() { s.Must(controlled.Write(p.Provider+".advisory.json", script)) }
	setQuestion := func(id, state string) {
		script.Question = &runnercontract.Question{ID: id, Text: "原题-" + id + "：请确认验收依据", AllowFreeText: true}
		script.Results[id] = controlled.AdvisoryResponse{State: state, Prompt: "请依据已提供的原题与材料继续讨论。场景：" + id}
	}
	setQuestion("ready", "ready")
	write()
	p.PrepareControlledEnvironment("controlled")
	p.CreateTasks()
	head := p.git(p.Repo, "rev-parse", "HEAD")
	worktree := p.git(p.Repo, "status", "--porcelain=v1", "--untracked-files=all")
	materials := p.git(p.Repo, "diff", "HEAD", "--", "SteerlineFlow")
	p.Claim("a")
	candidate := p.WaitPhase("waiting_decision").Runner.Candidates[0]
	project, e := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(e)
	public := project.Identity().ProjectID
	project.Close()
	get := func(path string, out any) error {
		client, e := applicationclient.Open(s.Context, p.Home)
		if e != nil {
			return e
		}
		defer client.Close()
		return client.Get(s.Context, "/projects/"+public+path, out)
	}
	fixture := func() controlled.RunnerFixture {
		var f controlled.RunnerFixture
		var raw []byte
		s.Wait("public peer snapshot readable during atomic replacement", func() (bool, error) {
			var err error
			raw, err = os.ReadFile(filepath.Join(runnerstatepath.Directory(p.Home), "caller-fixture.json"))
			return err == nil, err
		})
		s.Must(json.Unmarshal(raw, &f))
		return f
	}
	list := func() []struct {
		ID     string `json:"id"`
		Source struct {
			Occurrence string `json:"occurrence"`
		} `json:"source"`
	} {
		var out struct {
			Records []struct {
				ID     string `json:"id"`
				Source struct {
					Occurrence string `json:"occurrence"`
				} `json:"source"`
			} `json:"records"`
		}
		s.Must(get("/main-agent?candidate_id="+candidate.ID, &out))
		return out.Records
	}
	find := func(question string) string {
		var id string
		s.Wait("registered advice "+question, func() (bool, error) {
			for _, r := range list() {
				if strings.HasSuffix(r.Source.Occurrence, ":"+question) {
					id = r.ID
					return true, nil
				}
			}
			return false, nil
		})
		return id
	}
	read := func(id string) adviceView { var out adviceView; s.Must(get("/main-agent/"+id, &out)); return out }
	wait := func(id, state string) adviceView {
		var out adviceView
		s.Wait("advice "+state, func() (bool, error) { e := get("/main-agent/"+id, &out); return e == nil && out.State == state, e })
		return out
	}
	activity := func(id string) api.AsyncActivity {
		var found api.AsyncActivity
		s.Wait("activity "+id, func() (bool, error) {
			var out api.AsyncActivities
			e := get("/activities?filter=attention", &out)
			if e != nil {
				return false, e
			}
			for _, a := range out.Items {
				if a.Source == "main_agent_advisory" && a.SourceID == id {
					found = a
					return true, nil
				}
			}
			return false, nil
		})
		return found
	}
	bindings := func() map[string]string {
		out := map[string]string{}
		for run, q := range fixture().AdvisoryQuestions {
			out[q] = run
		}
		return out
	}
	assertCount := func(q string, want int) {
		s.t.Helper()
		f := fixture()
		count := 0
		for run, question := range f.AdvisoryQuestions {
			if question == q {
				for request, command := range f.Requests {
					if command.RunID != nil && *command.RunID == run && command.Command == "submit" {
						count += f.SubmitCalls[request]
					}
				}
			}
		}
		if count != want {
			s.t.Fatalf("question %s submitted %d times, want %d", q, count, want)
		}
	}

	// No browser or manual generation before the first settled suggestion.
	firstID := find("ready")
	first := wait(firstID, "ready")
	if !first.Settled || first.Prompt != script.Results["ready"].Prompt || first.Context == nil || len(first.Context.Materials) < 3 {
		s.t.Fatal("automatic frozen result incomplete", first)
	}
	firstActivity := activity(firstID)
	if firstActivity.State != "succeeded" || !firstActivity.NeedsAttention || firstActivity.Details.CandidateID != candidate.ID || firstActivity.Details.Target != firstID {
		s.t.Fatal("activity binding differs", firstActivity)
	}
	assertCount("ready", 1)
	web := startAdvisoryWeb(s, p)
	browser := func(mode, id, activityID, prompt string) {
		before := p.CallerRequests()
		node, e := exec.LookPath("node")
		s.Must(e)
		output := filepath.Join(s.Output, "browser-"+mode)
		s.Must(os.MkdirAll(output, 0700))
		command := p.Command(node, filepath.Join(s.Run.Root, "web", "node_modules", "@playwright", "test", "cli.js"), "test", "tests/main-agent-advisory.spec.ts")
		command.Dir = filepath.Join(s.Run.Root, "web")
		command.Env = append(command.Env, "STEERLINE_ADVISORY_WEB="+web, "STEERLINE_ADVISORY_HOME="+p.Home, "STEERLINE_ADVISORY_PROJECT="+public, "STEERLINE_ADVISORY_CANDIDATE="+candidate.ID, "STEERLINE_ADVISORY_TASK="+candidate.TaskID, "STEERLINE_ADVISORY_ID="+id, "STEERLINE_ADVISORY_ACTIVITY="+activityID, "STEERLINE_ADVISORY_PROMPT="+prompt, "STEERLINE_ADVISORY_MODE="+mode, "STEERLINE_WEB_EVIDENCE="+output)
		_, e = s.RunCommand(command, "advisory-browser-"+mode)
		s.Must(e)
		if !reflect.DeepEqual(before, p.CallerRequests()) {
			s.t.Fatal("browser sent a Runner execution input")
		}
	}
	browser("ready", firstID, firstActivity.ID, first.Prompt)
	script.Question = nil
	write()
	s.Wait("source resolved", func() (bool, error) {
		var out api.AsyncActivities
		e := get("/activities?filter=attention", &out)
		for _, a := range out.Items {
			if a.SourceID == firstID {
				return false, e
			}
		}
		snap, e2 := p.Snapshot()
		if e2 != nil {
			return false, e2
		}
		return len(snap.Runner.Candidates) == 1 && snap.Runner.Candidates[0].Projection.Question == nil, e
	})
	script.Question = &runnercontract.Question{ID: "ready", Text: "原题-ready 恢复后的显示", AllowFreeText: true}
	write()
	activity(firstID)
	p.RestartService()
	restored := wait(firstID, "ready")
	if restored.Prompt != first.Prompt || !reflect.DeepEqual(restored.Context, first.Context) {
		s.t.Fatal("restart or source restoration replaced snapshot")
	}
	assertCount("ready", 1)

	setQuestion("unknown", "running")
	response := script.Results["unknown"]
	response.LoseReceipt, response.Unavailable = true, true
	script.Results["unknown"] = response
	write()
	unknownID := find("unknown")
	wait(unknownID, "unknown")
	s.Wait("unknown receipt followed by failed original inspection", func() (bool, error) {
		f := fixture()
		for run, question := range f.AdvisoryQuestions {
			if question == "unknown" && f.AdvisoryQueryFailures[run] > 0 {
				return true, nil
			}
		}
		return false, nil
	})
	assertCount("unknown", 1)
	unknownRun := bindings()["unknown"]
	unknownQueries := fixture().AdvisoryInspects[unknownRun]
	// Crash with the unknown record intact, then restore peer availability before
	// boot: startup deliberately requires its original recovery queries to work.
	stopContext, stopCancel := context.WithTimeout(s.Context, 30*time.Second)
	s.Must(p.Service.Stop(stopContext))
	stopCancel()
	p.Service = nil
	response.Unavailable = false
	script.Results["unknown"] = response
	write()
	p.StartService()
	wait(unknownID, "running")
	s.Wait("restored unknown observes the same Run", func() (bool, error) { return fixture().AdvisoryInspects[unknownRun] > unknownQueries, nil })
	assertCount("unknown", 1)
	response.State, response.Unavailable = "ready", false
	script.Results["unknown"] = response
	write()
	wait(unknownID, "ready")
	assertCount("unknown", 1)

	setQuestion("failed", "failed")
	write()
	failedID := find("failed")
	failure := wait(failedID, "failed")
	if !failure.Settled {
		s.t.Fatal("fixture failure did not settle")
	}
	assertCount("failed", 1)
	// A subsequent observed source transition is a semantic barrier; no sleep.
	setQuestion("cancel", "running")
	write()
	cancelID := find("cancel")
	wait(cancelID, "running")
	assertCount("failed", 1)
	response = script.Results["cancel"]
	response.Unavailable, response.HoldStop = true, true
	script.Results["cancel"] = response
	write()
	cancelRun := bindings()["cancel"]
	s.Wait("blocked observer before UI cancellation", func() (bool, error) { return fixture().AdvisoryQueryFailures[cancelRun] > 0, nil })
	browser("cancel", cancelID, "", "")
	if value := wait(cancelID, "cancelling"); value.Settled {
		s.t.Fatal("cancel prematurely settled")
	}
	response.Unavailable = false
	script.Results["cancel"] = response
	write()
	s.Wait("original stop accepted", func() (bool, error) { return fixture().AdvisoryStops[cancelRun], nil })
	if read(cancelID).Settled {
		s.t.Fatal("accepted stop treated as settlement")
	}
	setQuestion("after", "ready")
	write()
	afterID := find("after")
	s.Wait("queued frozen context behind occupied slot", func() (bool, error) {
		value := read(afterID)
		return value.State == "queued" && value.Context != nil, nil
	})
	n := fixture().AdvisoryInspects[cancelRun]
	s.Wait("two observer rounds across queued admission", func() (bool, error) { return fixture().AdvisoryInspects[cancelRun] >= n+2, nil })
	assertCount("after", 0)
	response.HoldStop = false
	script.Results["cancel"] = response
	write()
	wait(cancelID, "cancelled")
	wait(afterID, "ready")
	assertCount("after", 1)

	setQuestion("source", "running")
	write()
	sourceID := find("source")
	wait(sourceID, "running")
	response = script.Results["source"]
	response.HoldStop = true
	script.Results["source"] = response
	script.Question = nil
	write()
	value := wait(sourceID, "cancelling")
	if value.Settled {
		s.t.Fatal("source resolution skipped settlement")
	}
	sourceRun := bindings()["source"]
	s.Wait("source resolution stopped only advisory", func() (bool, error) { return fixture().AdvisoryStops[sourceRun], nil })
	response.HoldStop = false
	script.Results["source"] = response
	write()
	wait(sourceID, "cancelled")

	f := fixture()
	if len(f.AdvisoryQuestions) != 6 || len(list()) != 6 {
		s.t.Fatal("wrong number of formal advisory jobs")
	}
	for _, q := range []string{"ready", "unknown", "failed", "cancel", "after", "source"} {
		assertCount(q, 1)
	}
	for request, command := range f.Requests {
		if command.RunID == nil {
			s.t.Fatal("unbound execution command", request)
		}
		runID := *command.RunID
		if f.AdvisoryQuestions[runID] == "" && runID != candidate.RunID {
			run := f.Runs[runID]
			if command.Command != "submit" || run.Workflow.ID != "execute" || run.Correlation["environment_activity"] == "" {
				s.t.Fatal("unexpected execution outside original candidate and advisory", request)
			}
		}
		if command.RunID != nil && *command.RunID == candidate.RunID && command.Command != "submit" {
			s.t.Fatal("unexpected original input", request)
		}
		if command.Command == "stop" && (command.RunID == nil || f.AdvisoryQuestions[*command.RunID] == "") {
			s.t.Fatal("stopped another Run")
		}
		if command.Command == "submit" && command.RunID != nil && f.AdvisoryQuestions[*command.RunID] != "" {
			var input struct {
				Correlation map[string]string `json:"correlation"`
				Execution   struct {
					Settings map[string]any `json:"settings"`
				} `json:"execution"`
				Generate map[string]json.RawMessage `json:"generate"`
			}
			s.Must(json.Unmarshal([]byte(f.Bodies[request]), &input))
			if input.Execution.Settings["model"] != "gpt-6.1-sol" || input.Execution.Settings["reasoning_effort"] != "high" || input.Execution.Settings["access_mode"] != "read_only" {
				s.t.Fatal("main-agent model configuration differs")
			}
			for _, key := range []string{"repository", "directory", "output_schema"} {
				if _, ok := input.Generate[key]; ok {
					s.t.Fatal("unexpected generation capability", key)
				}
			}
			if string(input.Generate["materials"]) != "[]" {
				s.t.Fatal("file/repository materials were supplied")
			}
			var instruction string
			s.Must(json.Unmarshal(input.Generate["instruction"], &instruction))
			advisoryID := input.Correlation["steerline_advisory"]
			frozen := read(advisoryID)
			if frozen.Context == nil {
				s.t.Fatal("missing frozen context")
			}
			expected, err := mainagent.Prepare(contract.Request{Protocol: contract.Protocol, RequestID: advisoryID, Capability: contract.Capability, Context: *frozen.Context})
			s.Must(err)
			if instruction != expected.Text {
				s.t.Fatal("sent instruction differs from complete frozen context and fixed rules")
			}
			if !strings.Contains(instruction, "原题-"+f.AdvisoryQuestions[*command.RunID]) {
				s.t.Fatal("wrong question bound to generation")
			}
		}
	}
	snap, e := p.Snapshot()
	s.Must(e)
	if len(snap.Domain.Deliveries) != 0 || len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].Participation != "active" || f.Runs[candidate.RunID].StopStatus == "stopped" || snap.Helper.PR.URL != "" || p.git(p.Repo, "rev-parse", "HEAD") != head || p.git(p.Repo, "status", "--porcelain=v1", "--untracked-files=all") != worktree || p.git(p.Repo, "diff", "HEAD", "--", "SteerlineFlow") != materials {
		s.t.Fatal("suggestion changed original business or Git/PR")
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "advisory-public-requests.json"), f))
	p.StopService()
	s.Must(controlled.Write(filepath.Join(s.Output, "advisory-proof.json"), map[string]any{"jobs": 6, "automatic_before_browser": true, "immutable_restart_and_return": true, "unknown_only_observed": true, "failure_not_retried": true, "cancel_and_source_settled": true, "shared_slot_held": true, "no_business_side_effects": true, "public_wire_checked": true, "browser_ready": true, "browser_cancel": true}))
	s.Must(s.FinishBody())
}
func startAdvisoryWeb(s *Scenario, p *Project) string {
	if s.Run.Web == "" {
		s.t.Fatal("frozen Web binary required")
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	s.Must(e)
	address := listener.Addr().String()
	s.Must(listener.Close())
	s.Must(controlled.Write(filepath.Join(p.Home, "web.json"), map[string]any{"version": 1, "address": address}))
	s.start(p.Command(s.Run.Web, "serve"), "advisory-web")
	s.Wait("isolated advisory Web listener", func() (bool, error) {
		c, e := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if e == nil {
			c.Close()
		}
		return e == nil, e
	})
	return s.Run.Web
}
