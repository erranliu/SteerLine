package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
)

func TestAPIControlRequestFaultRecovery(t *testing.T) {
	requireWindowsAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	service := startIntegrationService(t, ctx, fixture.config, &integrationLifecycleResolver{})
	defer service.stop(t)
	endpoint, client, product, driver := startIntegrationHTTPAPIWithInboxInterceptor(t, ctx, fixture, service)
	defer func() {
		if endpoint != nil {
			_ = endpoint.Close(context.Background())
		}
		if client != nil {
			_ = client.Close()
		}
	}()

	initialRevision, initialPaused := integrationControlState(t, ctx, client)

	// The client exits after the immutable HTTP envelope and mapped intent are
	// durable, but before the formal host request is published.
	preSend := integrationDispatchEnvelope(t, client, initialRevision, initialPaused)
	preEntered := make(chan api.ControlInput, 1)
	driver.SetHook(func(requestCtx context.Context, input api.ControlInput, _ integrationInboxPublisher) (api.ControlRequest, error) {
		if input.RequestID != preSend.RequestID {
			return driver.publish(requestCtx, input)
		}
		preEntered <- input
		<-requestCtx.Done()
		return api.ControlRequest{}, requestCtx.Err()
	})
	preContext, cancelPre := context.WithCancel(ctx)
	preDone := make(chan error, 1)
	go func() {
		_, submitErr := client.SubmitCommand(preContext, preSend)
		preDone <- submitErr
	}()
	var preMapped api.ControlInput
	select {
	case preMapped = <-preEntered:
	case <-time.After(15 * time.Second):
		cancelPre()
		t.Fatal("HTTP request did not reach the controlled pre-send failure point")
	}
	assertIntegrationTransportIntent(t, fixture, preSend, preMapped)
	assertNoFormalControlRequest(t, fixture, preSend.RequestID)
	cancelPre()
	if err := <-preDone; err == nil {
		t.Fatal("canceled pre-send request unexpectedly returned a successful response")
	}
	driver.SetHook(nil)
	endpoint, client = reconnectIntegrationAPI(t, ctx, fixture, service, endpoint, client, product)
	if _, err := client.Command(ctx, preSend.RequestID); !integrationClientErrorIs(err, "command_not_found") {
		t.Fatalf("authorities did not report the pre-send request absent after reconnect: %v", err)
	}
	if _, err := client.SubmitCommand(ctx, preSend); err != nil {
		t.Fatalf("original envelope did not recover through its original ID: %v", err)
	}
	waitIntegrationControlApplied(t, ctx, client, preSend.RequestID)
	preRevision, prePaused := integrationControlState(t, ctx, client)
	if preRevision != initialRevision+1 || prePaused == initialPaused || driver.Forwarded(preSend.RequestID) != 1 {
		t.Fatalf("pre-send recovery did not produce exactly one real control action: revision %d->%d paused %t->%t forwarded=%d", initialRevision, preRevision, initialPaused, prePaused, driver.Forwarded(preSend.RequestID))
	}

	// The formal host request is published, then the HTTP client loses its
	// response. Reconnect queries that same authority and does not republish.
	lostRevision, lostPaused := integrationControlState(t, ctx, client)
	lost := integrationDispatchEnvelope(t, client, lostRevision, lostPaused)
	published := make(chan api.ControlRequest, 1)
	driver.SetHook(func(requestCtx context.Context, input api.ControlInput, submit integrationInboxPublisher) (api.ControlRequest, error) {
		if input.RequestID != lost.RequestID {
			return submit(requestCtx, input)
		}
		request, submitErr := submit(requestCtx, input)
		if submitErr != nil {
			return request, submitErr
		}
		published <- request
		<-requestCtx.Done()
		return request, requestCtx.Err()
	})
	lostContext, cancelLost := context.WithCancel(ctx)
	lostDone := make(chan error, 1)
	go func() {
		_, submitErr := client.SubmitCommand(lostContext, lost)
		lostDone <- submitErr
	}()
	select {
	case request := <-published:
		if request.RequestID != lost.RequestID || request.ExpectedControlRevision != lost.ExpectedControlRevision {
			cancelLost()
			t.Fatalf("formal control authority published a different original identity: %+v", request)
		}
	case <-time.After(15 * time.Second):
		cancelLost()
		t.Fatal("HTTP request did not reach the registered-after-send failure point")
	}
	cancelLost()
	if err := <-lostDone; err == nil {
		t.Fatal("canceled post-publish request unexpectedly returned a successful response")
	}
	driver.SetHook(nil)
	endpoint, client = reconnectIntegrationAPI(t, ctx, fixture, service, endpoint, client, product)
	settledLost := waitIntegrationControlApplied(t, ctx, client, lost.RequestID)
	if settledLost.Original == nil || settledLost.Original.RequestID != lost.RequestID || driver.Forwarded(lost.RequestID) != 1 {
		t.Fatalf("lost-response recovery did not return the original formal receipt: %+v forwarded=%d", settledLost, driver.Forwarded(lost.RequestID))
	}
	if _, err := client.SubmitCommand(ctx, lost); err != nil {
		t.Fatalf("retry after lost response did not reconcile against the original receipt: %v", err)
	}
	if driver.Forwarded(lost.RequestID) != 1 {
		t.Fatalf("retry after lost response republished the control request: forwarded=%d", driver.Forwarded(lost.RequestID))
	}
	settledRevision, settledPaused := integrationControlState(t, ctx, client)
	if settledRevision != lostRevision+1 || settledPaused == lostPaused {
		t.Fatalf("lost response produced other than one business state change: revision %d->%d paused %t->%t", lostRevision, settledRevision, lostPaused, settledPaused)
	}

	// A second formal consumer wins after transport registration. The late HTTP
	// submit may report the immutable publisher collision; GET must still join
	// that persisted content to the original envelope and expose one action.
	raceRevision, racePaused := integrationControlState(t, ctx, client)
	race := integrationDispatchEnvelope(t, client, raceRevision, racePaused)
	raceEntered, releaseRace := make(chan api.ControlInput, 1), make(chan struct{})
	driver.SetHook(func(requestCtx context.Context, input api.ControlInput, submit integrationInboxPublisher) (api.ControlRequest, error) {
		if input.RequestID != race.RequestID {
			return submit(requestCtx, input)
		}
		raceEntered <- input
		select {
		case <-releaseRace:
			return submit(requestCtx, input)
		case <-requestCtx.Done():
			return api.ControlRequest{}, requestCtx.Err()
		}
	})
	type commandResult struct {
		response httpapi.CommandResponse
		err      error
	}
	raceDone := make(chan commandResult, 1)
	go func() {
		response, submitErr := client.SubmitCommand(ctx, race)
		raceDone <- commandResult{response: response, err: submitErr}
	}()
	var raceMapped api.ControlInput
	select {
	case raceMapped = <-raceEntered:
	case <-time.After(15 * time.Second):
		close(releaseRace)
		t.Fatal("HTTP request did not enter the late-consumer race window")
	}
	assertIntegrationTransportIntent(t, fixture, race, raceMapped)
	formal, err := driver.PublishAsFormalConsumer(ctx, raceMapped)
	if err != nil || formal.RequestID != race.RequestID || formal.ExpectedControlRevision != race.ExpectedControlRevision {
		close(releaseRace)
		t.Fatalf("formal control consumer failed to publish the original mapped intent: %+v %v", formal, err)
	}
	close(releaseRace)
	raceOutcome := <-raceDone
	if raceOutcome.err != nil {
		if !integrationClientErrorIs(raceOutcome.err, "identity_conflict") {
			t.Fatalf("late immutable publisher collision was not reported as identity_conflict: %v", raceOutcome.err)
		}
	} else if raceOutcome.response.RequestID != race.RequestID || raceOutcome.response.ControlRequest == nil ||
		raceOutcome.response.ControlRequest.RequestID != race.RequestID || raceOutcome.response.ControlRequest.ExpectedControlRevision != race.ExpectedControlRevision {
		t.Fatalf("late formal consumer response changed the original request identity: %+v", raceOutcome.response)
	}
	raceReceipt := waitIntegrationControlApplied(t, ctx, client, race.RequestID)
	if raceReceipt.Original == nil || raceReceipt.Original.RequestID != race.RequestID {
		t.Fatalf("GET did not reconcile the formal consumer result to its original envelope: %+v", raceReceipt)
	}
	var duplicateWG sync.WaitGroup
	duplicateErrors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		duplicateWG.Add(1)
		go func() {
			defer duplicateWG.Done()
			_, submitErr := client.SubmitCommand(ctx, race)
			duplicateErrors <- submitErr
		}()
	}
	duplicateWG.Wait()
	close(duplicateErrors)
	for duplicateErr := range duplicateErrors {
		if duplicateErr != nil {
			t.Fatalf("late duplicate did not reconcile to the original formal receipt: %v", duplicateErr)
		}
	}
	if driver.Forwarded(race.RequestID) != 2 {
		t.Fatalf("late duplicates caused extra formal submissions: forwarded=%d", driver.Forwarded(race.RequestID))
	}
	assertIntegrationControlRequestAndReceipt(t, fixture, race.RequestID)
	raceAfterRevision, raceAfterPaused := integrationControlState(t, ctx, client)
	if raceAfterRevision != raceRevision+1 || raceAfterPaused == racePaused {
		t.Fatalf("late duplicate race produced other than one business state change: revision %d->%d paused %t->%t", raceRevision, raceAfterRevision, racePaused, raceAfterPaused)
	}
	changedEnvelope := race
	changedEnvelope.ExpectedVersion++
	if _, err = client.SubmitCommand(ctx, changedEnvelope); !integrationClientErrorIs(err, "request_id_conflict") {
		t.Fatalf("same request ID with a changed revision was not rejected: %v", err)
	}

	// A different formal revision under the same request ID must not be
	// projected over the original HTTP envelope, even when its receipt exists.
	foreignRevision, foreignPaused := integrationControlState(t, ctx, client)
	foreign := integrationDispatchEnvelope(t, client, foreignRevision, foreignPaused)
	foreignEntered, releaseForeign := make(chan api.ControlInput, 1), make(chan struct{})
	driver.SetHook(func(requestCtx context.Context, input api.ControlInput, submit integrationInboxPublisher) (api.ControlRequest, error) {
		if input.RequestID != foreign.RequestID {
			return submit(requestCtx, input)
		}
		foreignEntered <- input
		select {
		case <-releaseForeign:
			return submit(requestCtx, input)
		case <-requestCtx.Done():
			return api.ControlRequest{}, requestCtx.Err()
		}
	})
	foreignDone := make(chan error, 1)
	go func() {
		_, submitErr := client.SubmitCommand(ctx, foreign)
		foreignDone <- submitErr
	}()
	var foreignMapped api.ControlInput
	select {
	case foreignMapped = <-foreignEntered:
	case <-time.After(15 * time.Second):
		close(releaseForeign)
		t.Fatal("HTTP request did not enter the foreign-revision race window")
	}
	assertIntegrationTransportIntent(t, fixture, foreign, foreignMapped)
	wrongRevision := foreignRevision + 1
	foreignMapped.ExpectedRevision = &wrongRevision
	foreignFormal, err := driver.PublishAsFormalConsumer(ctx, foreignMapped)
	if err != nil || foreignFormal.ExpectedControlRevision != wrongRevision {
		close(releaseForeign)
		t.Fatalf("formal consumer did not publish its conflicting revision: %+v %v", foreignFormal, err)
	}
	close(releaseForeign)
	if err = <-foreignDone; !integrationClientErrorIs(err, "identity_conflict") {
		t.Fatalf("different formal revision was not rejected as an identity conflict: %v", err)
	}
	foreignReceipt := waitIntegrationControlReceipt(t, ctx, product, foreign.RequestID)
	if foreignReceipt.Status != "rejected" {
		t.Fatalf("foreign stale revision unexpectedly applied: %+v", foreignReceipt)
	}
	if _, err = client.Command(ctx, foreign.RequestID); !integrationClientErrorIs(err, "identity_conflict") {
		t.Fatalf("GET overlaid the foreign control receipt onto the original envelope: %v", err)
	}
	foreignAfterRevision, foreignAfterPaused := integrationControlState(t, ctx, client)
	if foreignAfterRevision != foreignRevision || foreignAfterPaused != foreignPaused || driver.Forwarded(foreign.RequestID) != 2 {
		t.Fatalf("foreign revision collision changed business state or retried: revision %d->%d paused %t->%t forwarded=%d", foreignRevision, foreignAfterRevision, foreignPaused, foreignAfterPaused, driver.Forwarded(foreign.RequestID))
	}
}

