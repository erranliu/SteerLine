package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	steerlinehost "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	transport "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	api "github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
)

// These tests use a temporary Git project, the real app service and SQLite
// stores, and the controlled app-server/Git fixtures owned by this package.
// They never start a live model or touch the user's configured project.
func TestAPIServiceLifecycle(t *testing.T) {
	requireWindowsAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	service := startIntegrationService(t, ctx, fixture.config, &integrationLifecycleResolver{})
	defer service.stop(t)
	product, err := steerlineapp.OpenClient(ctx, fixture.configPath)
	require(t, err)
	defer product.Close()
	identity := service.service.State.Service.ProcessIdentity
	if identity == nil {
		t.Fatal("controlled service did not publish its real process identity")
	}
	process := httpapi.ProcessIdentity{Kind: identity.Kind, PID: identity.PID, Created: identity.Created, Executable: identity.Executable}
	endpoint, err := httpapi.Prepare(ctx, product, httpapi.Options{RunDir: fixture.config.RunDir, ProjectIdentity: fixture.config.Identity(),
		InstanceID: service.instanceID, Process: process, Capabilities: append([]string(nil), service.service.State.Service.Capabilities...)})
	require(t, err)
	if err = endpoint.SetReady(service.instanceID, false, "test service not ready"); err != nil {
		t.Fatal(err)
	}
	if err = endpoint.Start(); err != nil {
		t.Fatal(err)
	}
	client, err := httpclient.Open(ctx, fixture.config.RunDir, fixture.config.Identity())
	require(t, err)
	if client.Identity().Ready || client.Identity().Diagnostic != "test service not ready" {
		t.Fatalf("service startup diagnostic was lost: %+v", client.Identity())
	}
	if _, err = client.ListTasks(ctx, "", 10); err != nil {
		t.Fatalf("read-only query was blocked while preserving the startup diagnosis: %v", err)
	}
	snapshot, err := client.Snapshot(ctx, "")
	require(t, err)
	if snapshot.Connection.Snapshot.Page == nil {
		t.Fatal("read-only project snapshot omitted the existing service record")
	}
	revision := snapshot.Connection.Snapshot.Page.Control.Revision
	blocked := httpapi.CommandEnvelope{RequestID: randomRequestID(t), ProjectID: httpapi.ProjectID(fixture.config.Identity()),
		InstanceID: service.instanceID, Action: "pause_dispatch", TargetID: "dispatch", ExpectedVersion: revision,
		ExpectedControlRevision: revision, Payload: []byte(`{}`)}
	if _, err = client.SubmitCommand(ctx, blocked); err == nil || !strings.Contains(err.Error(), "service_not_ready") {
		t.Fatalf("not-ready service accepted a business control action: %v", err)
	}
	rejected, err := client.Command(ctx, blocked.RequestID)
	if err != nil || rejected.Status != "rejected" || rejected.Code != "service_not_ready" {
		t.Fatalf("not-ready request did not retain its original rejection: %+v %v", rejected, err)
	}
	endpointCloseCtx, endpointCancel := context.WithTimeout(context.Background(), time.Second)
	if err = endpoint.SetReady(service.instanceID, true, ""); err != nil {
		endpointCancel()
		t.Fatal(err)
	}
	if err = endpoint.Close(endpointCloseCtx); err != nil {
		endpointCancel()
		t.Fatal(err)
	}
	endpointCancel()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require(t, err)
	defer listener.Close()
	_, err = httpapi.Prepare(ctx, product, httpapi.Options{RunDir: fixture.config.RunDir, ProjectIdentity: fixture.config.Identity(),
		InstanceID: service.instanceID, Address: listener.Addr().String()})
	if err == nil || !strings.Contains(err.Error(), "bind local API") {
		t.Fatalf("existing port conflict did not stop endpoint startup: %v", err)
	}
	if _, err = steerlineapp.OpenHTTPClient(ctx, fixture.configPath); err == nil || !strings.Contains(err.Error(), "bind local API loopback listener") {
		t.Fatalf("client did not get the real port conflict diagnosis or tried to replace the service: %v", err)
	}
}

func TestAPIConfiguredServicePublishesDiscovery(t *testing.T) {
	requireWindowsAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	serviceCtx, stopService := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- steerlineapp.RunConfiguredService(serviceCtx, fixture.config, "manual", "", "off") }()
	var client *httpclient.Client
	var lastOpenErr error
	// Native authority preflight and the real service factory precede API
	// publication. Keep a bounded minute for that complete startup path.
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		select {
		case serviceErr := <-done:
			runtimeState, runtimeErr := steerlineapp.LoadDatabaseRuntime(fixture.config)
			startupText := integrationStartupText(fixture.config.RunDir)
			t.Fatalf("RunConfiguredService exited before discovery: service=%v runtime_error=%v runtime=%+v startup=%s", serviceErr, runtimeErr, runtimeState, startupText)
		default:
		}
		if client != nil {
			_ = client.Close()
			client = nil
		}
		connected, openErr := steerlineapp.OpenHTTPClient(serviceCtx, fixture.configPath)
		if openErr != nil {
			lastOpenErr = openErr
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if !connected.Identity().Ready {
			lastOpenErr = fmt.Errorf("discovered service not ready: %s", connected.Identity().Diagnostic)
			_ = connected.Close()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		client = connected
		break
	}
	if client == nil {
		stopService()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
		}
		runtimeState, runtimeErr := steerlineapp.LoadDatabaseRuntime(fixture.config)
		t.Fatalf("RunConfiguredService did not publish a ready local HTTP discovery: last_open=%v runtime_error=%v runtime=%+v startup=%s", lastOpenErr, runtimeErr, runtimeState, integrationStartupText(fixture.config.RunDir))
	}
	identity := client.Identity()
	if identity.Protocol != httpapi.Protocol || identity.ProjectID != httpapi.ProjectID(fixture.config.Identity()) || identity.Process.PID != os.Getpid() {
		_ = client.Close()
		stopService()
		t.Fatalf("app service HTTP discovery has a different identity: %+v", identity)
	}
	if _, err := client.Snapshot(ctx, ""); err != nil {
		_ = client.Close()
		stopService()
		t.Fatalf("RunConfiguredService did not serve the product snapshot: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Error(err)
	}
	stopService()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunConfiguredService shutdown: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("RunConfiguredService did not stop its local API listener")
	}
	if _, err := os.Stat(filepath.Join(fixture.config.RunDir, "local-api", "discovery.json")); !os.IsNotExist(err) {
		t.Fatalf("service close left its discovery record behind: %v", err)
	}
}

