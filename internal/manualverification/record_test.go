package manualverification

import (
	"strings"
	"testing"
)

func TestManualVerificationCloseRequiresAllChecksOnSelectedSnapshot(t *testing.T) {
	head, old := strings.Repeat("a", 40), strings.Repeat("b", 40)
	record := Record{Head: head, Merged: true}
	record.Definition.Definition.Checks = []Check{{ID: "M01"}, {ID: "M02"}}
	record.Results = []Result{
		{Source: SourcePR, Head: head, CheckID: "M01", Outcome: "passed"},
		{Source: SourceRemote, Head: old, CheckID: "M01", Outcome: "passed"},
		{Source: SourceRemote, Head: head, CheckID: "M02", Outcome: "failed"},
	}
	closure := Closure{Source: SourceRemote, Head: head}
	if err := record.Close(closure); err == nil || record.Closure != nil {
		t.Fatal("closure reused a result from another source or commit")
	}
	record.Results = append(record.Results, Result{Source: SourceRemote, Head: head, CheckID: "M01", Outcome: "passed"})
	if err := record.Close(closure); err != nil {
		t.Fatal(err)
	}
	if record.Closure == nil || *record.Closure != closure || record.Allows(SourcePR, head) || record.Allows(SourceRemote, head) {
		t.Fatal("the closure did not close the verification to-do")
	}
}
