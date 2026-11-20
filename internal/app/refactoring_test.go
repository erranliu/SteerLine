package app

import (
	"context"
	"encoding/json"
	"errors"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/execution/backend"
	em "github.com/erranliu/SteerLine/internal/execution/model"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	rc "github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRefactoringInvestigationPreservesLFSPointer(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "empty.gitconfig"))
	t.Setenv("GIT_CONFIG_COUNT", "0")
	source := t.TempDir()
	fixtureGit(t, source, "init", "-q", "-b", "main")
	pointer := "version https://git-lfs.github.com/spec/v1\noid sha256:" + strings.Repeat("a", 64) + "\nsize 4096\n"
	require(t, os.WriteFile(filepath.Join(source, ".gitattributes"), []byte("bundle.bin filter=lfs -text\n"), 0600))
	require(t, os.WriteFile(filepath.Join(source, "bundle.bin"), []byte(pointer), 0600))
	fixtureGit(t, source, "add", ".")
	fixtureGit(t, source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "LFS pointer")
	revision := fixtureGit(t, source, "rev-parse", "HEAD")
	fixtureGit(t, source, "remote", "add", "origin", "https://example.invalid/project.git")

	// Exercise checkout with an LFS filter without requiring a Git LFS install.
	t.Setenv("GIT_LFS_SKIP_SMUDGE", "0")
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "filter.lfs.smudge")
	t.Setenv("GIT_CONFIG_VALUE_0", `test "$GIT_LFS_SKIP_SMUDGE" = 1 && cat`)
	t.Setenv("GIT_CONFIG_KEY_1", "filter.lfs.required")
	t.Setenv("GIT_CONFIG_VALUE_1", "true")
	port := refactoringPort{config: Config{CommandTimeout: "20s", ProcessHost: NewProcessHost(20 * time.Second)}}
	record := ref.Record{Frozen: ref.Frozen{Repository: rc.Repository{LocalPath: source, BaseRevision: revision}}}
	id := ref.ID()
	require(t, port.investigation(t.Context(), record, id))
	dir, err := investigationPath(id)
	require(t, err)
	body, err := os.ReadFile(filepath.Join(dir, "bundle.bin"))
	require(t, err)
	if string(body) != pointer || fixtureGit(t, dir, "rev-parse", "HEAD") != revision || fixtureGit(t, dir, "remote") != "" {
		t.Fatal("expected the pinned snapshot to preserve the LFS pointer and have no remote")
	}
	require(t, releaseInvestigation(id))

	record.Frozen.Repository.BaseRevision = strings.Repeat("b", 40)
	err = port.investigation(t.Context(), record, id)
	if err == nil || !strings.Contains(err.Error(), "git fetch 失败") || !strings.Contains(err.Error(), "快照时限 20s") {
		t.Fatalf("expected a fetch failure with a snapshot deadline, got %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed snapshot was not removed: %v", err)
	}
}

func TestRefactoringPRVersionAndOriginalBranch(t *testing.T) {
	for _, kind := range []string{"valid", "fork", "closed", "body", "head", "base"} {
		t.Run(kind, func(t *testing.T) {
			head := strings.Repeat("a", 40)
			body := "完整报告"
			base := strings.Repeat("b", 40)
			if kind == "base" {
				base = ""
			}
			fork := "owner/repo"
			state := "open"
			if kind == "fork" {
				fork = "other/repo"
			}
			if kind == "closed" {
				state = "closed"
			}
			if kind == "body" {
				body = "不同正文"
			}
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"number": 1, "html_url": server.URL + "/owner/repo/pulls/1", "body": body, "state": state, "merged": false, "base": map[string]any{"ref": "main", "sha": base, "repo": map[string]string{"full_name": "owner/repo"}}, "head": map[string]any{"ref": "codex/maintenance", "sha": head, "repo": map[string]string{"full_name": fork}}})
			}))
			defer server.Close()
			p := refactoringPort{config: Config{PR: em.PRProviderConfig{Kind: "gitea", BaseURL: server.URL, Repository: "owner/repo"}, CommandTimeout: "3s"}}
			record := ref.Record{Frozen: ref.Frozen{Repository: rc.Repository{BaseBranch: "main"}, Publication: map[string]string{"repository": "owner/repo", "base_url": server.URL}}, Operations: []ref.Operation{{Run: &rc.Run{Execution: rc.Execution{Branch: "codex/maintenance"}}}}}
			result := ref.Result{Head: head, PR: server.URL + "/owner/repo/pulls/1"}
			if kind == "head" {
				result.Head = strings.Repeat("c", 40)
			}
			fact, checks, e := p.verify(context.Background(), record, result)
			if (e == nil) != (kind == "valid" || kind == "body" || kind == "head" || kind == "closed") {
				t.Fatal(kind, fact, e)
			}
			if e == nil && (fact.BaseRevision != base || fact.HeadRevision != head || checks != "") {
				t.Fatal("version query must return the actual platform facts")
			}
		})
	}
}