func TestAPIConfiguredServiceDiscardsTaskThroughOwnedWriter(t *testing.T) {
	requireWindowsAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	createIntegrationTask(t, ctx, fixture, 909)

	serviceCtx, stopService := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- steerlineapp.RunConfiguredService(serviceCtx, fixture.config, "manual", "", "off") }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		stopService()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("RunConfiguredService shutdown: %v", err)
			}
		case <-time.After(20 * time.Second):
			t.Error("RunConfiguredService did not stop after cancellation")
		}
	}
	t.Cleanup(stop)

	var client *httpclient.Client
	var lastOpenErr error
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		select {
		case serviceErr := <-done:
			stopped = true
			t.Fatalf("RunConfiguredService exited before ready: %v", serviceErr)
		default:
		}
		connected, openErr := steerlineapp.OpenHTTPClient(serviceCtx, fixture.configPath)
		if openErr != nil {
			lastOpenErr = openErr
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if !connected.Identity().Ready {
			lastOpenErr = fmt.Errorf("discovered service is not ready: %s", connected.Identity().Diagnostic)
			_ = connected.Close()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		client = connected
		break
	}
	if client == nil {
		t.Fatalf("RunConfiguredService did not publish ready HTTP discovery: %v", lastOpenErr)
	}
	t.Cleanup(func() {
		if client != nil {
			_ = client.Close()
		}
	})
	identity := client.Identity()
	if identity.ProjectID != httpapi.ProjectID(fixture.config.Identity()) || identity.InstanceID == "" || !identity.Ready {
		t.Fatalf("configured service identity is incomplete: %+v", identity)
	}
	assertRepositoryLocked := func(phase string) {
		t.Helper()
		lock, err := steerlinehost.LockRepository(fixture.config.Identity())
		if err == nil {
			lock.Close()
			t.Fatalf("configured service released its repository lock %s", phase)
		}
		if !strings.Contains(err.Error(), "repository already locked") {
			t.Fatalf("repository lock check failed for an unrelated reason %s: %v", phase, err)
		}
	}
	assertRepositoryLocked("after readiness")

	beforeTask, err := client.ShowTask(ctx, "909")
	require(t, err)
	if len(beforeTask.Tasks) != 1 || beforeTask.Tasks[0].Status != "open" || beforeTask.Tasks[0].Version == 0 {
		t.Fatalf("configured-service fixture task was not open: %+v", beforeTask.Tasks)
	}
	beforeGraph, err := client.Graph(ctx, "", 20)
	require(t, err)
	if beforeGraph.GraphRevision != beforeTask.GraphRevision {
		t.Fatalf("task and graph queries came from different native snapshots: task=%+v graph=%+v", beforeTask, beforeGraph)
	}
	envelope := httpapi.CommandEnvelope{RequestID: randomRequestID(t), ProjectID: identity.ProjectID, InstanceID: identity.InstanceID,
		Action: "discard_task", TargetID: "909", ExpectedVersion: beforeTask.Tasks[0].Version,
		Payload: mustJSON(map[string]string{"reason": "configured service owns the discard writer"})}
	first, err := client.SubmitCommand(ctx, envelope)
	require(t, err)
	if first.Status != "applied" || first.Discard == nil || first.Discard.RequestID != envelope.RequestID ||
		first.Discard.TaskID != "909" || first.Discard.Status != "discarded" || first.Command == nil ||
		first.Command.CommandID != envelope.RequestID || first.Command.Kind != "discard_task" || first.Command.TargetID != "909" ||
		first.Command.DiscardRequest == nil || first.Command.DiscardRequest.Reason != "configured service owns the discard writer" {
		t.Fatalf("configured service did not return the typed original discard receipt: %+v", first)
	}
	if first.Original == nil || first.Original.RequestID != envelope.RequestID || first.Original.Action != envelope.Action ||
		first.Original.TargetID != envelope.TargetID || string(first.Original.Payload) != string(envelope.Payload) {
		t.Fatalf("discard receipt lost its original immutable HTTP envelope: %+v", first.Original)
	}
	afterTask, err := client.ShowTask(ctx, "909")
	require(t, err)
	afterGraph, err := client.Graph(ctx, "", 20)
	require(t, err)
	if len(afterTask.Tasks) != 1 || afterTask.Tasks[0].Status != "discarded" || afterTask.Tasks[0].Version <= beforeTask.Tasks[0].Version ||
		afterGraph.GraphRevision <= beforeGraph.GraphRevision || afterTask.GraphRevision != afterGraph.GraphRevision {
		t.Fatalf("discard did not change the task and graph exactly through native authority: before=%+v after=%+v graph_before=%+v graph_after=%+v",
			beforeTask.Tasks[0], afterTask.Tasks[0], beforeGraph, afterGraph)
	}
	stableAuthority, stableGraph, stableVersion := afterTask.AuthorityVersion, afterTask.GraphRevision, afterTask.Tasks[0].Version

	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	client, err = steerlineapp.OpenHTTPClient(ctx, fixture.configPath)
	require(t, err)
	if client.Identity().InstanceID != identity.InstanceID || !client.Identity().Ready {
		t.Fatalf("reconnected client observed another or unready service instance: %+v", client.Identity())
	}
	reconnectedReceipt, err := client.Command(ctx, envelope.RequestID)
	require(t, err)
	if reconnectedReceipt.Status != "applied" || reconnectedReceipt.Discard == nil || reconnectedReceipt.Command == nil ||
		reconnectedReceipt.Discard.RequestID != first.Discard.RequestID || reconnectedReceipt.Command.CommandID != first.Command.CommandID {
		t.Fatalf("reconnected command lookup did not recover the original typed receipt: %+v", reconnectedReceipt)
	}
	duplicate, err := client.SubmitCommand(ctx, envelope)
	require(t, err)
	if duplicate.Status != "applied" || duplicate.Discard == nil || duplicate.Discard.RequestID != first.Discard.RequestID ||
		duplicate.Discard.GraphRevision != first.Discard.GraphRevision {
		t.Fatalf("same-envelope retry did not reconcile to the original discard outcome: first=%+v duplicate=%+v", first.Discard, duplicate.Discard)
	}
	changed := envelope
	changed.Payload = mustJSON(map[string]string{"reason": "conflicting use of the same request ID"})
	if _, err = client.SubmitCommand(ctx, changed); err == nil {
		t.Fatal("same request ID with different content was accepted")
	} else {
		var clientErr *httpclient.ClientError
		if !errors.As(err, &clientErr) || clientErr.Status != 409 || clientErr.Code != "request_id_conflict" {
			t.Fatalf("same-ID content conflict returned the wrong result: %v", err)
		}
	}
	finalReceipt, err := client.Command(ctx, envelope.RequestID)
	require(t, err)
	finalTask, err := client.ShowTask(ctx, "909")
	require(t, err)
	finalGraph, err := client.Graph(ctx, "", 20)
	require(t, err)
	if finalReceipt.Discard == nil || finalReceipt.Discard.GraphRevision != first.Discard.GraphRevision || len(finalTask.Tasks) != 1 ||
		finalTask.Tasks[0].Status != "discarded" || finalTask.Tasks[0].Version != stableVersion ||
		finalTask.AuthorityVersion != stableAuthority || finalTask.GraphRevision != stableGraph || finalGraph.GraphRevision != stableGraph {
		t.Fatalf("duplicate or conflicting request changed authoritative task state: receipt=%+v task=%+v graph=%+v", finalReceipt, finalTask, finalGraph)
	}
	assertRepositoryLocked("after duplicate and conflicting retries")
	refreshed, err := client.RefreshIdentity(ctx)
	require(t, err)
	if !refreshed.Ready || refreshed.InstanceID != identity.InstanceID {
		t.Fatalf("service stopped being ready after task discard: %+v", refreshed)
	}
	revision, paused := integrationControlState(t, ctx, client)
	control := integrationDispatchEnvelope(t, client, revision, paused)
	if _, err = client.SubmitCommand(ctx, control); err != nil {
		t.Fatalf("configured service stopped processing normal control after discard: %v", err)
	}
	_ = waitIntegrationControlApplied(t, ctx, client, control.RequestID)
	refreshed, err = client.RefreshIdentity(ctx)
	require(t, err)
	if !refreshed.Ready {
		t.Fatal("configured service was not ready after a subsequent control receipt")
	}
	assertRepositoryLocked("after subsequent service control")

	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	stop()
	if _, err = os.Stat(filepath.Join(fixture.config.RunDir, "local-api", "discovery.json")); !os.IsNotExist(err) {
		t.Fatalf("configured service shutdown left discovery behind: %v", err)
	}
	lock, err := steerlinehost.LockRepository(fixture.config.Identity())
	require(t, err)
	lock.Close()
}

