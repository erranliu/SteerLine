package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func seedNativeGraphForView(t *testing.T, c Config, commit string, store *sqliteFixture, count int, edges []TaskDependencyRecord) {
	t.Helper()
	base := nativeContract(t, c, commit, "fixture-contract")
	created := time.Now().UTC()
	mapping := make(map[string]string, count)
	for i := 1; i <= count; i++ {
		id := strconv.Itoa(i)
		mapping["k"+id] = id
	}
	err := store.transactRecords(context.Background(), func(tx *fixtureTransaction) error {
		batch := TaskBatchRecord{ProjectIdentity: c.Identity(), BatchID: "view-fixture", SchemaVersion: taskBatchSchemaVersion, PayloadDigest: strings.Repeat("a", 64), CommandID: "view-fixture-command", TaskIDs: mapping, CreatedAt: created}
		if err := tx.PutTaskBatch(batch, 1, 0); err != nil {
			return err
		}
		for i := 1; i <= count; i++ {
			id := strconv.Itoa(i)
			contract := base
			contract.ContractID = "contract-" + id
			var err error
			contract.Digest, err = TaskContractDigest(contract)
			if err != nil {
				return err
			}
			title := "Task " + id
			if i == 3 {
				title = strings.Repeat("中文标题🙂", 12) + "<script>\x1b[2J\n伪造状态\u202e"
			}
			task := TaskRecord{ProjectIdentity: c.Identity(), TaskID: id, BatchID: batch.BatchID, Title: title, Type: "task", Priority: 2, ContractID: contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
			if err = tx.PutTask(task); err != nil {
				return err
			}
			if err = tx.PutTaskContract(c.Identity(), id, contract); err != nil {
				return err
			}
		}
		for _, edge := range edges {
			if err := tx.PutTaskDependency(edge); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTaskGraphViewSnapshotAndBoundary(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	edges := []TaskDependencyRecord{
		{ProjectIdentity: c.Identity(), PredecessorID: "1", SuccessorID: "2"},
		{ProjectIdentity: c.Identity(), PredecessorID: "2", SuccessorID: "3"},
		{ProjectIdentity: c.Identity(), PredecessorID: "2", SuccessorID: "5"},
		{ProjectIdentity: c.Identity(), PredecessorID: "3", SuccessorID: "4"},
		{ProjectIdentity: c.Identity(), PredecessorID: "5", SuccessorID: "4"},
	}
	seedNativeGraphForView(t, c, commit, store, 6, edges)
	q := NewQueryService(store, c.Identity())
	full, err := q.QueryTaskGraph(ctx, TaskGraphFilter{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	list, err := q.ListNativeTasks(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	show, err := q.ShowNativeTask(ctx, "4")
	if err != nil {
		t.Fatal(err)
	}
	if full.Protocol != nativeTaskGraphProtocol || len(full.Nodes) != 6 || len(full.Edges) != len(edges) || full.Boundary.Truncated {
		t.Fatalf("full graph is incomplete: %+v", full)
	}
	if full.DatabaseID != list.DatabaseID || full.DatabaseRevision != list.DatabaseRevision || full.GraphRevision != list.GraphRevision || full.DatabaseRevision != show.DatabaseRevision {
		t.Fatal("graph, list and detail disagree on the unchanged database snapshot version")
	}
	if full.Edges[0].Predecessor != "1" || full.Edges[0].Successor != "2" || len(show.Tasks[0].UnmetPredecessors) != 2 || full.Nodes[3].TaskID != "4" || len(full.Nodes[3].Blockers) != 2 {
		t.Fatalf("edge direction or blockers differ between graph and detail: %+v / %+v", full, show)
	}
	if full.Nodes[2].Title != list.Tasks[2].Title || !strings.Contains(full.Nodes[2].Title, "中文标题") {
		t.Fatal("graph and list titles differ")
	}
	encoded, err := json.Marshal(full)
	if err != nil || strings.ContainsRune(string(encoded), '\u202e') || !strings.Contains(string(encoded), `\u202e`) || strings.ContainsRune(string(encoded), '\x1b') {
		t.Fatalf("graph JSON allowed terminal controls: %v / %s", err, encoded)
	}
	encodedList, err := json.Marshal(list)
	if err != nil || strings.ContainsRune(string(encodedList), '\u202e') || !strings.Contains(string(encodedList), `\u202e`) {
		t.Fatalf("task list JSON allowed bidi controls: %v / %s", err, encodedList)
	}
	var decodedList NativeTaskPage
	if err = json.Unmarshal(encodedList, &decodedList); err != nil || len(decodedList.Tasks) <= 2 || decodedList.Tasks[2].Title != list.Tasks[2].Title {
		t.Fatalf("terminal-safe JSON changed the source title: %v", err)
	}
	depthOne, err := q.QueryTaskGraph(ctx, TaskGraphFilter{CenterTaskID: "1", Direction: "downstream", Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(depthOne.Nodes) != 2 || depthOne.Boundary.OmittedNodeCount != 3 || !depthOne.Boundary.Truncated || len(depthOne.Boundary.Edges) != 2 {
		t.Fatalf("depth boundary missed the downstream graph: %+v", depthOne.Boundary)
	}
	depthTwo, err := q.QueryTaskGraph(ctx, TaskGraphFilter{CenterTaskID: "1", Direction: "downstream", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(depthTwo.Nodes) != 4 || depthTwo.Boundary.OmittedNodeCount != 1 || len(depthTwo.Boundary.Edges) != 2 {
		t.Fatalf("merge boundary is wrong: %+v", depthTwo.Boundary)
	}
	if _, err = q.QueryTaskGraph(ctx, TaskGraphFilter{CenterTaskID: "1", Depth: 11}); err == nil || !strings.Contains(err.Error(), "invalid_filter") {
		t.Fatalf("excess depth accepted: %v", err)
	}
	if _, err = q.QueryTaskGraph(ctx, TaskGraphFilter{Limit: 501}); err == nil || !strings.Contains(err.Error(), "invalid_filter") {
		t.Fatalf("excess node limit accepted: %v", err)
	}

	discard, err := (CommandService{Store: store, Project: c.Identity()}).DiscardNativeTask(ctx, c, "1", strings.Repeat("d", 32), 1, "superseded", "auditor")
	if err != nil || discard.Status != "discarded" {
		t.Fatalf("discard fixture failed: %+v / %v", discard, err)
	}
	filtered, err := q.QueryTaskGraph(ctx, TaskGraphFilter{Status: "open", CenterTaskID: "2", Direction: "upstream", Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Nodes) != 1 || filtered.Nodes[0].TaskID != "2" || len(filtered.Nodes[0].Blockers) != 1 || filtered.Nodes[0].Blockers[0].Blocker != "discarded_predecessor" || filtered.Boundary.OmittedNodeCount != 1 || len(filtered.Boundary.Edges) != 1 || filtered.Boundary.Edges[0].Reason != "status_filter" {
		t.Fatalf("discarded predecessor became invisible or unblocked: %+v", filtered)
	}
	anchored, err := q.QueryTaskGraph(ctx, TaskGraphFilter{Status: "open", CenterTaskID: "1", Direction: "downstream", Depth: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(anchored.Boundary.Edges) != 1 || anchored.Boundary.Edges[0].Reason != "node_limit" {
		t.Fatalf("a mismatched visible anchor mislabeled its omitted neighbor: %+v", anchored.Boundary)
	}
}

func TestTaskGraphLimitsAndExportPublication(t *testing.T) {
	c, commit, store := openDAGFixture(t)
	seedNativeGraphForView(t, c, commit, store, 502, nil)
	q := NewQueryService(store, c.Identity())
	ctx := context.Background()
	graph, err := q.QueryTaskGraph(ctx, TaskGraphFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || graph.Nodes[0].TaskID != "1" || graph.Nodes[1].TaskID != "2" || graph.Boundary.OmittedNodeCount != 500 || !graph.Boundary.Truncated || len(graph.Edges) != 0 || len(graph.Boundary.Edges) != 0 {
		t.Fatalf("representative graph limit projection is wrong: %+v", graph.Boundary)
	}
	path := filepath.Join(t.TempDir(), "full.json")
	if _, err = WriteTaskGraphExport(path, graph, false); err == nil || !strings.Contains(err.Error(), "export_limit_exceeded") {
		t.Fatalf("truncated full export accepted: %v", err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rejected export created a file: %v", err)
	}
	page, err := q.ListNativeTasks(ctx, "", 500)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := q.ReadyNativeTaskSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := q.ShowNativeTask(ctx, "502")
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasMore || len(page.Tasks) != 500 || len(ready.Tasks) != 502 || ready.Tasks[501].TaskID != "502" || selected.Tasks[0].TaskID != "502" || ready.DatabaseRevision != page.DatabaseRevision || selected.DatabaseRevision != page.DatabaseRevision {
		t.Fatalf("ready candidate beyond the detail page was lost: page=%d ready=%d selected=%+v", len(page.Tasks), len(ready.Tasks), selected)
	}
	filtered, err := q.QueryTaskGraph(ctx, TaskGraphFilter{Status: "open", Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), "filtered.json")
	receipt, err := WriteTaskGraphExport(path, filtered, false)
	if err != nil || receipt.Overwritten || receipt.NodeCount != 500 {
		t.Fatalf("filtered export failed: %+v / %v", receipt, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	t.Logf("filtered export bytes=%d sha256=%x nodes=%d omitted_nodes=%d omitted_boundary_edges=%d", len(body), digest, len(filtered.Nodes), filtered.Boundary.OmittedNodeCount, filtered.Boundary.OmittedBoundaryEdgeCount)
	if strings.ContainsRune(string(body), '\u202e') || !strings.Contains(string(body), `\u202e`) {
		t.Fatal("export JSON exposed a bidi control as terminal text")
	}
	if _, _, err = decodeTaskBatch(body); err == nil {
		t.Fatal("graph export was accepted as task-batch/v1 input")
	}
	var exported TaskGraphPage
	if err = json.Unmarshal(body, &exported); err != nil || exported.Protocol != nativeTaskGraphProtocol || exported.DatabaseID != filtered.DatabaseID || exported.DatabaseRevision != filtered.DatabaseRevision || !exported.Boundary.Truncated || len(exported.Nodes) <= 2 || exported.Nodes[2].Title != filtered.Nodes[2].Title {
		t.Fatalf("export lost version or boundary: %v / %+v", err, exported)
	}
	if _, err = WriteTaskGraphExport(path, filtered, false); err == nil || !strings.Contains(err.Error(), "output_exists") {
		t.Fatalf("existing output was replaced without permission: %v", err)
	}
	still, err := os.ReadFile(path)
	if err != nil || string(still) != string(body) {
		t.Fatalf("failed publication changed existing output: %v", err)
	}
	receipt, err = WriteTaskGraphExport(path, filtered, true)
	if err != nil || !receipt.Overwritten {
		t.Fatalf("explicit replacement failed: %+v / %v", receipt, err)
	}
}

func TestNativeTaskReaderRequiresExistingMatchingAuthority(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	path := filepath.Join(c.RunDir, "steerline.db")
	if reader, err := OpenNativeTaskReader(context.Background(), &c); err == nil {
		_ = reader.Close()
		t.Fatal("read-only reader initialized missing state")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing state was created by a read: %v", err)
	}
	reader, err := OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	changed := c
	changed.Branch = "other"
	if reader, err = OpenNativeTaskReader(context.Background(), &changed); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		if reader != nil {
			_ = reader.Close()
		}
		t.Fatalf("mismatched branch was shown as the current task authority: %v", err)
	}
}

func TestTaskGraphReadCoexistsWithWriter(t *testing.T) {
	c, commit, store := openDAGFixture(t)
	seedNativeGraphForView(t, c, commit, store, 1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reader, err := OpenNativeTaskReader(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	writer, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if _, err = writer.ExecContext(ctx, `UPDATE project_task_authorities SET updated_at=updated_at WHERE project_identity=?`, c.Identity()); err != nil {
		t.Fatal(err)
	}
	graph, err := (NewQueryService(reader, c.Identity())).QueryTaskGraph(ctx, TaskGraphFilter{})
	if err != nil || len(graph.Nodes) != 1 {
		t.Fatalf("read-only graph query waited for a center writer: %v / %+v", err, graph)
	}
	if err = writer.Commit(); err != nil {
		t.Fatalf("graph read prevented writer commit: %v", err)
	}
}
