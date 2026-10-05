package steerlineintegration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// Real app construction, product task commands and the built CLI share the
// migrated task application against an isolated committed Git/SQLite project.
func TestTM03TaskWiring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	f := newLifecycleProject(t, "local")
	client, err := steerlineapp.OpenClient(ctx, f.configPath)
	require(t, err)
	defer client.Close()
	batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: f.config.Identity(), Dependencies: []taskmodel.TaskBatchEdge{}}
	for _, id := range []int64{201, 202} {
		contract := taskmodel.TaskContract{ContractID: "task-wiring-" + strconv.FormatInt(id, 10), Repository: f.remoteURL, Remote: "origin", Branch: "main", SpecRef: integrationReference(t, f.repository, f.commit, "docs/spec/integration.spec.md"), PlanRef: integrationReference(t, f.repository, f.commit, "docs/spec/integration.plan.md")}
		contract.AcceptanceRefs = []taskmodel.FileReference{contract.SpecRef}
		contract.Digest, err = taskmodel.ContractDigest(contract)
		require(t, err)
		batch.Tasks = append(batch.Tasks, taskmodel.TaskBatchTask{LocalKey: strconv.FormatInt(id, 10), TaskID: id, Title: "Task wiring", Type: "task", Contract: contract})
	}
	payload := mustJSON(batch)
	reader, err := steerlineapp.OpenNativeTaskReader(ctx, &f.config)
	require(t, err)
	tasks := steerlineapp.NewTaskService(reader, f.config)
	var verifier interface {
		VerifyContractFiles(context.Context, taskmodel.TaskContract) error
	} = tasks
	require(t, verifier.VerifyContractFiles(ctx, batch.Tasks[0].Contract))
	badContract := batch.Tasks[0].Contract
	badContract.SpecRef.SHA256 = strings.Repeat("0", 64)
	badContract.AcceptanceRefs = []taskmodel.FileReference{badContract.SpecRef}
	badContract.Digest, err = taskmodel.ContractDigest(badContract)
	require(t, err)
	if err := verifier.VerifyContractFiles(ctx, badContract); err == nil || !strings.Contains(err.Error(), "committed file SHA-256") {
		t.Fatalf("injected fixed-document capability accepted bad bytes: %v", err)
	}
	require(t, reader.Close())
	validation, err := client.ValidateTaskBatch(ctx, payload)
	require(t, err)
	if !validation.Valid || validation.TaskCount != 2 {
		t.Fatalf("app did not validate task input: %+v", validation)
	}
	in := productapi.CreateInput{Payload: payload, RequestID: strings.Repeat("a", 32), ExpectedAuthorityVersion: validation.AuthorityVersion, RequestedBy: "planner"}
	created, err := client.CreateTaskBatch(ctx, in)
	require(t, err)
	replayed, err := client.CreateTaskBatch(ctx, in)
	require(t, err)
	if created.BatchID != replayed.BatchID || created.GraphRevision != replayed.GraphRevision {
		t.Fatalf("app lost original receipt: %+v / %+v", created, replayed)
	}
	// Inject a failure in the app adapter's audit step, without changing any
	// business rows. The same request can then be retried through the real CLI.
	database, err := sql.Open("sqlite", filepath.Join(f.config.RunDir, "steerline.db"))
	require(t, err)
	defer database.Close()
	_, err = database.ExecContext(ctx, `CREATE TRIGGER fail_task_audit BEFORE INSERT ON operations BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`)
	require(t, err)
	addition := productapi.TaskPredecessorAddInput{TaskID: "202", PredecessorIDs: []string{"201"}, RequestID: strings.Repeat("b", 32), ExpectedTaskVersion: 1, ExpectedGraphRevision: 1, Reason: "task 202 consumes 201", RequestedBy: "planner"}
	if failed, err := client.AddPredecessors(ctx, addition); err == nil || failed.Status != "" {
		t.Fatalf("failed app audit published success: %+v / %v", failed, err)
	}
	before, err := client.TaskGraph(ctx, "", 100)
	require(t, err)
	if before.GraphRevision != 1 || len(before.Edges) != 0 {
		t.Fatalf("failed app audit left task changes: %+v", before)
	}
	_, err = database.ExecContext(ctx, `DROP TRIGGER fail_task_audit`)
	require(t, err)
	command := exec.CommandContext(ctx, buildProductLifecycleCLI(t), "task", "add-predecessors", "-config", f.configPath, "-task-id", "202", "-predecessors", "201", "-request-id", strings.Repeat("b", 32), "-expected-version", "1", "-expected-graph-revision", "1", "-reason", "task 202 consumes 201", "-by", "planner")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI add predecessors: %v\n%s", err, output)
	}
	var receipt productapi.TaskPredecessorAddReceipt
	require(t, json.Unmarshal(output, &receipt))
	if receipt.GraphRevision != 2 || receipt.TaskVersion != 2 || receipt.Status != "open" {
		t.Fatalf("CLI receipt: %+v", receipt)
	}
	page, err := client.ShowTask(ctx, "202")
	require(t, err)
	if len(page.Tasks) != 1 || page.Tasks[0].DependencyReady || len(page.Tasks[0].UnmetPredecessors) != 1 || page.Tasks[0].UnmetPredecessors[0].TaskID != "201" {
		t.Fatalf("CLI dependency not visible in app: %+v", page)
	}
	// The second CLI request is a read-only replay, preserving both versions.
	output, err = exec.CommandContext(ctx, command.Path, command.Args[1:]...).CombinedOutput()
	if err != nil {
		t.Fatalf("CLI replay: %v\n%s", err, output)
	}
	var replay productapi.TaskPredecessorAddReceipt
	require(t, json.Unmarshal(output, &replay))
	if replay.GraphRevision != receipt.GraphRevision || replay.TaskVersion != receipt.TaskVersion {
		t.Fatalf("CLI replay changed revisions: %+v", replay)
	}
	// A malformed pinned digest must be rejected by the real task document port.
	batch.Tasks = batch.Tasks[:1]
	batch.Tasks[0].TaskID, batch.Tasks[0].LocalKey = 203, "203"
	batch.Tasks[0].Contract.ContractID = "task-wiring-203"
	batch.Tasks[0].Contract.SpecRef.SHA256 = strings.Repeat("0", 64)
	batch.Tasks[0].Contract.AcceptanceRefs = []taskmodel.FileReference{batch.Tasks[0].Contract.SpecRef}
	batch.Tasks[0].Contract.Digest, err = taskmodel.ContractDigest(batch.Tasks[0].Contract)
	require(t, err)
	file := filepath.Join(t.TempDir(), "invalid-batch.json")
	require(t, os.WriteFile(file, mustJSON(batch), 0600))
	output, err = exec.CommandContext(ctx, command.Path, "task", "validate", "-config", f.configPath, "-file", file).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "committed file SHA-256 does not match pinned reference") {
		t.Fatalf("CLI accepted malformed fixed files: %v\n%s", err, output)
	}
	graph, err := client.TaskGraph(ctx, "", 100)
	require(t, err)
	if graph.GraphRevision != receipt.GraphRevision || len(graph.Nodes) != 2 {
		t.Fatalf("rejected fixed files mutated the task graph: %+v", graph)
	}
	// A committed link is constructed in Git's index so Windows needs no OS
	// symlink privilege. Keep the original regular-file rejection assertion.
	hash := exec.CommandContext(ctx, "git", "-C", f.repository, "hash-object", "-w", "--stdin")
	hash.Stdin = strings.NewReader("integration.spec.md")
	blob, err := hash.Output()
	require(t, err)
	fixtureGitCommand(t, "-C", f.repository, "update-index", "--add", "--cacheinfo", "120000", strings.TrimSpace(string(blob)), "docs/spec/linked.spec.md")
	fixtureGitCommand(t, "-C", f.repository, "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "task document link fixture")
	head, err := exec.CommandContext(ctx, "git", "-C", f.repository, "rev-parse", "HEAD").Output()
	require(t, err)
	linked := batch.Tasks[0].Contract
	linkDigest := sha256.Sum256([]byte("integration.spec.md"))
	linked.SpecRef = taskmodel.FileReference{Path: "docs/spec/linked.spec.md", Commit: strings.TrimSpace(string(head)), SHA256: hex.EncodeToString(linkDigest[:])}
	linked.AcceptanceRefs = []taskmodel.FileReference{linked.SpecRef}
	linked.Digest, err = taskmodel.ContractDigest(linked)
	require(t, err)
	if err := verifier.VerifyContractFiles(ctx, linked); err == nil || !strings.Contains(err.Error(), "regular committed files") {
		t.Fatalf("committed symlink accepted by task document port: %v", err)
	}
}