func integrationStartupText(runDir string) string {
	root := filepath.Join(runDir, "service", "startups")
	entries, err := os.ReadDir(root)
	if err != nil {
		return err.Error()
	}
	var values []string
	for _, entry := range entries {
		data, readErr := os.ReadFile(filepath.Join(root, entry.Name(), "startup.json"))
		if readErr != nil {
			values = append(values, entry.Name()+": "+readErr.Error())
		} else {
			values = append(values, entry.Name()+": "+string(data))
		}
	}
	return strings.Join(values, "\n")
}

func TestAPIIdentityAndAuthentication(t *testing.T) {
	requireWindowsAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	service := startIntegrationService(t, ctx, fixture.config, &integrationLifecycleResolver{})
	defer service.stop(t)
	endpoint, client, product := startIntegrationHTTPAPI(t, ctx, fixture, service)
	defer func() {
		if endpoint != nil {
			_ = endpoint.Close(context.Background())
		}
	}()
	defer product.Close()
	if client.Identity().Protocol != httpapi.Protocol || client.Identity().ProjectID != httpapi.ProjectID(fixture.config.Identity()) ||
		client.Identity().InstanceID != service.instanceID || !client.Identity().Ready || client.Identity().Process.PID != os.Getpid() {
		t.Fatalf("HTTP identity did not match the real service process: %+v", client.Identity())
	}
	initialRevision, initialPaused := integrationControlState(t, ctx, client)
	discovery, token := integrationDiscoveryCredential(t, fixture.config.RunDir)
	valid := integrationDispatchEnvelope(t, client, initialRevision, initialPaused)
	wrongProjectRoute := httpapi.ProjectID(fixture.config.Identity() + "/different-project")
	wrongProjectEnvelope := valid
	wrongProjectEnvelope.RequestID = randomRequestID(t)
	wrongProjectEnvelope.ProjectID += "-wrong"
	wrongInstance := valid
	wrongInstance.RequestID = randomRequestID(t)
	wrongInstance.InstanceID += "-wrong"
	for _, attempt := range []struct {
		name, token, host, origin, route string
		envelope                         httpapi.CommandEnvelope
		wantCode                         string
	}{
		{name: "missing credential", envelope: valid, route: client.Identity().ProjectID, wantCode: "unauthorized"},
		{name: "wrong credential", token: strings.Repeat("0", 64), envelope: valid, route: client.Identity().ProjectID, wantCode: "unauthorized"},
		{name: "wrong Host", token: token, host: "localhost:1", envelope: valid, route: client.Identity().ProjectID, wantCode: "host_rejected"},
		{name: "browser Origin", token: token, origin: "https://untrusted.example", envelope: valid, route: client.Identity().ProjectID, wantCode: "origin_rejected"},
		{name: "wrong project route", token: token, envelope: valid, route: wrongProjectRoute, wantCode: "project_mismatch"},
		{name: "wrong project envelope", token: token, envelope: wrongProjectEnvelope, route: client.Identity().ProjectID, wantCode: "request_invalid"},
		{name: "wrong instance envelope", token: token, envelope: wrongInstance, route: client.Identity().ProjectID, wantCode: "request_invalid"},
	} {
		if attempt.envelope.RequestID == valid.RequestID {
			attempt.envelope.RequestID = randomRequestID(t)
		}
		status, response, requestErr := postIntegrationCommand(ctx, discovery.Address, attempt.route, attempt.token, attempt.host, attempt.origin, attempt.envelope)
		if requestErr != nil || response.Code != attempt.wantCode {
			t.Fatalf("%s was not rejected at the real listener: status=%d response=%+v err=%v", attempt.name, status, response, requestErr)
		}
		assertNoIntegrationCommandEffect(t, ctx, fixture, product, attempt.envelope.RequestID)
	}
	if afterRevision, afterPaused := integrationControlState(t, ctx, client); afterRevision != initialRevision || afterPaused != initialPaused {
		t.Fatalf("unauthorized/misbound requests changed control facts: revision %d->%d paused %t->%t", initialRevision, afterRevision, initialPaused, afterPaused)
	}
	if err := endpoint.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	rotatedEndpoint, rotatedClient, err := newIntegrationHTTPAPI(ctx, fixture, service, product)
	require(t, err)
	endpoint, client = rotatedEndpoint, rotatedClient
	newDiscovery, newToken := integrationDiscoveryCredential(t, fixture.config.RunDir)
	if newDiscovery.LeaseID == discovery.LeaseID || newToken == token || client.Identity().InstanceID != service.instanceID {
		t.Fatalf("service reconnection did not rotate credentials while preserving instance identity: old=%+v new=%+v", discovery, newDiscovery)
	}
	rotatedRevision, rotatedPaused := integrationControlState(t, ctx, client)
	oldCredentialAttempt := integrationDispatchEnvelope(t, client, rotatedRevision, rotatedPaused)
	oldCredentialAttempt.RequestID = randomRequestID(t)
	status, response, requestErr := postIntegrationCommand(ctx, newDiscovery.Address, client.Identity().ProjectID, token, "", "", oldCredentialAttempt)
	if requestErr != nil || response.Code != "unauthorized" {
		t.Fatalf("retired instance credential was accepted after rotation: status=%d response=%+v err=%v", status, response, requestErr)
	}
	assertNoIntegrationCommandEffect(t, ctx, fixture, product, oldCredentialAttempt.RequestID)
	if afterRevision, afterPaused := integrationControlState(t, ctx, client); afterRevision != rotatedRevision || afterPaused != rotatedPaused {
		t.Fatalf("retired credential changed control facts: revision %d->%d paused %t->%t", rotatedRevision, afterRevision, rotatedPaused, afterPaused)
	}
	positive := integrationDispatchEnvelope(t, client, rotatedRevision, rotatedPaused)
	positiveResponse, err := client.SubmitCommand(ctx, positive)
	require(t, err)
	if positiveResponse.ControlRequest == nil {
		t.Fatalf("authorized positive-control request was not published: %+v", positiveResponse)
	}
	waitIntegrationControlApplied(t, ctx, client, positive.RequestID)
	finalRevision, finalPaused := integrationControlState(t, ctx, client)
	if finalRevision != rotatedRevision+1 || finalPaused == rotatedPaused {
		t.Fatalf("authorized positive control did not produce one observable state change: revision %d->%d paused %t->%t", rotatedRevision, finalRevision, rotatedPaused, finalPaused)
	}
}

