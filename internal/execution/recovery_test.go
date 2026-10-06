package execution

import (
	"context"
	"strings"
	"testing"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"

	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestRecoveryInspectionReadsNativeClaimFromCenter(t *testing.T) {
	project := "repo:native-recovery"
	task := taskmodel.TaskRecord{ProjectIdentity: project, TaskID: "7", Claimant: "native-claimant", Status: "in_progress"}
	service := Service{Config: Config{identity: project}, State: reconstruction.Runtime{}, domain: reconstruction.Snapshot{Tasks: []taskmodel.TaskRecord{task}}}
	report := RecoveryInspection{}
	service.inspectRecoveryClaim(context.Background(), &reconstruction.RunView{TaskID: "7", DomainTaskID: "7"}, &report)
	if report.NativeClaim == nil || report.NativeClaim.TaskID != task.TaskID || report.NativeClaim.Claimant != task.Claimant || report.ClaimError != "" {
		t.Fatalf("native recovery claim lost its center evidence: %+v", report)
	}
}

func TestRecoveryConfirmationBasisIsBoundedAndPreserved(t *testing.T) {
	basis := strings.Repeat("x", 2001)
	if _, err := validateConfirmationBasis(basis); err == nil || !strings.Contains(err.Error(), "2000 character") {
		t.Fatalf("oversized confirmation was accepted: %v", err)
	}
	if len([]rune(basis)) != 2001 {
		t.Fatal("confirmation input was modified")
	}
}