func TestRefactoringReviewDiffIgnoresLocalReplacementObjects(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	remote, local := t.TempDir(), t.TempDir()
	fixtureGit(t, remote, "init", "-q", "-b", "main")
	commit := func(text string) string {
		require(t, os.WriteFile(filepath.Join(remote, "code.txt"), []byte(text+"\n"), 0600))
		fixtureGit(t, remote, "add", "--", "code.txt")
		fixtureGit(t, remote, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", text)
		return fixtureGit(t, remote, "rev-parse", "HEAD")
	}
	base := commit("original")
	head := commit("reviewed")
	fixtureGit(t, remote, "checkout", "-q", "-b", "unrelated", base)
	replacement := commit("replacement")
	fixtureGit(t, local, "clone", "-q", "--no-checkout", remote, ".")
	fixtureGit(t, local, "replace", head, replacement)
	remoteURL := fixtureGit(t, local, "remote", "get-url", "origin")
	p := refactoringPort{config: Config{Repository: local, Git: backend.Command{Executable: "git"}, CommandTimeout: "10s", ProcessHost: NewProcessHost(10 * time.Second)}}
	r := ref.Record{Frozen: ref.Frozen{Repository: rc.Repository{URL: remoteURL, Remote: "origin"}}}
	diff, err := p.reviewDiff(context.Background(), r, rc.PRFact{BaseRevision: base, HeadRevision: head})
	if err != nil || !strings.Contains(diff, "+reviewed") || strings.Contains(diff, "+replacement") {
		t.Fatalf("review diff differs from the pinned objects: %s, %v", diff, err)
	}
	// The review reads the same head as the diff, never the startup base,
	// mutable local files or local replacement refs.
	require(t, os.WriteFile(filepath.Join(local, "code.txt"), []byte("uncommitted local contents"), 0600))
	r.Phase = "report"
	r.Frozen.Repository.LocalPath, r.Frozen.Repository.BaseRevision = local, base
	r.PR = &rc.PRFact{HeadRevision: head, BaseRevision: base}
	id := ref.ID()
	r.Operations = []ref.Operation{{ID: id, Purpose: "report", PRVersion: head + ":" + base}}
	t.Cleanup(func() { require(t, releaseInvestigation(id)) })
	require(t, p.investigation(t.Context(), r, id))
	directory, err := investigationPath(id)
	require(t, err)
	contents, err := os.ReadFile(filepath.Join(directory, "code.txt"))
	require(t, err)
	if strings.TrimSpace(string(contents)) != "reviewed" || fixtureGit(t, directory, "rev-parse", "HEAD") != head {
		t.Fatal("review source did not use the pinned canonical PR head", string(contents))
	}
	require(t, releaseInvestigation(id))
	r.Operations[0].PRVersion = base + ":" + base
	if err := p.investigation(t.Context(), r, id); err == nil {
		t.Fatal("snapshot accepted a different queued review version")
	}
}

func TestRefactoringStartupAllowsActionsWithoutRunnerBinding(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	cli := startupFixtureCLI(t)
	c := Config{identity: "project", executionPath: filepath.Join(dir, "execution.json"), runtimeRunner: &cli}
	store, err := sqlite.OpenSQLiteStore(filepath.Join(dir, "state.db"))
	require(t, err)
	t.Cleanup(func() { store.Close() })
	require(t, store.InitializeProject(ctx, taskmodel.ProjectTaskAuthority{
		ProjectIdentity: c.Identity(), Authority: "native", AuthorityVersion: 1,
		Repository: "https://example.invalid/repo.git", Remote: "origin", Branch: "main", DispatchMode: "manual",
	}))
	for _, phase := range []string{ref.PhaseCheckingStart, "blocked", "stopped"} {
		now := time.Now().UTC()
		record := ref.Record{ID: ref.ID(), Project: c.Identity(), Revision: 1, Phase: phase, CreatedAt: now, UpdatedAt: now}
		require(t, store.SaveRefactoring(ctx, record, 0, nil))
	}
	require(t, bindRunnerBuild(ctx, c, store))
	stops, err := prepareRefactoringStops(ctx, &c, store, &cli)
	require(t, err)
	if len(stops) != 0 {
		t.Fatal("unbound actions created Runner stop requests")
	}
}

func TestRefactoringStartupReconcilesOriginalStop(t *testing.T) {
	for _, kind := range []string{"missing-stale", "missing-current", "accepted", "rejected", "foreign", "unavailable", "stopped", "deployment-stop-pending", "deployment-stop-unknown"} {
		t.Run(kind, func(t *testing.T) {
			c := Config{identity: "project"}
			cli := startupFixtureCLI(t)
			ctx := context.Background()
			store, e := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, e)
			defer store.Close()
			id := strings.Repeat("a", 32)
			op := ref.Operation{ID: strings.Repeat("b", 32), Purpose: "implement", State: "submitted", RunID: "original-run", InputDigest: "original-input", Request: rc.Request{Protocol: rc.Protocol, RequestID: "original-submit", Input: json.RawMessage(`{}`)}, Stop: runnerbinding.ContentStop("original-stop", 3), StopState: "unknown"}
			run := rc.Run{RunID: op.RunID, Revision: 4, InputDigest: op.InputDigest, Workflow: rc.Workflow{ID: "execute", Version: 1}, Correlation: map[string]string{"steerline_project": c.Identity(), "steerline_refactoring": id, "refactoring_operation": op.ID}, Status: "running", StopStatus: "active"}
			if kind == "missing-current" {
				run.Revision = 3
			}
			if kind == "stopped" {
				run.StopStatus = "stopped"
			}
			status, deploymentStop := strings.CutPrefix(kind, "deployment-stop-")
			if deploymentStop {
				run.StopStatus = status
				op.Stop, op.StopState = nil, ""
			}
			entries := map[string]any{run.RunID: rc.Inspection{Run: &run}}
			if kind == "accepted" || kind == "rejected" || kind == "foreign" {
				target := run.RunID
				if kind == "foreign" {
					target = "foreign"
				}
				status := "pending"
				if kind == "rejected" {
					status = "rejected"
				}
				entries["original-stop"] = rc.Inspection{Run: &run, Command: &rc.Command{RequestID: "original-stop", Command: "stop", RunID: &target, ReceiptStatus: "accepted", ActionStatus: status}}
			}
			if kind == "unavailable" {
				entries["original-stop"] = map[string]string{"fixture_error": "service_unavailable"}
			}
			require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-inspections-fixture.json"), mustJSON(entries), 0600))
			record := ref.Record{ID: id, Project: c.Identity(), Revision: 1, Phase: "execute", Frozen: ref.Frozen{Runner: cli.BuildPin()}, Operations: []ref.Operation{op}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			require(t, store.SaveRefactoring(ctx, record, 0, nil))
			stops, e := prepareRefactoringStops(ctx, &c, store, &cli)
			if kind == "foreign" || kind == "unavailable" {
				if e == nil {
					t.Fatal("unverified stop accepted")
				}
				return
			}
			require(t, e)
			if kind == "stopped" || deploymentStop {
				if len(stops) != 0 {
					t.Fatal(stops)
				}
				if deploymentStop {
					saved, err := store.Refactoring(ctx, c.Identity(), id)
					require(t, err)
					if !saved.StopRequested || saved.Phase != "stopping" || saved.Operations[0].Stop != nil {
						t.Fatal("original deployment stop was replaced or maintenance was reactivated", saved)
					}
				}
				return
			}
			if len(stops) != 1 {
				t.Fatal(stops)
			}
			changed := kind == "missing-stale" || kind == "rejected"
			if (stops[0].Request.RequestID != "original-stop") != changed {
				t.Fatal("wrong original stop decision", kind, stops)
			}
			saved, e := store.Refactoring(ctx, c.Identity(), id)
			require(t, e)
			if changed && len(saved.Operations[0].PriorStops) != 1 {
				t.Fatal("old stop evidence lost")
			}
			again, e := prepareRefactoringStops(ctx, &c, store, &cli)
			require(t, e)
			if len(again) != 1 || again[0].Request.RequestID != stops[0].Request.RequestID {
				t.Fatal("second startup changed unresolved original", again)
			}
		})
	}
}

func TestRefactoringFirstDispatchUsesCurrentQualificationForFrozenBase(t *testing.T) {
	ctx := context.Background()
	environment, config, store := environmentFixture(t)
	config.runtimeMaintenance = newProjectMaintenance(config, store)
	config.runtimeMaintenance.environmentObservation = environment.observeBasis
	environment.config.runtimeMaintenance = config.runtimeMaintenance
	ready := seedDetectedEnvironment(t, environment, "fixture")
	profile, err := config.RunnerExecution.Profile("fixture", "")
	require(t, err)
	key, err := config.profileEnvironmentKey(ctx, profile)
	require(t, err)
	record := ref.Record{
		Project: config.Identity(),
		Input:   ref.Start{Profile: profile.ID, ProfileVersion: refactoringProfileVersion(profile), EnvironmentKey: key},
		Frozen:  ref.Frozen{Repository: rc.Repository{BaseRevision: ready.Prepared.Commit}},
	}
	port := refactoringPort{config: config, store: store}
	require(t, os.WriteFile(filepath.Join(config.Repository, "unrelated.md"), []byte("ordinary project change"), 0600))
	advanced := commitEnvironmentMaterials(t, config)
	proof, err := port.qualifyFirst(ctx, record)
	require(t, err)
	if advanced == record.Frozen.Repository.BaseRevision || proof.Basis != ready.Prepared.Inputs {
		t.Fatal("ordinary branch advance did not preserve the frozen qualification")
	}
	port.config.environmentPolicy = env.NewAdmissionPolicy(false)
	if _, err = port.qualifyFirst(ctx, record); !errors.Is(err, ref.ErrEnvironmentChanged) {
		t.Fatal("developer bypass granted maintenance qualification", err)
	}
	port.config.environmentPolicy = config.environmentPolicy
	port.config.RunnerExecution.Profiles = append([]RunnerProfile(nil), config.RunnerExecution.Profiles...)
	port.config.RunnerExecution.Profiles[0].Execution.Settings = json.RawMessage(`{"executable":"native-fixture","model":"changed","access_mode":"full_access"}`)
	if _, err = port.qualifyFirst(ctx, record); !errors.Is(err, ref.ErrEnvironmentChanged) {
		t.Fatal("changed effective settings admitted original profile ID", err)
	}
	port.config.RunnerExecution.Profiles = config.RunnerExecution.Profiles
	require(t, os.WriteFile(filepath.Join(config.Repository, "go.mod"), []byte("module changed.invalid\n"), 0600))
	commitEnvironmentMaterials(t, config)
	if _, err = port.qualifyFirst(ctx, record); !errors.Is(err, ref.ErrEnvironmentChanged) {
		t.Fatal("changed environment basis admitted frozen action", err)
	}
}

func TestRefactoringListKeepsOngoingAndDeepLinksAcrossHistoryPages(t *testing.T) {
	ctx := context.Background()
	config := Config{identity: "project"}
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, err)
	defer store.Close()
	port := refactoringPort{store: store, config: config}
	now := time.Now().UTC()
	oldest := ref.ID()
	for i := 0; i < 22; i++ {
		id := ref.ID()
		if i == 0 {
			id = oldest
		}
		ended := now.Add(time.Duration(i) * time.Second)
		require(t, store.SaveRefactoring(ctx, ref.Record{
			ID: id, Project: config.Identity(), Revision: 1, Phase: "delivered", CreatedAt: now, UpdatedAt: ended, FinishedAt: &ended,
		}, 0, nil))
	}
	for i := 0; i < 4; i++ {
		require(t, store.SaveRefactoring(ctx, ref.Record{
			ID: ref.ID(), Project: config.Identity(), Revision: 1, Phase: "select", CreatedAt: now, UpdatedAt: now, Slot: i < 3,
		}, 0, nil))
	}
	type page struct {
		Ongoing, History []struct{ ID string }
		Next             string `json:"history_next"`
		Used             int    `json:"slots_used"`
	}
	read := func(values url.Values) page {
		t.Helper()
		view, err := port.read(ctx, "/refactoring", values)
		require(t, err)
		var result page
		require(t, json.Unmarshal(mustJSON(view), &result))
		return result
	}
	first := read(nil)
	if len(first.Ongoing) != 4 || first.Used != 3 || len(first.History) != 20 || first.Next == "" {
		t.Fatal("first page lost queued or historical records", first)
	}
	second := read(url.Values{"history_before": {first.Next}})
	if len(second.History) != 2 || second.History[1].ID != oldest || second.Next != "" {
		t.Fatal("history cursor did not return the final two records", second)
	}
	_, err = port.read(ctx, "/refactoring/"+oldest, nil)
	require(t, err)
}