func integrationDiscoveryCredential(t *testing.T, runDir string) (httpapi.Discovery, string) {
	t.Helper()
	path := filepath.Join(runDir, "local-api", "discovery.json")
	data, err := os.ReadFile(path)
	require(t, err)
	var discovery httpapi.Discovery
	require(t, json.Unmarshal(data, &discovery))
	token, err := os.ReadFile(discovery.CredentialPath)
	require(t, err)
	return discovery, string(token)
}

func postIntegrationCommand(ctx context.Context, address, routeProject, token, host, origin string, envelope httpapi.CommandEnvelope) (int, httpapi.ErrorResponse, error) {
	body, err := json.Marshal(envelope)
	if err != nil {
		return 0, httpapi.ErrorResponse{}, err
	}
	url := "http://" + address + "/v1/projects/" + routeProject + "/commands"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, httpapi.ErrorResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if host != "" {
		request.Host = host
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	transport := &http.Transport{Proxy: nil}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		transport.CloseIdleConnections()
		return 0, httpapi.ErrorResponse{}, err
	}
	defer response.Body.Close()
	defer transport.CloseIdleConnections()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return response.StatusCode, httpapi.ErrorResponse{}, err
	}
	var decoded httpapi.ErrorResponse
	if err = json.Unmarshal(data, &decoded); err != nil {
		return response.StatusCode, decoded, fmt.Errorf("decode command response: %w: %s", err, data)
	}
	return response.StatusCode, decoded, nil
}

func integrationDispatchEnvelope(t *testing.T, client *httpclient.Client, revision uint64, paused bool) httpapi.CommandEnvelope {
	t.Helper()
	action := "pause_dispatch"
	if paused {
		action = "resume_dispatch"
	}
	identity := client.Identity()
	return httpapi.CommandEnvelope{RequestID: randomRequestID(t), ProjectID: identity.ProjectID, InstanceID: identity.InstanceID,
		Action: action, TargetID: "dispatch", ExpectedVersion: revision, ExpectedControlRevision: revision, Payload: []byte(`{}`)}
}

func integrationControlState(t *testing.T, ctx context.Context, client *httpclient.Client) (uint64, bool) {
	t.Helper()
	snapshot, err := client.Snapshot(ctx, "")
	require(t, err)
	if snapshot.Connection.Snapshot.Page == nil {
		t.Fatal("real project snapshot omitted control state")
	}
	control := snapshot.Connection.Snapshot.Page.Control
	return control.Revision, control.DispatchPaused
}

func assertNoIntegrationCommandEffect(t *testing.T, ctx context.Context, fixture integrationFixture, product *productclient.Client, requestID string) {
	t.Helper()
	for _, path := range []string{
		filepath.Join(fixture.config.RunDir, "local-api", "commands", requestID+".json"),
		filepath.Join(fixture.config.RunDir, "control", "requests", requestID, "request.json"),
		filepath.Join(fixture.config.RunDir, "control", "receipts", requestID+".json"),
	} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("rejected request left an original transport/control record at %s: %v", path, err)
		}
	}
	if _, err := product.Command(ctx, requestID); !errors.Is(err, api.ErrRecordNotFound) {
		t.Fatalf("rejected request left a SQLite command fact or a read error: %v", err)
	}
}

