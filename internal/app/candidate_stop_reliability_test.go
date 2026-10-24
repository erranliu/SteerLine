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

	"github.com/erranliu/SteerLine/internal/app/host"
	api "github.com/erranliu/SteerLine/internal/product/api"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func TestCandidateStopReliabilityRecoversMissingReceipt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := Config{identity: "project", RunDir: dir}
	c.runtimeHost = &host.Config{RepositoryIdentity: c.Identity(), RunDir: dir, TaskAuthority: "native", Repository: "repository", Remote: "origin", Branch: "main", ControlValidation: applicationControlValidation{}}
	store, err := sqlite.OpenSQLiteStore(filepath.Join(dir, "steerline.db"))
	require(t, err)
	defer store.Close()
	input := api.ControlInput{Action: api.StopCandidate, RequestID: strings.Repeat("e", 32), InstanceID: "original", RequestedBy: "operator", ExpectedRevision: new(uint64), Payload: api.ControlPayload{TaskID: "18", RunID: strings.Repeat("b", 32), ExpectedRunRevision: 3}}
	request, err := host.SubmitControlInbox(*c.runtimeHost, input)
	require(t, err)
	now := time.Now().UTC()
	saved := api.ControlReceipt{Version: 1, RequestID: request.RequestID, RequestDigest: taskrunner.RequestDigest(request), InstanceID: "original", Action: "stop_candidate", Status: "rejected", Code: "last_candidate", CompletedAt: &now}
	body, _ := json.Marshal(request)
	result, _ := json.Marshal(saved)
	_, _, err = store.EnqueueCommand(ctx, ports.CommandRecord{ProjectIdentity: c.Identity(), CommandID: request.RequestID, IdempotencyKey: request.RequestID, Kind: "runner_control", TargetID: c.Identity(), RequestedBy: "operator", Payload: body, Status: "rejected", Result: result, CreatedAt: now})
	require(t, err)
	// There is no host receipt file. Query recovers the original durable result.
	control := clientControl{config: c, store: store}
	receipt, err := control.Receipt(ctx, request.RequestID)
	require(t, err)
	if receipt.Status != "rejected" || receipt.Code != "last_candidate" || receipt.RequestID != input.RequestID {
		t.Fatal("lost original result", receipt)
	}
	command, found, err := store.LookupCommand(ctx, c.Identity(), request.RequestID)
	require(t, err)
	if !found {
		t.Fatal("original command missing")
	}
	for _, field := range []string{"version", "status"} {
		t.Run("invalid_"+field, func(t *testing.T) {
			invalid := saved
			if field == "version" {
				invalid.Version++
			} else {
				invalid.Status = "unexpected"
			}
			command.Result, _ = json.Marshal(invalid)
			require(t, store.UpdateCommand(ctx, command))
			if _, err := control.Receipt(ctx, request.RequestID); err == nil {
				t.Fatal("invalid durable result accepted")
			}
		})
	}
	command.Result = result
	require(t, store.UpdateCommand(ctx, command))
	ordinary := api.ControlInput{Action: api.PauseDispatch, RequestID: strings.Repeat("a", 32), InstanceID: "original", RequestedBy: "operator", ExpectedRevision: new(uint64)}
	_, err = host.SubmitControlInbox(*c.runtimeHost, ordinary)
	require(t, err)
	if _, err = control.Receipt(ctx, ordinary.RequestID); !os.IsNotExist(err) {
		t.Fatal("ordinary request without a receipt must remain submitted", err)
	}
	// A malformed outer receipt is not treated as absence and bypassed.
	receiptDir := filepath.Join(dir, "control", "receipts")
	require(t, os.MkdirAll(receiptDir, 0700))
	require(t, os.WriteFile(filepath.Join(receiptDir, input.RequestID+".json"), []byte(`{}`), 0600))
	if _, err = control.Receipt(ctx, input.RequestID); err == nil {
		t.Fatal("invalid outer receipt bypassed")
	}
	_, digest, err := host.ReadControlRequestWithDigest(*c.runtimeHost, input.RequestID)
	require(t, err)
	saved.RequestDigest = fmt.Sprintf("%x", digest)
	require(t, host.NewServiceHost(*c.runtimeHost, "off", nil, nil).WriteControlReceipt(saved))
	wrong := c
	wrongHost := *c.runtimeHost
	wrongHost.RepositoryIdentity = "other-project"
	wrong.identity, wrong.runtimeHost = "other-project", &wrongHost
	if _, err = (clientControl{config: wrong, store: store}).Receipt(ctx, input.RequestID); err == nil {
		t.Fatal("valid receipt for another project accepted")
	}
}
