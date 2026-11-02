package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

// All assertions use one real Git/SQLite project. Each new detection still gets
// its own activity and CAS revision; repeated fixture creation adds no coverage.
func TestSharedEnvironmentMaterialsAndIndependentQualifications(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	commit, err := p.source(ctx)
	require(t, err)
	local := c.RunnerExecution.Profiles[0].ID
	seed := func(t *testing.T, profile string) (env.Record, env.Current) {
		t.Helper()
		prepared, err := p.materialAt(ctx, profile, commit)
		require(t, err)
		current, _, err := p.current(ctx, profile)
		require(t, err)
		r := env.Record{ID: env.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: profile, Execution: prepared.Input.Execution, Prepared: prepared, State: "ready", CreatedAt: time.Now().UTC(), Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1, Settled: true}}}
		require(t, store.StartEnvironment(ctx, r, current.Revision))
		current, err = store.CurrentEnvironment(ctx, r.Project, mustEnvironmentKey(t, r.Execution))
		require(t, err)
		return r, current
	}
	a, currentA := seed(t, local)
	if _, _, err := p.admission(ctx, "remote", "", false); err == nil {
		t.Fatal("local success admitted an untested remote chain")
	}
	b, _ := seed(t, "remote")
	if a.Prepared.Inputs == b.Prepared.Inputs || mustEnvironmentKey(t, a.Execution) == mustEnvironmentKey(t, b.Execution) {
		t.Fatal("different chains shared qualification identity")
	}
	for _, r := range []env.Record{a, b} {
		if r.Prepared.Input.Documents.Spec.Path != env.DocumentPath() || r.Prepared.Input.Documents.Plan.Path != env.ScriptPath("ps1") {
			t.Fatal("request did not use common paths", r.Prepared.Input.Documents)
		}
		value, err := p.materials(ctx, mustEnvironmentKey(t, r.Execution))
		require(t, err)
		material := value.(map[string]any)
		if material["document_path"] != env.DocumentPath() || material["script_path"] != env.ScriptPath("ps1") || material["document"] != environmentFixtureDocument("ps1", []string{"go.mod"}) {
			t.Fatal("selected target did not receive common material", material)
		}
	}
	if _, err := p.materials(ctx, strings.Repeat("f", 64)); err == nil {
		t.Fatal("unknown target acquired shared material endpoint")
	}
	for _, change := range []string{"failed", "invalidate"} {
		t.Run(change, func(t *testing.T) {
			if change == "invalidate" {
				a, currentA = seed(t, local)
				require(t, store.RevokeEnvironmentBasis(ctx, a.Project, mustEnvironmentKey(t, a.Execution), a.ID, "explicit target revocation", currentA.Revision))
			} else {
				failed := a
				failed.ID, failed.Revision, failed.State, failed.CreatedAt = env.ID(), 1, "failed", time.Now().UTC()
				failed.Requests = []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}
				require(t, store.StartEnvironment(ctx, failed, currentA.Revision))
			}
			if _, _, err := p.admission(ctx, local, "", false); err == nil {
				t.Fatal("revoked or failed chain admitted")
			}
			if _, _, err := p.admission(ctx, "remote", "", false); err != nil {
				t.Fatal("one chain's failure revoked another", err)
			}
		})
	}
	for _, path := range []string{env.DocumentPath(), env.ScriptPath("ps1"), "go.mod"} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			a, beforeA := seed(t, local)
			b, beforeB := seed(t, "remote")
			full := filepath.Join(c.Repository, filepath.FromSlash(path))
			original, err := os.ReadFile(full)
			require(t, err)
			require(t, os.WriteFile(full, append(original, []byte("\n# changed common basis\n")...), 0600))
			commit = commitEnvironmentMaterials(t, c)
			require(t, p.observeBasis(ctx, commit))
			for i, r := range []env.Record{a, b} {
				current, err := store.CurrentEnvironment(ctx, r.Project, mustEnvironmentKey(t, r.Execution))
				require(t, err)
				if !current.Invalidated || current.Revision != []uint64{beforeA.Revision, beforeB.Revision}[i]+1 {
					t.Fatal("common change did not durably revoke each chain", current)
				}
			}
			require(t, os.WriteFile(full, original, 0600))
			commit = commitEnvironmentMaterials(t, c)
			require(t, p.observeBasis(ctx, commit))
			for _, profile := range []string{local, "remote"} {
				if _, _, err := p.admission(ctx, profile, "", false); err == nil {
					t.Fatal("rollback revived old qualification", profile)
				}
			}
		})
	}
	t.Run("no-keyed-fallback", func(t *testing.T) {
		legacy := filepath.Join(c.Repository, "SteerlineFlow", "environments", mustEnvironmentKey(t, b.Execution), "environment.md")
		require(t, os.MkdirAll(filepath.Dir(legacy), 0700))
		require(t, os.WriteFile(legacy, []byte(environmentFixtureDocument("ps1", []string{})), 0600))
		require(t, os.Remove(filepath.Join(c.Repository, filepath.FromSlash(env.DocumentPath()))))
		commit = commitEnvironmentMaterials(t, c)
		if _, err := p.materialAt(ctx, "remote", commit); err == nil || !strings.Contains(err.Error(), env.DocumentPath()) {
			t.Fatal("missing common document fell back to keyed material", err)
		}
	})
}

// Invoked only when a copied test binary acts as a version-query tool.
func runEnvironmentToolFixture() bool {
	mode := os.Getenv("STEERLINE_TEST_ENVIRONMENT_TOOL")
	if mode == "" {
		return false
	}
	name := strings.ToLower(filepath.Base(os.Args[0]))
	versions := map[string]string{"git.exe": "git version 2.50.0", "go.exe": "go version go1.26.4 windows/amd64", "node.exe": "v24.4.0", "python.exe": "Python 3.13.0"}
	text, ok := versions[name]
	argument := "--version"
	if name == "go.exe" {
		argument = "version"
	}
	if !ok || len(os.Args) != 2 || os.Args[1] != argument {
		os.Exit(12)
	}
	if mode == "error" && name == "node.exe" {
		fmt.Println("untrusted tool output must not leak")
		os.Exit(9)
	}
	if mode == "timeout" && name == "node.exe" {
		time.Sleep(30 * time.Second)
	}
	if mode == "low" && name == "node.exe" {
		text = "v20.1.0"
	}
	fmt.Println(text)
	return true
}