func waitIntegrationControlApplied(t *testing.T, ctx context.Context, client *httpclient.Client, requestID string) httpapi.CommandResponse {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	for {
		response, err := client.Command(deadline, requestID)
		if err == nil && response.ControlReceipt != nil && response.ControlReceipt.Status == "applied" {
			return response
		}
		if deadline.Err() != nil {
			t.Fatalf("real control request did not apply: response=%+v err=%v", response, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestAPIQueryParity(t *testing.T) {
	requireWindowsAPI(t)
	// Two original candidates each run execute/report before their API and
	// frozen profile facts are queried. Match the staged wiring's 5m bound.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	addAlternateLocalProfile(t, &fixture)
	addUnsupportedOrcaProfile(t, &fixture)
	createIntegrationTask(t, ctx, fixture, 901)
	service := startIntegrationService(t, ctx, fixture.config, &integrationLifecycleResolver{})
	defer service.stop(t)
	endpoint, client, product := startIntegrationHTTPAPI(t, ctx, fixture, service)
	defer endpoint.Close(context.Background())
	defer product.Close()

	page, err := client.ListTasks(ctx, "", 25)
	require(t, err)
	if len(page.Tasks) != 1 || page.Tasks[0].TaskID != "901" || page.DatabaseID == "" || page.GraphRevision == 0 {
		t.Fatalf("HTTP task page dropped product query identity/version: %+v", page)
	}
	graph, err := client.Graph(ctx, "", 20)
	require(t, err)
	if graph.DatabaseID != page.DatabaseID || graph.GraphRevision != page.GraphRevision || len(graph.Nodes) != 1 || graph.Nodes[0].TaskID != "901" {
		t.Fatalf("HTTP graph mixed or lost the native query snapshot: %+v", graph)
	}
	snapshot, err := client.Snapshot(ctx, "901")
	require(t, err)
	if snapshot.SelectedNative == nil || snapshot.SelectedNative.TaskID != "901" || snapshot.Connection.Snapshot.Page == nil || snapshot.Connection.Snapshot.Page.Service.InstanceID != service.instanceID {
		t.Fatalf("HTTP project snapshot lost selected task or service identity: %+v", snapshot)
	}
	candidates, err := client.Candidates(ctx)
	require(t, err)
	if candidates.RepositoryIdentity != fixture.config.Identity() || candidates.InstanceID != service.instanceID {
		t.Fatalf("HTTP candidates were not bound to this project/service: %+v", candidates)
	}
	presentation, err := client.ProjectPresentation(ctx)
	require(t, err)
	defaultProfileID := presentation.Presentation.DefaultProfileID
	profileID := "fixture-alt"
	if presentation.ProjectID != httpapi.ProjectID(fixture.config.Identity()) || defaultProfileID != "fixture" || defaultProfileID == profileID {
		t.Fatalf("HTTP presentation omitted project identity or configured default profile: %+v", presentation)
	}
	var selectedProfile, defaultProfile, unsupportedOrcaProfile *api.ExecutionProfileSummary
	for i := range presentation.Presentation.Profiles {
		if presentation.Presentation.Profiles[i].ID == profileID {
			selectedProfile = &presentation.Presentation.Profiles[i]
		}
		if presentation.Presentation.Profiles[i].ID == defaultProfileID {
			defaultProfile = &presentation.Presentation.Profiles[i]
		}
		if presentation.Presentation.Profiles[i].ID == "orca-terminal" {
			unsupportedOrcaProfile = &presentation.Presentation.Profiles[i]
		}
	}
	if selectedProfile == nil || defaultProfile == nil || selectedProfile.UnavailableReason != "" || selectedProfile.Digest == "" || selectedProfile.Name == "" {
		t.Fatalf("HTTP presentation did not expose both default and usable nondefault profiles: %+v", presentation.Presentation)
	}
	if unsupportedOrcaProfile == nil || unsupportedOrcaProfile.UnavailableReason != "" {
		t.Fatalf("HTTP presentation rejected the terminal route before native preflight: %+v", presentation.Presentation.Profiles)
	}
	for _, configured := range fixture.config.AvailableExecutionProfiles() {
		if configured.ID == profileID && configured.Digest(fixture.config.ExecutionDirectory) != selectedProfile.Digest {
			t.Fatal("HTTP profile digest differs from the existing application profile query")
		}
	}

	claimIntegrationTaskWithProfile(t, ctx, fixture, 901, service.instanceID, defaultProfileID)
	run := waitIntegrationRun(t, ctx, service, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && run.Decision != nil && run.Decision.Status == "pending" && execution.CurrentTurnID == execution.LastStoppedTurnID
	})
	firstRunID := run.run.RunID
	projectSnapshot, err := client.Snapshot(ctx, "901")
	require(t, err)
	if projectSnapshot.History == nil || len(projectSnapshot.History.Decisions) == 0 {
		t.Fatal("HTTP history query omitted the real controlled decision")
	}
	candidate, err := client.Run(ctx, run.run.RunID)
	require(t, err)
	if candidate.ExecutionID != run.execution.ExecutionID || candidate.AttemptID != run.run.AttemptID || candidate.Decision == nil {
		t.Fatalf("run query collapsed or changed execution identity: %+v", candidate)
	}
	activity, err := client.Activity(ctx, candidate.RunID, candidate.ExecutionID, candidate.AttemptID, api.ActivityCursor{})
	require(t, err)
	if activity.Page.Cursor.RunID != candidate.RunID || activity.Page.Cursor.ExecutionID != candidate.ExecutionID || activity.Page.Cursor.AttemptID != candidate.AttemptID {
		t.Fatalf("activity page cursor lost its source identity: %+v", activity.Page.Cursor)
	}
	final, err := client.FinalReply(ctx, candidate.RunID)
	require(t, err)
	if final.ExecutionID != candidate.ExecutionID || len(final.Page.Events) == 0 || len(final.Bodies) == 0 || !final.Bodies[0].Complete || !strings.Contains(final.Bodies[0].Text, "completed") {
		t.Fatalf("final reply was not read from the real controlled conversation store: %+v", final)
	}
	if len(activity.Page.Activity) == 0 {
		t.Fatal("recent activity query did not return the real controlled service events")
	}

	current, err := client.Snapshot(ctx, "901")
	require(t, err)
	if current.SelectedNative == nil || current.NativeTasks == nil || current.Connection.Snapshot.Page == nil {
		t.Fatal("selected task snapshot lacks task, graph or service control identity")
	}
	startCandidate := httpapi.CommandEnvelope{RequestID: randomRequestID(t), ProjectID: httpapi.ProjectID(fixture.config.Identity()),
		InstanceID: service.instanceID, Action: "start_candidate", TargetID: "901", ExpectedVersion: current.SelectedNative.Version,
		ExpectedControlRevision: current.Connection.Snapshot.Page.Control.Revision, ExpectedGraphRevision: current.NativeTasks.GraphRevision,
		Payload: mustJSON(map[string]string{"execution_profile_id": profileID})}
	started, err := client.SubmitCommand(ctx, startCandidate)
	require(t, err)
	if started.ControlRequest == nil {
		t.Fatalf("second native candidate did not publish its original control request: %+v", started)
	}
	var postedProfile api.ControlPayload
	require(t, json.Unmarshal(started.ControlRequest.Payload, &postedProfile))
	if postedProfile.ExecutionProfileID != profileID || postedProfile.ExecutionProfileDigest != selectedProfile.Digest {
		t.Fatalf("HTTP start did not preserve the selected nondefault profile ID/digest: %+v", postedProfile)
	}
	second := waitIntegrationRun(t, ctx, service, fixture.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && run.RunID != firstRunID && run.Decision != nil && run.Decision.Status == "pending" &&
			e.CurrentTurnID != "" && e.CurrentTurnID == e.LastStoppedTurnID
	})
	if second.run.ExecutionProfile == nil || second.run.ExecutionProfile.ID != profileID || second.run.ExecutionProfile.Digest == "" ||
		second.execution.ConfigurationDigest != second.run.ExecutionProfile.Digest {
		t.Fatalf("nondefault execution did not persist its own frozen profile identity: profile=%+v execution=%+v", second.run.ExecutionProfile, second.execution)
	}
	var selectedWorkerConfig *codexworker.ProfileConfig
	for _, envelope := range second.run.ExecutionProfile.ImplementationData {
		if envelope.ImplementationKey != codexworker.ProfileCodecKey {
			continue
		}
		settings, decodeErr := codexworker.DecodeProfileConfig(envelope.Payload)
		require(t, decodeErr)
		selectedWorkerConfig = &settings
	}
	if selectedWorkerConfig == nil || selectedWorkerConfig.Model != "fixture-alt-model" || selectedWorkerConfig.ReasoningEffort != "high" {
		t.Fatalf("actual backend turn did not use the selected nondefault implementation: %+v", selectedWorkerConfig)
	}
	t.Logf("profile query/request digest=%s; claim-frozen profile digest=%s", selectedProfile.Digest, second.run.ExecutionProfile.Digest)
	var candidateRuns []api.CandidateSummary
	waitFor(t, 30*time.Second, func() bool {
		fresh, readErr := client.Snapshot(ctx, "901")
		if readErr != nil || fresh.Connection.Snapshot.Page == nil {
			return false
		}
		candidateRuns = nil
		for _, run := range fresh.Connection.Snapshot.Page.Runs {
			if run != nil && run.TaskID == "901" {
				candidateRuns = append(candidateRuns, *run)
			}
		}
		return len(candidateRuns) >= 2
	})
	if len(candidateRuns) != 2 || candidateRuns[0].RunID == candidateRuns[1].RunID || candidateRuns[0].ExecutionID == candidateRuns[1].ExecutionID || candidateRuns[0].AttemptID == candidateRuns[1].AttemptID {
		t.Fatalf("HTTP query merged the two local candidates or their identities: %+v", candidateRuns)
	}
}

func TestAPIWorkerStageQueryProjection(t *testing.T) {
	requireWindowsAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	provider := filepath.Join(filepath.Dir(fixture.configPath), "stage-provider.json")
	t.Setenv("STEERLINE_STAGE_PROVIDER", provider)
	raw, err := os.ReadFile(fixture.configPath)
	require(t, err)
	var config map[string]any
	require(t, json.Unmarshal(raw, &config))
	config["pr"] = map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "owner/repo", "query": executionbackend.Command{Executable: os.Args[0], Args: []string{"--fake", "staged-pr"}}}
	raw, err = json.Marshal(config)
	require(t, err)
	require(t, os.WriteFile(fixture.configPath, raw, 0600))
	fixture.config, err = steerlineapp.LoadConfig(fixture.configPath)
	require(t, err)
	require(t, fixture.config.ResolveIdentity(ctx))
	createIntegrationTask(t, ctx, fixture, 901)
	service := startIntegrationService(t, ctx, fixture.config, &integrationLifecycleResolver{})
	defer service.stop(t)
	claimIntegrationTask(t, ctx, fixture, 901, service.instanceID)
	ready := waitIntegrationRun(t, ctx, service, fixture.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.WorkflowStatus == "waiting_external" && run.WorkflowStage == executionmodel.StepAwaitMerge
	}, 3*time.Minute)
	if len(ready.run.StageResults) != 3 || ready.run.ReportToolPolicy == "" || ready.run.StageDiagnostic != "" {
		t.Fatalf("controlled worker did not produce the expected real staged query facts: %+v", ready.run)
	}
	endpoint, client, product := startIntegrationHTTPAPI(t, ctx, fixture, service)
	defer endpoint.Close(context.Background())
	defer client.Close()
	defer product.Close()
	byRunID, err := client.Run(ctx, ready.run.RunID)
	require(t, err)
	assertWorkerStageProjection(t, ready.run, byRunID, "HTTP run query")
	snapshot, err := client.Snapshot(ctx, "901")
	require(t, err)
	if snapshot.Connection.Snapshot.Page == nil {
		t.Fatal("HTTP project snapshot omitted its native page")
	}
	var fromPage *api.CandidateSummary
	for _, candidate := range snapshot.Connection.Snapshot.Page.Runs {
		if candidate != nil && candidate.RunID == ready.run.RunID {
			fromPage = candidate
			break
		}
	}
	if fromPage == nil {
		t.Fatal("HTTP page query omitted the staged run")
	}
	assertWorkerStageProjection(t, ready.run, *fromPage, "HTTP page query")
}

