//go:build runneracceptance

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/backend"
	em "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/refactoring"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	rc "github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// This explicit live diagnostic starts at review for an existing PR. It never
// imports a database, adopts an old Run, writes the repository or publishes.
func TestRefactoringPinnedSourceReviewLive(t *testing.T) {
	var input struct {
		Root, Repository, RemoteURL, PRURL, BaseBranch, HeadBranch string
		Head, Base, DirectionPrompt, NativeExecutable              string
		Runner                                                     runnerclient.Selection
		SourcePaths                                                []string
	}
	raw, err := os.ReadFile(os.Getenv("STEERLINE_REVIEW_LIVE_INPUT"))
	require(t, err)
	require(t, json.Unmarshal(raw, &input))
	root, err := filepath.Abs(input.Root)
	require(t, err)
	if !strings.HasPrefix(strings.ToLower(root), `c:\steerlinedebug\`) ||
		input.PRURL != "http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/463" ||
		!applicationPathInside(root, input.Repository) ||
		!ref.ValidRevision(input.Head) || !ref.ValidRevision(input.Base) || len(input.SourcePaths) == 0 {
		t.Fatal("explicit isolated PR #463 fixture required")
	}
	require(t, discardPlainPath(root))
	if _, err := os.Stat(filepath.Join(root, "state")); !os.IsNotExist(err) {
		t.Fatal("fresh Runner state required")
	}
	home := filepath.Join(root, "home")
	if !strings.EqualFold(filepath.Clean(os.Getenv("CODEX_HOME")), filepath.Join(home, "native-auth")) {
		t.Fatal("isolated native authentication home required")
	}
	t.Setenv("STEERLINE_HOME", home)
	write := func(name string, value any) {
		t.Helper()
		body, err := json.MarshalIndent(value, "", "  ")
		require(t, err)
		require(t, os.WriteFile(filepath.Join(root, name), body, 0600))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	var cli runnerclient.Client
	var version runnerclient.Version
	var info runnerclient.ServiceInfo
	startupAttempted := false
	var service *refactoring.Service
	var store *sqlite.SQLiteStore
	var actionID string
	var result map[string]any
	exampleSource, err := investigationPath(ref.ID())
	require(t, err)
	t.Cleanup(func() {
		stopCtx, done := context.WithTimeout(context.Background(), 50*time.Second)
		defer done()
		var cleanupErr error
		if service != nil && actionID != "" {
			cleanupErr = errors.Join(cleanupErr, service.StopForAbandonment(stopCtx, actionID))
			for stopCtx.Err() == nil {
				tickErr := service.Tick(stopCtx)
				current, readErr := store.Refactoring(stopCtx, service.Project, actionID)
				cleanupErr = errors.Join(cleanupErr, tickErr, readErr)
				if tickErr != nil || readErr != nil {
					break
				}
				pendingSource := false
				for _, op := range current.Operations {
					pendingSource = pendingSource || op.Investigation != ""
				}
				if !current.Unsettled() && !pendingSource {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			cleanupErr = errors.Join(cleanupErr, stopCtx.Err())
		}
		requestID := ref.ID()
		runnerSettled := !startupAttempted
		if startupAttempted {
			// Reconcile a service that started even if its start response failed.
			status, statusErr := cli.ServiceStatus(stopCtx)
			if statusErr == nil && status.Status == "ready" && status.InstanceID != "" &&
				strings.EqualFold(filepath.Clean(status.StateDir), filepath.Join(root, "state")) &&
				strings.EqualFold(status.ExecutableSHA256, input.Runner.SHA256) &&
				(info.InstanceID == "" || info.InstanceID == status.InstanceID) {
				info = status
				cli.InstanceID = info.InstanceID
				stopErr := cli.StopService(stopCtx, info.InstanceID, requestID)
				settledErr := cli.VerifyServiceSettlement(stopCtx, info.InstanceID, requestID)
				cleanupErr = errors.Join(cleanupErr, stopErr, settledErr)
				runnerSettled = stopErr == nil && settledErr == nil
			} else {
				cleanupErr = errors.Join(cleanupErr, statusErr, errors.New("live review service identity could not be reconciled"))
			}
		}
		if service != nil && actionID != "" {
			current, readErr := store.Refactoring(stopCtx, service.Project, actionID)
			cleanupErr = errors.Join(cleanupErr, readErr)
			for _, op := range current.Operations {
				if op.Investigation != "" {
					cleanupErr = errors.Join(cleanupErr, errors.New("review source still registered"))
				}
			}
		}
		entries, sourceErr := os.ReadDir(filepath.Dir(exampleSource))
		if sourceErr != nil && !os.IsNotExist(sourceErr) {
			cleanupErr = errors.Join(cleanupErr, sourceErr)
		} else if len(entries) != 0 {
			cleanupErr = errors.Join(cleanupErr, errors.New("review source directories remain"))
		}
		authRemoved := false
		if runnerSettled {
			authErr := os.Remove(filepath.Join(home, "native-auth", "auth.json"))
			authRemoved = authErr == nil || os.IsNotExist(authErr)
			if !authRemoved {
				cleanupErr = errors.Join(cleanupErr, authErr)
			}
		}
		if store != nil {
			cleanupErr = errors.Join(cleanupErr, store.Close())
		}
		if cleanupErr != nil {
			t.Error("live review cleanup", cleanupErr)
		}
		verified := cleanupErr == nil && runnerSettled && authRemoved
		write("cleanup.json", map[string]any{"verified": verified, "runner_settled": runnerSettled, "auth_removed": authRemoved, "error": errorText(cleanupErr)})
		if result != nil {
			result["passed"] = !t.Failed() && verified
			write("result.json", result)
		}
	})
	cli, version, err = input.Runner.Open(ctx, filepath.Join(root, "state"), 30*time.Second)
	require(t, err)
	cli.UsagePolicy = json.RawMessage(`{"enabled":false}`)
	startupAttempted = true
	info, err = cli.StartService(ctx, version)
	require(t, err)
	cli.InstanceID = info.InstanceID
	catalog, err := cli.Catalog(ctx, version)
	require(t, err)
	cli = cli.WithCatalog(catalog)
	c := Config{Repository: input.Repository, Remote: "origin", Branch: input.BaseBranch,
		Git: backend.Command{Executable: "git"}, CommandTimeout: "2m", ProcessHost: NewProcessHost(2 * time.Minute),
		PR: em.PRProviderConfig{Kind: "gitea", BaseURL: "http://192.168.0.66:3000", Repository: "zymobile/merge-h-308-code", TokenEnv: "STEERLINE_GITEA_TOKEN"}}
	require(t, c.ResolveIdentity(ctx))
	port := refactoringPort{config: c}
	db := filepath.Join(root, "review.sqlite")
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatal("fresh review store required")
	}
	store, err = sqlite.OpenSQLiteStore(db)
	require(t, err)
	require(t, store.InitializeProject(ctx, taskmodel.ProjectTaskAuthority{ProjectIdentity: c.Identity(), Authority: "native", AuthorityVersion: 1, Repository: input.RemoteURL, Remote: "origin", Branch: input.BaseBranch, DispatchMode: "manual"}))
	settings, err := json.Marshal(map[string]string{"access_mode": "read_only", "executable": input.NativeExecutable, "model": "gpt-6-luna", "reasoning_effort": "medium"})
	require(t, err)
	now := time.Now().UTC()
	actionID = ref.ID()
	record := ref.Record{ID: actionID, Project: c.Identity(), Revision: 1, Phase: "report", Slot: true, Authorized: true,
		CreatedAt: now, UpdatedAt: now, ActiveSince: &now, EnqueuedAt: now,
		Input: ref.Start{Strategy: "cautious"},
		Frozen: ref.Frozen{Direction: ref.Direction{Prompt: input.DirectionPrompt}, Runner: cli.BuildPin(),
			Repository:  rc.Repository{URL: input.RemoteURL, LocalPath: input.Repository, Remote: "origin", BaseBranch: input.BaseBranch, BaseRevision: input.Base},
			Publication: map[string]string{"repository": c.PR.Repository, "base_url": c.PR.BaseURL},
			Read:        rc.ExecutionInput{Adapter: "codex-app-server", Target: rc.ExecutionTarget{Kind: "local"}, Settings: settings}},
		PR: &rc.PRFact{URL: input.PRURL, HeadBranch: input.HeadBranch, HeadRevision: input.Head, BaseRevision: input.Base},
		// Only the original branch binding is supplied to the existing verifier.
		// There is no historical native result or claimed validation evidence.
		Operations: []ref.Operation{{ID: ref.ID(), Purpose: "existing_pr_binding", State: "cancelled", Run: &rc.Run{Execution: rc.Execution{Branch: input.HeadBranch}}}}}
	verifiedPR, _, err := port.verify(ctx, record, ref.Result{PR: input.PRURL})
	require(t, err)
	if verifiedPR.HeadRevision != input.Head || verifiedPR.BaseRevision != input.Base || verifiedPR.State != "open" || verifiedPR.Merged {
		t.Fatal("PR changed before live review")
	}
	record.PR = verifiedPR
	write("initial-record.json", record)
	require(t, store.SaveRefactoring(ctx, record, 0, nil))
	service = &refactoring.Service{Project: c.Identity(), Store: store, Runner: cli,
		Verify: port.verify, ReviewDiff: port.reviewDiff, InvestigationPath: investigationPath,
		Investigate: port.investigation, ReleaseInvestigation: releaseInvestigation}
	sourceSaved := false
	for ctx.Err() == nil {
		require(t, service.Tick(ctx))
		record, err = store.Refactoring(ctx, c.Identity(), actionID)
		require(t, err)
		write("latest-record.json", record)
		for _, op := range record.Operations {
			if op.Purpose != "report" {
				continue
			}
			var request rc.GenerateRunInput
			require(t, json.Unmarshal(op.Request.Input, &request))
			if request.Workflow.ID != "generate" || request.Generate.Directory == nil || request.Generate.Directory.Access != "read_only" || op.PRVersion != input.Head+":"+input.Base {
				t.Fatal("unexpected review access or version")
			}
			write("report-request.json", op.Request)
			if !sourceSaved && op.Investigation != "" {
				if _, statErr := os.Stat(filepath.Join(op.Investigation, ".git")); statErr == nil {
					out, err := (commandRunner{Dir: op.Investigation, ProcessHost: c.ProcessHost, Timeout: 30 * time.Second}).Output(ctx, c.Git, "--no-replace-objects", "rev-parse", "HEAD")
					require(t, err)
					if strings.TrimSpace(string(out)) != input.Head {
						t.Fatal("live source is not PR head")
					}
					files := map[string]string{}
					for _, path := range input.SourcePaths {
						if !filepath.IsLocal(filepath.FromSlash(path)) {
							t.Fatal("unsafe source check path")
						}
						body, err := os.ReadFile(filepath.Join(op.Investigation, filepath.FromSlash(path)))
						require(t, err)
						digest := sha256.Sum256(body)
						files[path] = hex.EncodeToString(digest[:])
					}
					write("source-proof.json", map[string]any{"head": input.Head, "base": input.Base, "directory": op.Investigation, "files": files})
					sourceSaved = true
				}
			}
		}
		if record.Phase == "blocked" || record.Phase == "stopped" {
			t.Fatal("live review failed", record.Phase, record.Diagnostic)
		}
		if record.Phase == "delivered" {
			if !sourceSaved || len(record.Reports) != 1 || record.Reports[0].Head != input.Head || record.Reports[0].Base != input.Base {
				t.Fatal("missing fixed source or report binding")
			}
			report := record.Reports[0]
			cited := false
			for _, path := range input.SourcePaths {
				cited = cited || strings.Contains(report.Body, filepath.Base(path))
			}
			if !cited {
				t.Fatal("review did not cite any supplied source file", report.Body)
			}
			result = map[string]any{"scope": "existing PR read-only review only", "report": report}
			return
		}
		select {
		case <-ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Fatal(ctx.Err())
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