func reconnectIntegrationAPI(t *testing.T, ctx context.Context, fixture integrationFixture, service *runningIntegrationService, endpoint *httpapi.Server, client *httpapi.Client, product *productclient.Client) (*httpapi.Server, *httpapi.Client) {
	t.Helper()
	if err := endpoint.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	server, reconnected, err := newIntegrationHTTPAPI(ctx, fixture, service, product)
	require(t, err)
	return server, reconnected
}

func assertIntegrationTransportIntent(t *testing.T, fixture integrationFixture, envelope httpapi.CommandEnvelope, mapped api.ControlInput) {
	t.Helper()
	path := filepath.Join(fixture.config.RunDir, "local-api", "commands", envelope.RequestID+".json")
	data, err := os.ReadFile(path)
	require(t, err)
	var record struct {
		Envelope httpapi.CommandEnvelope `json:"envelope"`
		Control  *api.ControlInput       `json:"control"`
	}
	require(t, json.Unmarshal(data, &record))
	if !reflect.DeepEqual(record.Envelope, envelope) || record.Control == nil || !reflect.DeepEqual(*record.Control, mapped) {
		t.Fatalf("transport record did not bind the original envelope to its actual mapped intent: envelope=%+v mapped=%+v record=%+v", envelope, mapped, record)
	}
}

