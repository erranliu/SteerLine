package testenv

import (
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestApplicationProjectSnapshotRejectsDifferentDatabaseRevisions(t *testing.T) {
	sampledAt := time.Now().UTC()
	read := ports.ApplicationProjectSnapshot{
		ProjectView: ports.ProjectView{
			SampledAt: sampledAt,
			Tasks:     &ports.NativeTaskPage{ProjectID: "project", AuthorityVersion: 7, GraphRevision: 9, DatabaseID: "db", DatabaseRevision: 11, SampledAt: sampledAt},
			Ready:     &ports.NativeReadyTaskSnapshot{ProjectID: "project", AuthorityVersion: 7, GraphRevision: 9, DatabaseID: "db", DatabaseRevision: 11, SampledAt: sampledAt},
		},
		ExecutionState: ports.ExecutionStateSnapshot{
			Graph:   ports.TaskGraphSnapshot{Authority: executionmodel.ProjectTaskAuthority{ProjectIdentity: "project", AuthorityVersion: 7, GraphRevision: 9}, DatabaseID: "db", DatabaseRevision: 11},
			Records: ports.ExecutionSnapshot{ProjectIdentity: "project"},
		},
	}
	snapshot, err := snapshotFromApplicationRead(read)
	if err != nil {
		t.Fatalf("combined storage snapshot was rejected: %v", err)
	}
	if snapshot.Page.DatabaseID != read.ExecutionState.Graph.DatabaseID || snapshot.Page.DatabaseRevision != read.ExecutionState.Graph.DatabaseRevision || snapshot.Domain.ProjectIdentity != "project" {
		t.Fatalf("testenv separated product and execution facts: page=%+v domain=%+v", snapshot.Page, snapshot.Domain)
	}
	read.ExecutionState.Graph.DatabaseRevision++
	if _, err := snapshotFromApplicationRead(read); err == nil {
		t.Fatal("testenv accepted task and execution values from different database revisions")
	}
}