func assertWorkerStageProjection(t *testing.T, source *reconstructiontypes.RunView, got api.CandidateSummary, query string) {
	t.Helper()
	if got.FlowRef != source.FlowRef || got.WorkflowStage != source.WorkflowStage || got.WorkflowStatus != source.WorkflowStatus ||
		got.StageDiagnostic != source.StageDiagnostic || got.ReportToolPolicy != source.ReportToolPolicy || len(got.StageResults) != len(source.StageResults) {
		t.Fatalf("%s changed stage identity/status/policy: source=%+v got=%+v", query, source, got)
	}
	for i, result := range source.StageResults {
		mapped := got.StageResults[i]
		if mapped.StepID != result.StepID || mapped.CommandID != result.CommandID || mapped.TurnID != result.TurnID || mapped.InputVersion != result.InputVersion ||
			mapped.Succeeded != result.Succeeded || mapped.FinalVerified != result.FinalVerified || mapped.Final != result.Final ||
			mapped.FinalSource != result.FinalSource || mapped.Failure != result.Failure || mapped.Outcome != result.Outcome {
			t.Fatalf("%s changed staged result %d: source=%+v mapped=%+v", query, i, result, mapped)
		}
	}
}

func addAlternateLocalProfile(t *testing.T, fixture *integrationFixture) {
	t.Helper()
	profiles := fixture.config.AvailableExecutionProfiles()
	if len(profiles) != 1 || profiles[0].ID != "fixture" {
		t.Fatalf("fixture profile catalog changed before adding the alternate: %+v", profiles)
	}
	alternate := profiles[0]
	alternate.ID, alternate.Name = "fixture-alt", "Integration alternate"
	alternate.ImplementationConfigs = append(alternate.ImplementationConfigs[:0:0], alternate.ImplementationConfigs...)
	found := false
	for i := range alternate.ImplementationConfigs {
		if alternate.ImplementationConfigs[i].ImplementationKey != codexworker.ProfileCodecKey {
			continue
		}
		settings, err := codexworker.DecodeProfileConfig(alternate.ImplementationConfigs[i].Payload)
		require(t, err)
		settings.Model, settings.ReasoningEffort = "fixture-alt-model", "high"
		alternate.ImplementationConfigs[i].Payload, err = codexworker.EncodeProfileConfig(settings)
		require(t, err)
		found = true
	}
	if !found {
		t.Fatal("fixture did not have a Codex implementation profile to vary")
	}
	globalPath := steerlineapp.GlobalExecutionConfigPath()
	data, err := os.ReadFile(globalPath)
	require(t, err)
	var global map[string]json.RawMessage
	require(t, json.Unmarshal(data, &global))
	var configured []execution.ExecutionProfile
	require(t, json.Unmarshal(global["execution_profiles"], &configured))
	configured = append(configured, alternate)
	global["execution_profiles"] = mustJSON(configured)
	require(t, os.WriteFile(globalPath, mustJSON(global), 0600))
	fixture.config, err = steerlineapp.LoadConfig(fixture.configPath)
	require(t, err)
	require(t, fixture.config.ResolveIdentity(context.Background()))
	if fixture.config.DefaultProfileID() != "fixture" || len(fixture.config.AvailableExecutionProfiles()) != 2 {
		t.Fatalf("alternate profile changed the configured default or catalog size: default=%q profiles=%+v", fixture.config.DefaultProfileID(), fixture.config.AvailableExecutionProfiles())
	}
}