func assertNoFormalControlRequest(t *testing.T, fixture integrationFixture, requestID string) {
	t.Helper()
	path := filepath.Join(fixture.config.RunDir, "control", "requests", requestID, "request.json")
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("pre-send fault already published a formal control request at %s: %v", path, err)
	}
}

func assertIntegrationControlRequestAndReceipt(t *testing.T, fixture integrationFixture, requestID string) {
	t.Helper()
	requestPath := filepath.Join(fixture.config.RunDir, "control", "requests", requestID, "request.json")
	receiptPath := filepath.Join(fixture.config.RunDir, "control", "receipts", requestID+".json")
	if _, err := os.Stat(requestPath); err != nil {
		t.Fatalf("formal control request missing after recovery: %v", err)
	}
	if _, err := os.Stat(receiptPath); err != nil {
		t.Fatalf("formal control receipt missing after recovery: %v", err)
	}
}

func waitIntegrationControlReceipt(t *testing.T, ctx context.Context, product *productclient.Client, requestID string) *api.ControlReceipt {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		receipt, err := product.Receipt(ctx, requestID)
		if err == nil && receipt != nil {
			return receipt
		}
		time.Sleep(25 * time.Millisecond)
	}
	receipt, err := product.Receipt(ctx, requestID)
	t.Fatalf("formal control receipt did not settle: receipt=%+v err=%v", receipt, err)
	return nil
}

func integrationClientErrorIs(err error, code string) bool {
	var clientErr *httpapi.ClientError
	return errors.As(err, &clientErr) && clientErr.Code == code
}
