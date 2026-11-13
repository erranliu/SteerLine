package app

import (
	"context"
	"encoding/json"
	"errors"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	roadmapmodel "github.com/erranliu/SteerLine/internal/roadmap/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/roadmap"
)

func TestRoadmapStaticInitializationAndPublication(t *testing.T) {
	c, head := nativeTaskFixture(t)
	probe := &startupCountingHost{ManagedProcessHost: c.ProcessHost}
	c.ProcessHost = probe
	c.runtimeMaintenance = newProjectMaintenance(c, nil)
	p := &roadmapPort{config: c, materials: c.runtimeMaintenance}
	ctx := context.Background()
	before := fixtureGit(t, c.Repository, "status", "--porcelain")
	state := p.preparation(ctx)
	if state.Ready || state.LocalReady || len(state.Files) != 3 {
		t.Fatal(state)
	}
	if fixtureGit(t, c.Repository, "status", "--porcelain") != before {
		t.Fatal("GET initialized files")
	}
	in := httpcontract.CommandEnvelope{Action: "roadmap_prepare_static", TargetID: httpcontract.ProjectID(c.Identity()), ExpectedVersion: 1, Payload: json.RawMessage(`{"commit":"` + head + `"}`)}
	for _, bad := range []httpcontract.CommandEnvelope{
		{TargetID: "other", ExpectedVersion: 1, Payload: in.Payload},
		{TargetID: in.TargetID, ExpectedVersion: 0, Payload: in.Payload},
		{TargetID: in.TargetID, ExpectedVersion: 1, Payload: json.RawMessage(`{"commit":"stale"}`)},
		{TargetID: in.TargetID, ExpectedVersion: 1, Payload: json.RawMessage(`{"commit":"` + head + `","repository":"outside"}`)},
	} {
		if _, err := p.prepareStatic(ctx, bad); err == nil {
			t.Fatal("invalid initialization accepted", bad)
		}
	}
	if fixtureGit(t, c.Repository, "status", "--porcelain") != before {
		t.Fatal("rejected input wrote files")
	}
	if _, err := p.action(ctx, httpcontract.CommandEnvelope{Action: "roadmap_parse"}); err == nil || !strings.Contains(err.Error(), "静态材料未就绪") {
		t.Fatal("missing preparation reached parsing", err)
	}
	_, err := p.action(ctx, in)
	require(t, err)
	state = p.preparation(ctx)
	if state.Ready || !state.LocalReady {
		t.Fatal("local files granted fixed readiness", state)
	}
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "initialize static materials")
	if p.preparation(ctx).Ready {
		t.Fatal("unpublished commit granted readiness")
	}
	fixtureGit(t, c.Repository, "update-ref", "refs/remotes/origin/main", "HEAD")
	state = p.preparation(ctx)
	if !state.Ready || state.Commit != state.PublishedCommit {
		t.Fatal("published materials not ready", state)
	}
	probe.calls = 0
	if !p.preparation(ctx).Ready || probe.calls != 2 {
		t.Fatal("poll reread fixed static blobs", probe.calls)
	}
	for _, file := range state.Files {
		if !file.Ready {
			t.Fatal(file)
		}
	}
	// A local invalid platform rule is never overwritten by explicit preparation.
	path := filepath.Join(c.Repository, filepath.FromSlash(roadmap.ParseRulesPath))
	require(t, os.WriteFile(path, []byte("custom invalid rule"), 0600))
	in.Payload, _ = json.Marshal(map[string]string{"commit": state.Commit})
	if _, err := p.action(ctx, in); err == nil {
		t.Fatal("invalid rule overwritten")
	}
	got, err := os.ReadFile(path)
	require(t, err)
	if string(got) != "custom invalid rule" {
		t.Fatal("changed existing rule")
	}
	if _, err := os.Stat(filepath.Join(c.Repository, "roadmap-inputs")); !os.IsNotExist(err) {
		t.Fatal("initialization created dynamic JSON")
	}
}

func TestRoadmapStaticInitializationRejectsGitOperation(t *testing.T) {
	c, head := nativeTaskFixture(t)
	c.runtimeMaintenance = newProjectMaintenance(c, nil)
	p := &roadmapPort{config: c, materials: c.runtimeMaintenance}
	marker := fixtureGit(t, c.Repository, "rev-parse", "--git-path", "MERGE_HEAD")
	if !filepath.IsAbs(marker) {
		marker = filepath.Join(c.Repository, marker)
	}
	require(t, os.WriteFile(marker, []byte(head+"\n"), 0600))
	in := httpcontract.CommandEnvelope{Action: "roadmap_prepare_static", TargetID: httpcontract.ProjectID(c.Identity()), ExpectedVersion: 1, Payload: json.RawMessage(`{"commit":"` + head + `"}`)}
	if _, err := p.action(context.Background(), in); err == nil {
		t.Fatal("initialization during merge accepted")
	}
	if _, err := os.Stat(filepath.Join(c.Repository, filepath.FromSlash(roadmap.ParseRulesPath))); !os.IsNotExist(err) {
		t.Fatal("merge rejection wrote rules")
	}
}