func addUnsupportedOrcaProfile(t *testing.T, fixture *integrationFixture) {
	t.Helper()
	globalPath := steerlineapp.GlobalExecutionConfigPath()
	data, err := os.ReadFile(globalPath)
	require(t, err)
	var global map[string]json.RawMessage
	require(t, json.Unmarshal(data, &global))
	var profiles []execution.ExecutionProfile
	require(t, json.Unmarshal(global["execution_profiles"], &profiles))
	payload, err := json.Marshal(transport.Profile{Executable: "missing-orca-executable", RepositorySelector: "id:repo", WaitTimeout: "30s"})
	require(t, err)
	profiles = append(profiles, execution.ExecutionProfile{ID: "orca-terminal", Name: "Orca terminal", Host: "orca", Agent: "opaque-agent", ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: transport.ProfileKey, CodecVersion: transport.ProfileVersion, Payload: payload}}})

	global["execution_profiles"] = mustJSON(profiles)
	require(t, os.WriteFile(globalPath, mustJSON(global), 0600))
	fixture.config, err = steerlineapp.LoadConfig(fixture.configPath)
	require(t, err)
	require(t, fixture.config.ResolveIdentity(context.Background()))
}

func claimIntegrationTaskWithProfile(t *testing.T, ctx context.Context, fixture integrationFixture, taskID int64, serviceInstanceID, profileID string) {
	t.Helper()
	store, err := steerlineapp.OpenNativeTaskCommandStore(ctx, &fixture.config)
	require(t, err)
	commands := appExecutionCommands(t, store, fixture.config)
	page, err := productquery.NewService(store, fixture.config.Identity()).ShowNativeTask(ctx, fmt.Sprint(taskID))
	require(t, err)
	var fixed *executionmodel.FixedExecutionProfile
	for _, profile := range fixture.config.AvailableExecutionProfiles() {
		if profile.ID == profileID {
			value := profile.Freeze(fixture.config.ExecutionDirectory, fixture.config.Identity())
			fixed = &value
			break
		}
	}
	if fixed == nil || len(page.Tasks) != 1 {
		t.Fatalf("fixture task/profile not found: tasks=%d profile=%q", len(page.Tasks), profileID)
	}
	_, err = commands.ClaimNativeTask(ctx, fixture.config.ExecutionConfig(), execution.TaskClaimOptions{
		TaskID: fmt.Sprint(taskID), RequestID: randomRequestID(t), ExpectedTaskVersion: page.Tasks[0].Version,
		ExpectedAuthorityVersion: page.AuthorityVersion, RequestedBy: "operator", Explicit: true,
		ExecutionProfile: fixed, ServiceInstanceID: serviceInstanceID,
	})
	require(t, err)
	require(t, store.Close())
}

