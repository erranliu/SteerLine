package host

import (
	"context"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func seedNativeGraphForView(t *testing.T, c execution.Config, commit string, store *sqliteFixture, count int, edges []taskmodel.TaskDependencyRecord) {
	t.Helper()
	base := nativeContract(t, c, commit, "fixture-contract")
	created := time.Now().UTC()
	mapping := make(map[string]string, count)
	for i := 1; i <= count; i++ {
		id := strconv.Itoa(i)
		mapping["k"+id] = id
	}
	err := store.transactRecords(context.Background(), func(tx *fixtureTransaction) error {
		batch := taskmodel.TaskBatchRecord{ProjectIdentity: c.Identity(), BatchID: "view-fixture", SchemaVersion: taskmodel.BatchSchemaVersion, PayloadDigest: strings.Repeat("a", 64), CommandID: "view-fixture-command", TaskIDs: mapping, CreatedAt: created}
		if err := tx.PutTaskBatch(batch, 1, 0); err != nil {
			return err
		}
		for i := 1; i <= count; i++ {
			id := strconv.Itoa(i)
			contract := base
			contract.ContractID = "contract-" + id
			var err error
			contract.Digest, err = taskmodel.ContractDigest(contract)
			if err != nil {
				return err
			}
			title := "Task " + id
			if i == 3 {
				title = strings.Repeat("中文标题🙂", 12) + "<script>\x1b[2J\n伪造状态\u202e"
			}
			task := taskmodel.TaskRecord{ProjectIdentity: c.Identity(), TaskID: id, BatchID: batch.BatchID, Title: title, Type: "task", Priority: 2, ContractID: contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
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

func TestNativeTaskReaderRequiresExistingMatchingAuthority(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	path := filepath.Join(c.RunDir, "steerline.db")
	if reader, err := openHostFixtureNativeReader(context.Background(), &c); err == nil {
		_ = reader.Close()
		t.Fatal("read-only reader initialized missing state")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing state was created by a read: %v", err)
	}
	reader, err := openHostFixtureNativeProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	changed := c
	changed.Branch = "other"
	if reader, err = openHostFixtureNativeReader(context.Background(), &changed); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
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
	reader, err := openHostFixtureNativeReader(ctx, &c)
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
	graph, err := (productquery.NewService(reader, c.Identity())).TaskGraph(ctx, "", 100)
	if err != nil || len(graph.Nodes) != 1 {
		t.Fatalf("read-only graph query waited for a center writer: %v / %+v", err, graph)
	}
	if err = writer.Commit(); err != nil {
		t.Fatalf("graph read prevented writer commit: %v", err)
	}
}
