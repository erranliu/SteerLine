package taskrunner

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerbridge"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	task "github.com/erranliu/SteerLine/internal/task/model"
)

type baseStore struct {
	graph      ports.TaskGraphSnapshot
	candidates []runnerbridge.Candidate
}

func (s *baseStore) TaskGraphSnapshot(context.Context, string) (ports.TaskGraphSnapshot, error) {
	return s.graph, nil
}
func (s *baseStore) RunnerCandidates(context.Context, string, string) ([]runnerbridge.Candidate, error) {
	return s.candidates, nil
}
func TestRunnerBaseUsesFetchedTargetAndOriginalCandidateAncestry(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	remote := filepath.Join(root, "remote.git")
	git := func(dir string, args ...string) []byte {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("Git %v: %v %s", args, err, out)
		}
		return out
	}
	out, err := exec.Command("git", "init", "--quiet", "--initial-branch=main", repo).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	commit := func(label string) string {
		git(repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", label)
		return strings.TrimSpace(string(git(repo, "rev-parse", "HEAD")))
	}
	first := commit("base")
	merged := commit("predecessor merge")
	head := commit("current target")
	git(repo, "clone", "--bare", "--quiet", repo, remote)
	git(repo, "remote", "add", "origin", remote)
	store := &baseStore{graph: ports.TaskGraphSnapshot{Authority: task.ProjectTaskAuthority{ProjectIdentity: "p", AuthorityVersion: 1, GraphRevision: 3}, ByID: map[string]task.TaskRecord{"1": {TaskID: "1", Status: "closed"}}, Edges: []task.TaskDependencyRecord{{PredecessorID: "1", SuccessorID: "2"}}, Deliveries: map[string]task.TaskDeliveryRecord{"1": {MergeCommit: merged}}}}
	work := task.FixedWork{Record: task.TaskRecord{ProjectIdentity: "p", TaskID: "2"}, Contract: task.TaskContract{Remote: "origin", Branch: "main", Digest: "contract"}, Authority: store.graph.TaskGraph().Authority}
	calls := 0
	resolver := BaseResolver{Project: "p", Store: store, Git: func(ctx context.Context, args ...string) ([]byte, error) {
		calls++
		return exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	}}
	if base, err := resolver.Resolve(ctx, work, false); err != nil || base != head {
		t.Fatal("fresh target was not fixed", base, err)
	}
	store.candidates = []runnerbridge.Candidate{{ContractDigest: "contract", BaseRevision: merged}}
	if base, err := resolver.Resolve(ctx, work, true); err != nil || base != head {
		t.Fatal("candidate did not use the latest remote base", base, err)
	}
	store.candidates[0].BaseRevision = first
	if base, err := resolver.Resolve(ctx, work, true); err != nil || base != head {
		t.Fatal("old candidate prevented a fresh base")
	}
	store.candidates = append(store.candidates, runnerbridge.Candidate{ContractDigest: "contract", BaseRevision: head})
	if base, err := resolver.Resolve(ctx, work, true); err != nil || base != head {
		t.Fatal("sibling versions prevented latest base")
	}
	store.graph.Authority.GraphRevision++
	before := calls
	if _, err := resolver.Resolve(ctx, work, false); err == nil || calls != before {
		t.Fatal("stale graph fetched or accepted")
	}
	store.graph.Authority.GraphRevision--
	store.candidates = nil
	resolver.Git = func(context.Context, ...string) ([]byte, error) { return nil, errors.New("offline") }
	if _, err := resolver.Resolve(ctx, work, false); err == nil {
		t.Fatal("unacquired target was accepted")
	}
}
