package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
)

func startupEndpoint(t *testing.T, home string, c component, backend *localservice.Endpoint) *localservice.Endpoint {
	t.Helper()
	endpoint, err := localservice.Prepare(home, c.dir, c.protocol, "127.0.0.1:0")
	require(t, err)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !endpoint.Authorized(r) {
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case c.identity:
			localservice.JSON(w, 200, endpoint.Identity)
		case "/v2/application/projects":
			localservice.JSON(w, 200, contract.Catalog{Projects: []contract.Project{}})
		case "/native/backend":
			localservice.JSON(w, 200, backend.Identity)
		default:
			w.WriteHeader(404)
		}
	})}
	go server.Serve(endpoint.Listener)
	t.Cleanup(func() { server.Close(); endpoint.Close() })
	return endpoint
}

func TestUnifiedStartupConcurrentReuseAndMissingWeb(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	backend, web := components(home)
	backendEndpoint := startupEndpoint(t, home, backend, nil)
	if _, err := StartApplication(context.Background(), true); err == nil || !strings.Contains(err.Error(), "missing product executable") {
		t.Fatal("missing Web not reported", err)
	}
	if !localservice.Alive(backendEndpoint.Identity) {
		t.Fatal("Web failure stopped backend")
	}
	webEndpoint := startupEndpoint(t, home, web, backendEndpoint)
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, err := StartApplication(context.Background(), true)
			if err == nil && (status.Backend.Identity.InstanceID != backendEndpoint.Identity.InstanceID || status.Web.Identity.InstanceID != webEndpoint.Identity.InstanceID) {
				err = errors.New("concurrent start replaced original instance")
			}
			failures <- err
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require(t, err)
	}
}

func TestUnifiedStartupUnknownCreationCannotTakeOver(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	backend, _ := components(home)
	require(t, localservice.Write(backend.dir, "launch.json", launchRecord{Attempt: localservice.Token()}))
	status, err := StartApplication(context.Background(), true)
	if err == nil || !strings.Contains(err.Error(), "creation is unverified") || status.Backend.State != "unverified" {
		t.Fatal(status, err)
	}
	if _, err := os.Stat(filepath.Join(home, "state")); !os.IsNotExist(err) {
		t.Fatal("unknown startup initialized project")
	}
}

func TestUnifiedStartupLifecycleShutdownEvidenceFencesOriginalHost(t *testing.T) {
	home := t.TempDir()
	backend, _ := components(home)
	process, err := httpclient.CurrentProcessIdentity()
	require(t, err)
	process.Created = "previous-os-generation"
	homeID, err := localservice.HomeID(home)
	require(t, err)
	id := contract.Identity{Protocol: contract.Protocol, HomeID: homeID, InstanceID: localservice.Token(), Process: process, Address: "127.0.0.1:18742", Ready: true}
	require(t, localservice.Write(backend.dir, "instance.json", id))
	status, err := observeComponent(context.Background(), home, backend)
	require(t, err)
	if status.State != "exited" {
		t.Fatal("confirmed stale generation was not distinguished", status)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := verifyBackendStop(ctx, home, backend, &id); err == nil {
		t.Fatal("missing shutdown result became success")
	}
	original := contract.Shutdown{Version: 1, Identity: id, Clean: true, Projects: []contract.ProjectShutdown{{ConfigID: "one.json", ProjectID: strings.Repeat("a", 64), InstanceID: strings.Repeat("b", 32), LocalClosed: true, LocksReleased: true, ResourcesSettled: true}}}
	for _, kind := range []string{"wrong-instance", "local-unclosed", "worker-unknown", "unclean", "missing-projects", "duplicate-project"} {
		t.Run(kind, func(t *testing.T) {
			result := original
			result.Projects = append([]contract.ProjectShutdown(nil), original.Projects...)
			switch kind {
			case "wrong-instance":
				result.Identity.InstanceID = localservice.Token()
			case "local-unclosed":
				result.Projects[0].LocalClosed = false
			case "worker-unknown":
				result.Projects[0].ResourcesSettled = false
			case "missing-projects":
				result.Projects = nil
			case "duplicate-project":
				result.Projects = append(result.Projects, result.Projects[0])
			case "unclean":
				result.Clean = false
			}
			require(t, localservice.Write(backend.dir, "shutdown.json", result))
			if err := verifyBackendStop(ctx, home, backend, &id); err == nil {
				t.Fatal("unverified cleanup became success")
			}
		})
	}
	require(t, localservice.Write(backend.dir, "shutdown.json", original))
	require(t, verifyBackendStop(ctx, home, backend, &id))
	require(t, verifyBackendStop(ctx, home, backend, &id))
	// A corrupt result must not fall back to the last successful observation.
	require(t, localservice.Write(backend.dir, "shutdown.json", map[string]string{"invalid": "result"}))
	if err := verifyBackendStop(ctx, home, backend, &id); err == nil {
		t.Fatal("corrupt shutdown became success")
	}
}

func TestUnifiedStartupDefaultOpenUsesInstalledWeb(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	backend, web := components(home)
	b := startupEndpoint(t, home, backend, nil)
	startupEndpoint(t, home, web, b)
	executable, err := os.Executable()
	require(t, err)
	data, err := os.ReadFile(executable)
	require(t, err)
	require(t, os.MkdirAll(filepath.Dir(web.exe), 0700))
	require(t, os.WriteFile(web.exe, data, 0700))
	record := filepath.Join(home, "open-result.json")
	t.Setenv("STEERLINE_TEST_OPENER_RECORD", record)
	_, err = StartApplication(context.Background(), false)
	require(t, err)
	var fact struct {
		Executable, Home, Cwd string
		Args                  []string
	}
	raw, err := os.ReadFile(record)
	require(t, err)
	require(t, json.Unmarshal(raw, &fact))
	if !strings.EqualFold(fact.Executable, web.exe) || fact.Home != home || fact.Cwd != home || len(fact.Args) != 1 || fact.Args[0] != "open" {
		t.Fatalf("incorrect opener target: %+v", fact)
	}
	t.Setenv("STEERLINE_TEST_OPENER_FAIL", "1")
	_, err = StartApplication(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "browser opening failed") || !localservice.Alive(b.Identity) {
		t.Fatal("opener failure changed resident ownership", err)
	}
}