func TestAPIOriginalCommandReceipt(t *testing.T) {
	requireWindowsAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	createIntegrationTask(t, ctx, fixture, 902)
	service := startIntegrationService(t, ctx, fixture.config, &integrationLifecycleResolver{})
	defer service.stop(t)
	endpoint, client, product := startIntegrationHTTPAPI(t, ctx, fixture, service)
	defer endpoint.Close(context.Background())
	defer product.Close()
	page, err := client.Snapshot(ctx, "902")
	require(t, err)
	if page.Connection.Snapshot.Page == nil {
		t.Fatal("controlled service query lacked a control revision")
	}
	revision := page.Connection.Snapshot.Page.Control.Revision
	requestID := randomRequestID(t)
	envelope := httpapi.CommandEnvelope{RequestID: requestID, ProjectID: httpapi.ProjectID(fixture.config.Identity()), InstanceID: service.instanceID,
		Action: "pause_dispatch", TargetID: "dispatch", ExpectedVersion: revision, ExpectedControlRevision: revision, Payload: []byte(`{}`)}
	first, err := client.SubmitCommand(ctx, envelope)
	require(t, err)
	if first.ControlRequest == nil || first.Status != "submitted" {
		t.Fatalf("HTTP command did not return its original request identity: %+v", first)
	}
	deadline, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	var receipt httpapi.CommandResponse
	for {
		receipt, err = client.Command(deadline, requestID)
		if err == nil && receipt.ControlReceipt != nil && receipt.ControlReceipt.Status == "applied" {
			break
		}
		if deadline.Err() != nil {
			requestBytes, _ := os.ReadFile(filepath.Join(fixture.config.RunDir, "control", "requests", requestID, "request.json"))
			receiptBytes, _ := os.ReadFile(filepath.Join(fixture.config.RunDir, "control", "receipts", requestID+".json"))
			runtimeState, runtimeErr := steerlineapp.LoadDatabaseRuntime(fixture.config)
			t.Fatalf("original control receipt did not settle: %+v / %v request=%s receipt=%s runtime=%+v runtime_error=%v",
				receipt, err, requestBytes, receiptBytes, runtimeState, runtimeErr)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if receipt.Original == nil || receipt.Original.RequestID != requestID || receipt.ControlReceipt.RequestID != requestID || receipt.Status != "applied" {
		t.Fatalf("GET /commands did not return the original control receipt: %+v", receipt)
	}
	duplicate, err := client.SubmitCommand(ctx, envelope)
	require(t, err)
	if duplicate.RequestID != requestID || duplicate.ControlRequest == nil {
		t.Fatalf("retry changed the original control request: %+v", duplicate)
	}
	unsupported := envelope
	unsupported.RequestID = randomRequestID(t)
	unsupported.Action = "planning_publish"
	if _, err = client.SubmitCommand(ctx, unsupported); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unknown action did not return unsupported: %v", err)
	}
	unsupportedReceipt, err := client.Command(ctx, unsupported.RequestID)
	if err != nil || unsupportedReceipt.Status != "rejected" || unsupportedReceipt.Code != "unsupported" {
		t.Fatalf("unsupported action did not retain its protocol rejection: %+v %v", unsupportedReceipt, err)
	}
}

func TestAPIReconnectAndUnsupported(t *testing.T) {
	requireWindowsAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	service := startIntegrationService(t, ctx, fixture.config, &integrationLifecycleResolver{})
	defer service.stop(t)
	first, client, product := startIntegrationHTTPAPI(t, ctx, fixture, service)
	if client.Identity().InstanceID != service.instanceID {
		t.Fatal("initial client did not bind to the live service")
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	second, secondClient, err := reconnectIntegrationHTTPAPI(ctx, fixture, service, product)
	require(t, err)
	defer second.Close(context.Background())
	defer product.Close()
	if secondClient.Identity().InstanceID != service.instanceID || !secondClient.Identity().Ready {
		t.Fatalf("reconnected client did not query the same live instance: %+v", secondClient.Identity())
	}
	if _, err = httpclient.Open(ctx, fixture.config.RunDir, fixture.config.Identity()+"/wrong-project"); err == nil || !strings.Contains(err.Error(), "different project") {
		t.Fatalf("wrong project was accepted by discovery: %v", err)
	}
}

func startIntegrationHTTPAPI(t *testing.T, ctx context.Context, fixture integrationFixture, service *runningIntegrationService) (*httpapi.Server, *httpclient.Client, *productclient.Client) {
	t.Helper()
	product, err := steerlineapp.OpenClient(ctx, fixture.configPath)
	require(t, err)
	server, client, err := newIntegrationHTTPAPI(ctx, fixture, service, product)
	require(t, err)
	return server, client, product
}

func reconnectIntegrationHTTPAPI(ctx context.Context, fixture integrationFixture, service *runningIntegrationService, product *productclient.Client) (*httpapi.Server, *httpclient.Client, error) {
	server, client, err := newIntegrationHTTPAPI(ctx, fixture, service, product)
	return server, client, err
}

func newIntegrationHTTPAPI(ctx context.Context, fixture integrationFixture, service *runningIntegrationService, product *productclient.Client) (*httpapi.Server, *httpclient.Client, error) {
	identity := service.service.State.Service.ProcessIdentity
	if identity == nil {
		return nil, nil, fmt.Errorf("controlled service omitted process identity")
	}
	server, err := httpapi.Prepare(ctx, product, httpapi.Options{RunDir: fixture.config.RunDir, ProjectIdentity: fixture.config.Identity(),
		InstanceID: service.instanceID, Process: httpapi.ProcessIdentity{Kind: identity.Kind, PID: identity.PID, Created: identity.Created, Executable: identity.Executable},
		Capabilities: append([]string(nil), service.service.State.Service.Capabilities...)})
	if err != nil {
		return nil, nil, err
	}
	if err = server.SetReady(service.instanceID, true, ""); err != nil {
		return nil, nil, err
	}
	if err = server.Start(); err != nil {
		return nil, nil, err
	}
	client, err := httpclient.Open(ctx, fixture.config.RunDir, fixture.config.Identity())
	if err != nil {
		return nil, nil, err
	}
	return server, client, nil
}

type integrationInboxPublisher func(context.Context, api.ControlInput) (api.ControlRequest, error)
type integrationInboxHook func(context.Context, api.ControlInput, integrationInboxPublisher) (api.ControlRequest, error)

type integrationInboxInterceptor struct {
	productclient.ControlPort
	inbox     productclient.ControlInboxPort
	mu        sync.Mutex
	hook      integrationInboxHook
	forwarded map[string]int
}

func (c *integrationInboxInterceptor) SetHook(hook integrationInboxHook) {
	c.mu.Lock()
	c.hook = hook
	c.mu.Unlock()
}

func (c *integrationInboxInterceptor) SubmitInbox(ctx context.Context, input api.ControlInput) (api.ControlRequest, error) {
	c.mu.Lock()
	hook := c.hook
	c.mu.Unlock()
	if hook != nil {
		return hook(ctx, input, c.publish)
	}
	return c.publish(ctx, input)
}

func (c *integrationInboxInterceptor) publish(ctx context.Context, input api.ControlInput) (api.ControlRequest, error) {
	c.mu.Lock()
	if c.forwarded == nil {
		c.forwarded = make(map[string]int)
	}
	c.forwarded[input.RequestID]++
	c.mu.Unlock()
	return c.inbox.SubmitInbox(ctx, input)
}

func (c *integrationInboxInterceptor) PublishAsFormalConsumer(ctx context.Context, input api.ControlInput) (api.ControlRequest, error) {
	return c.publish(ctx, input)
}

func (c *integrationInboxInterceptor) Forwarded(requestID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.forwarded[requestID]
}

func startIntegrationHTTPAPIWithInboxInterceptor(t *testing.T, ctx context.Context, fixture integrationFixture, service *runningIntegrationService) (*httpapi.Server, *httpclient.Client, *productclient.Client, *integrationInboxInterceptor) {
	t.Helper()
	var interceptor *integrationInboxInterceptor
	product, err := productclient.Open(ctx, api.ProjectRef{ConfigPath: fixture.configPath}, func(ctx context.Context, ref api.ProjectRef) (productclient.Resources, error) {
		resources, err := steerlineapp.ClientResources(ctx, ref)
		if err != nil {
			return productclient.Resources{}, err
		}
		inbox, ok := resources.Control.(productclient.ControlInboxPort)
		if !ok {
			return productclient.Resources{}, fmt.Errorf("real app control port does not implement the request inbox")
		}
		interceptor = &integrationInboxInterceptor{ControlPort: resources.Control, inbox: inbox}
		resources.Control = interceptor
		return resources, nil
	})
	require(t, err)
	server, client, err := newIntegrationHTTPAPI(ctx, fixture, service, product)
	require(t, err)
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Close(closeCtx)
		_ = client.Close()
		_ = product.Close()
	})
	return server, client, product, interceptor
}

func randomRequestID(t *testing.T) string {
	t.Helper()
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw[:])
}

func requireWindowsAPI(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("local client protocol acceptance requires real Windows service identity and ACLs")
	}
}
