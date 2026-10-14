//go:build windows

package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
	"golang.org/x/sys/windows"
)

func TestTUIHTTPStopReopenUsesOriginalProcess(t *testing.T) {
	if os.Getenv("STEERLINE_TEST_STOP_CHILD") == "1" {
		time.Sleep(time.Minute)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestTUIHTTPStopReopenUsesOriginalProcess$")
	child.Env = append(os.Environ(), "STEERLINE_TEST_STOP_CHILD=1")
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer child.Process.Kill()
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(child.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	var created, exit, kernel, user windows.Filetime
	err = windows.GetProcessTimes(handle, &created, &exit, &kernel, &user)
	windows.CloseHandle(handle)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	locks := []string{filepath.Join(home, "repository", "steerline.lock"), filepath.Join(home, "state", "steerline.lock")}
	for _, path := range locks {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	identity := wire.Identity{Protocol: wire.Protocol, ProjectID: wire.ProjectID("repo"), RepositoryIdentity: "repo", InstanceID: "original", Process: wire.ProcessIdentity{Kind: "os", PID: child.Process.Pid, Created: fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime), Executable: executable}, LockPaths: locks}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(wire.ErrorResponse{Code: "command_not_found"})
			return
		}
		var e wire.CommandEnvelope
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			t.Error(err)
			return
		}
		json.NewEncoder(w).Encode(wire.CommandResponse{RequestID: e.RequestID, ProjectID: e.ProjectID, InstanceID: e.InstanceID, Action: e.Action, TargetID: e.TargetID, Original: &e, Status: "applied", ControlRequest: &api.ControlRequest{RequestID: e.RequestID, InstanceID: e.InstanceID, Action: e.Action}, ControlReceipt: &api.ControlReceipt{RequestID: e.RequestID, InstanceID: e.InstanceID, Action: e.Action, Status: "applied", Code: "applied"}})
	}))
	defer server.Close()
	ref := api.ProjectRef{Identity: "repo", ApplicationHome: home, RunDir: filepath.Join(home, "offline")}
	c := newTestConsole(ref, &Client{baseURL: server.URL, project: identity.ProjectID, identity: identity, http: server.Client()}, NewPlatformStopObserver())
	id := strings.Repeat("a", 32)
	revision := uint64(7)
	if _, err = c.SubmitControl(context.Background(), api.ControlInput{Action: api.StopService, RequestID: id, InstanceID: identity.InstanceID, ExpectedRevision: &revision}); err != nil {
		t.Fatal(err)
	}
	if receipt, err := c.Receipt(context.Background(), id); err != nil || receipt.Code != "stop_exit_unverified" {
		t.Fatalf("live original process was considered stopped: %+v %v", receipt, err)
	}
	c.Close()
	reopened := NewConsole(context.Background(), ref, NewPlatformStopObserver())
	defer reopened.Close()
	// A replacement connection cannot provide proof for the original process.
	reopened.identity = wire.Identity{InstanceID: "replacement", Process: wire.ProcessIdentity{Kind: "os", PID: os.Getpid(), Created: "replacement"}}
	if receipt, err := reopened.Receipt(context.Background(), id); err != nil || receipt.Code != "stop_exit_unverified" {
		t.Fatalf("reopening lost the pending original observation: %+v %v", receipt, err)
	}
	if err = child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	receipt, err := reopened.Receipt(context.Background(), id)
	if err != nil || receipt.Code != "applied" || receipt.Message != "原停服请求已应用，原进程退出与锁释放已核实" {
		t.Fatalf("original process could not be reconciled after reopening: %+v %v", receipt, err)
	}
	stored, err := reopened.journal.read(id)
	if err != nil || stored.StopIdentity == nil || stored.StopIdentity.Process != identity.Process {
		t.Fatal("original authenticated identity was not persisted", err)
	}
	changed := *stored.StopIdentity
	changed.Process.Created = "different"
	stored.StopIdentity = &changed
	if _, err = reopened.journal.save(context.Background(), stored); err == nil {
		t.Fatal("immutable original stop identity was replaced")
	}
}
