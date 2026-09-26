package steerline

import (
	"context"
	"strings"
	"testing"
)

func TestInterventionProcessorPreservesNativeCommandOwnership(t *testing.T) {
	db := openDomainTestStore(t)
	project := "repo:native-command-ownership"
	service := Service{Config: Config{identity: project}, domainStore: db}
	commands := CommandService{Store: db, Project: project}
	for _, kind := range []string{"claim_task", "discard_task", "create_task_batch", "set_dispatch_state"} {
		id := newID()
		if _, created, err := commands.Submit(context.Background(), CommandRecord{CommandID: id, Kind: kind, TargetID: "task", IdempotencyKey: id, RequestedBy: "operator", Payload: []byte("{}"), Status: "in_flight", CreatedAt: now()}); err != nil || !created {
			t.Fatalf("seed native %s command: created=%v err=%v", kind, created, err)
		}
	}
	serviceID := newID()
	if _, created, err := commands.Submit(context.Background(), CommandRecord{CommandID: serviceID, Kind: "mark_stopped", TargetID: "run", IdempotencyKey: serviceID, RequestedBy: "operator", Payload: []byte("{}"), Status: "pending", CreatedAt: now()}); err != nil || !created {
		t.Fatalf("seed service recovery command: created=%v err=%v", created, err)
	}
	if err := service.processPendingCommands(context.Background()); err != nil {
		t.Fatal(err)
	}
	remaining, err := db.Commands(context.Background(), project, "in_flight", 20)
	require(t, err)
	if len(remaining) != 4 {
		t.Fatalf("intervention processor rewrote center-owned task commands: %+v", remaining)
	}
	processed, err := db.Command(context.Background(), project, serviceID)
	require(t, err)
	if processed.Status != "rejected" {
		t.Fatalf("service recovery command was not checked by the online service: %+v", processed)
	}
}

func TestRecoveryInspectionReadsNativeClaimFromCenter(t *testing.T) {
	project := "repo:native-recovery"
	task := TaskRecord{ProjectIdentity: project, TaskID: "7", Claimant: "native-claimant", Status: "in_progress"}
	service := Service{Config: Config{identity: project}, State: Runtime{Tasks: []TaskRecord{task}}}
	report := RecoveryInspection{}
	service.inspectRecoveryClaim(context.Background(), &Run{TaskID: "7", DomainTaskID: "7"}, &report)
	if report.NativeClaim == nil || report.NativeClaim.TaskID != task.TaskID || report.NativeClaim.Claimant != task.Claimant || report.ClaimError != "" {
		t.Fatalf("native recovery claim lost its center evidence: %+v", report)
	}
}

func TestNativeStartBlockerNeedsCenterCommandEvidence(t *testing.T) {
	db := openDomainTestStore(t)
	service := Service{
		Config:      Config{identity: "repo:native-blocker"},
		State:       Runtime{Control: ControlState{Blockers: []ControlBlocker{{RequestID: "native-start", Action: "start_task"}}}},
		domainStore: db,
	}
	_, err := service.resolveControlBlocker(context.Background(), "native-start", "operator", "checked the center records")
	if err == nil || !strings.Contains(err.Error(), "center command receipt") {
		t.Fatalf("native start blocker without its center command was resolved: %v", err)
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