func TestRoadmapInitializationPendingReceiptReconcilesWithoutWriting(t *testing.T) {
	for _, mode := range []string{"missing", "partial", "complete", "head-changed", "git-error", "query-cancelled"} {
		t.Run(mode, func(t *testing.T) {
			c, head := nativeTaskFixture(t)
			ctx := context.Background()
			reader, err := OpenNativeTaskProject(ctx, &c)
			require(t, err)
			require(t, reader.Close())
			store, err := OpenNativeTaskCommandStore(ctx, &c)
			require(t, err)
			defer store.Close()
			c.runtimeMaintenance = newProjectMaintenance(c, store)
			port := &roadmapPort{config: c, materials: c.runtimeMaintenance, store: store}
			c.runtimeRoadmaps = port
			runtime := &runtimeProjectPort{config: c, store: store}
			if mode == "complete" || mode == "head-changed" || mode == "git-error" || mode == "query-cancelled" {
				_, err = roadmap.PrepareStatic(c.Repository, false)
				require(t, err)
			}
			if mode == "partial" {
				path := filepath.Join(c.Repository, filepath.FromSlash(roadmap.ParseRulesPath))
				require(t, os.MkdirAll(filepath.Dir(path), 0700))
				require(t, os.WriteFile(path, []byte("partial"), 0600))
			}
			if mode == "head-changed" {
				fixtureGit(t, c.Repository, "add", ".")
				fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "changed")
			}
			in := httpcontract.CommandEnvelope{RequestID: strings.Repeat("a", 32), Action: "roadmap_prepare_static", ProjectID: httpcontract.ProjectID(c.Identity()), InstanceID: "original", TargetID: httpcontract.ProjectID(c.Identity()), ExpectedVersion: 1, Payload: json.RawMessage(`{"commit":"` + head + `"}`)}
			raw, _ := json.Marshal(in)
			_, _, err = store.EnqueueCommand(ctx, storageports.CommandRecord{ProjectIdentity: c.Identity(), CommandID: in.RequestID, Kind: "roadmap_command", TargetID: in.TargetID, ExpectedVersion: 1, IdempotencyKey: in.RequestID, RequestedBy: "local-user", Payload: raw, Status: "pending", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
			require(t, err)
			before := fixtureGit(t, c.Repository, "status", "--porcelain")
			if mode == "git-error" || mode == "query-cancelled" {
				originalHost := c.runtimeMaintenance.config.ProcessHost
				queryCtx, cancel := context.WithCancel(ctx)
				cause := os.ErrPermission
				if mode == "query-cancelled" {
					c.runtimeMaintenance.config.ProcessHost = cancelInitializationQueryHost{ManagedProcessHost: originalHost, cancel: cancel}
					cause = context.Canceled
				} else {
					c.runtimeMaintenance.config.ProcessHost = failingDocumentHost{ManagedProcessHost: originalHost, stage: "rev-parse", cause: cause}
				}
				if _, err := runtime.roadmapReceipt(queryCtx, in.RequestID); !errors.Is(err, cause) {
					t.Fatal("query error classified as terminal initialization", err)
				}
				cancel()
				saved, _, err := store.LookupCommand(ctx, c.Identity(), in.RequestID)
				require(t, err)
				if saved.Status != "pending" || len(saved.Result) != 0 {
					t.Fatal("transient read ended original request", saved)
				}
				c.runtimeMaintenance.config.ProcessHost = originalHost
			}
			out, err := runtime.roadmapReceipt(ctx, in.RequestID)
			require(t, err)
			want := "rejected"
			if mode == "complete" || mode == "git-error" || mode == "query-cancelled" {
				want = "applied"
			}
			if out == nil || out.Status != want {
				t.Fatal("pending init not settled from read-only facts", out, want)
			}
			saved, found, err := store.LookupCommand(ctx, c.Identity(), in.RequestID)
			require(t, err)
			if !found || saved.Status != want {
				t.Fatal("reconciled receipt not durable", saved)
			}
			again, err := runtime.roadmapReceipt(ctx, in.RequestID)
			require(t, err)
			if again.Status != want || fixtureGit(t, c.Repository, "status", "--porcelain") != before {
				t.Fatal("query repeated filesystem initialization", again)
			}
			if mode == "missing" {
				if _, err = os.Stat(filepath.Join(c.Repository, filepath.FromSlash(roadmap.ParseRulesPath))); !os.IsNotExist(err) {
					t.Fatal("query created missing file")
				}
			}
			graph, err := store.TaskGraphSnapshot(ctx, c.Identity())
			require(t, err)
			if len(graph.Tasks) != 0 {
				t.Fatal("initialization created tasks")
			}
		})
	}
}

func TestRoadmapPublishedGuideMustBeValidUTF8AndBounded(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	ctx := context.Background()
	c.runtimeMaintenance = newProjectMaintenance(c, nil)
	p := &roadmapPort{config: c, materials: c.runtimeMaintenance}
	_, err := roadmap.PrepareStatic(c.Repository, false)
	require(t, err)
	for _, body := range [][]byte{{0xff}, []byte(strings.Repeat("x", roadmapmodel.MaxBytes+1))} {
		require(t, os.WriteFile(filepath.Join(c.Repository, "SteerlineFlow/spec/SpecGuide.md"), body, 0600))
		fixtureGit(t, c.Repository, "add", ".")
		fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "invalid guide")
		fixtureGit(t, c.Repository, "update-ref", "refs/remotes/origin/main", "HEAD")
		if state := p.preparation(ctx); state.Ready {
			t.Fatal("invalid published guide ready", state)
		}
	}
}

type cancelInitializationQueryHost struct {
	backend.ManagedProcessHost
	cancel context.CancelFunc
}

func (h cancelInitializationQueryHost) StartManaged(command *exec.Cmd) (backend.StopHandle, error) {
	h.cancel()
	return nil, context.Canceled
}
