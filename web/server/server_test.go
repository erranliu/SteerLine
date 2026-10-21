package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	privatefs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

type fakeClient struct {
	identity    httpcontract.Identity
	appIdentity contract.Identity
	calls       int
	route       string
	envelope    httpcontract.CommandEnvelope
	getError    error
}

func (c *fakeClient) Identity() contract.Identity { return c.appIdentity }
func (c *fakeClient) RefreshIdentity(context.Context) (contract.Identity, error) {
	return c.appIdentity, nil
}
func (c *fakeClient) Get(_ context.Context, route string, target any) error {
	c.route = route
	if c.getError != nil {
		return c.getError
	}
	return json.Unmarshal([]byte(`{"database_revision":17,"Tasks":["原正文"]}`), target)
}
func (c *fakeClient) Post(_ context.Context, route string, body, target any) error {
	c.calls++
	c.route = route
	c.envelope = body.(httpcontract.CommandEnvelope)
	data, _ := json.Marshal(httpcontract.CommandResponse{RequestID: c.envelope.RequestID, ProjectID: c.envelope.ProjectID, InstanceID: c.envelope.InstanceID, Action: c.envelope.Action, TargetID: c.envelope.TargetID, ExpectedVersion: c.envelope.ExpectedVersion, Original: &c.envelope, Status: "accepted"})
	return json.Unmarshal(data, target)
}
func (c *fakeClient) Close() error { return nil }
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}
func fixture(t *testing.T) (*Server, *fakeClient) {
	t.Helper()
	client := &fakeClient{identity: httpcontract.Identity{ProjectID: httpcontract.ProjectID("repository"), InstanceID: "original-instance", Ready: true}}
	home := t.TempDir()
	hid, _ := localservice.HomeID(home)
	client.appIdentity = contract.Identity{HomeID: hid, Protocol: contract.Protocol, InstanceID: "app"}
	s, err := Prepare(Options{Address: freeAddress(t), Home: home, LauncherDir: LauncherDirectory(home), Client: client, Assets: fstest.MapFS{"index.html": {Data: []byte("<h1>real build</h1>")}, "assets/main.js": {Data: []byte("export default 1")}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(context.Background()); s.listener.Close() })
	return s, client
}
func request(s *Server, method, route, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+s.options.Address+route, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:12121"
	if method == "POST" {
		r.Header.Set("Origin", "http://"+s.options.Address)
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func login(t *testing.T, s *Server) (*http.Cookie, string) {
	t.Helper()
	r := httptest.NewRequest("POST", "http://"+s.options.Address+"/auth/issue", nil)
	r.RemoteAddr = "127.0.0.1:12121"
	r.Header.Set("Authorization", "Bearer "+s.secret)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var ticket map[string]string
	json.Unmarshal(w.Body.Bytes(), &ticket)
	w = request(s, "POST", "/auth/session", `{"ticket":"`+ticket["ticket"]+`"}`, nil, "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var value map[string]string
	json.Unmarshal(w.Body.Bytes(), &value)
	return w.Result().Cookies()[0], value["csrf_token"]
}

func TestWebAuthenticationAndOriginalForwarding(t *testing.T) {
	s, client := fixture(t)
	route := "/api/v2/application/projects/" + client.identity.ProjectID + "/commands"
	if w := request(s, "GET", route+"/"+strings.Repeat("a", 32), "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookie, csrf := login(t, s)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("browser cookie policy missing")
	}
	envelope := httpcontract.CommandEnvelope{RequestID: strings.Repeat("a", 32), ProjectID: client.identity.ProjectID, InstanceID: client.identity.InstanceID, Action: "start_task", TargetID: "8", ExpectedVersion: 4, ExpectedControlRevision: 7, Payload: json.RawMessage(`{"execution_profile_id":"original"}`)}
	body, _ := json.Marshal(envelope)
	for _, token := range []string{"", "wrong"} {
		if w := request(s, "POST", route, string(body), cookie, token); w.Code != 403 || client.calls != 0 {
			t.Fatal("CSRF passed to business")
		}
	}
	w := request(s, "POST", route, string(body), cookie, csrf)
	if w.Code != 200 || client.calls != 1 || client.envelope.RequestID != envelope.RequestID || client.envelope.ExpectedControlRevision != 7 || string(client.envelope.Payload) != string(envelope.Payload) {
		t.Fatalf("envelope changed: %d %s %+v", w.Code, w.Body.String(), client.envelope)
	}
	envelope.ProjectID = httpcontract.ProjectID("other")
	body, _ = json.Marshal(envelope)
	if w = request(s, "POST", route, string(body), cookie, csrf); w.Code != 409 || client.calls != 1 {
		t.Fatal("foreign project reached application")
	}
	client.getError = &httpclient.ClientError{Status: 404, Code: "command_not_found", Reason: "both authorities have no original request"}
	w = request(s, "GET", route+"/"+strings.Repeat("a", 32), "", cookie, "")
	if w.Code != 404 || !strings.Contains(w.Body.String(), "command_not_found") {
		t.Fatal("formal absence was lost")
	}
	client.getError = errors.New("SQLite unavailable")
	w = request(s, "GET", route+"/"+strings.Repeat("a", 32), "", cookie, "")
	if w.Code != 503 || strings.Contains(w.Body.String(), "command_not_found") {
		t.Fatal("failure became absence")
	}
}

func TestEnvironmentCheckBrowserRoutes(t *testing.T) {
	s, client := fixture(t)
	if w := request(s, "GET", "/projects/"+client.identity.ProjectID+"/environment-checks", "", nil, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "real build") {
		t.Fatal("direct check URL did not serve SPA", w.Code, w.Body.String())
	}
	cookie, csrf := login(t, s)
	base := "/api/v2/application/projects/" + client.identity.ProjectID
	for _, path := range []string{"/environment-checks/options", "/environment-checks", "/environment-checks/" + strings.Repeat("a", 32)} {
		if w := request(s, "GET", base+path, "", cookie, ""); w.Code != 200 || client.route != "/projects/"+client.identity.ProjectID+path {
			t.Fatal("check read was not forwarded", w.Code, w.Body.String(), client.route)
		}
	}
	e := httpcontract.CommandEnvelope{RequestID: strings.Repeat("a", 32), ProjectID: client.identity.ProjectID, InstanceID: client.identity.InstanceID, Action: "start_environment_check", TargetID: client.identity.ProjectID, Payload: json.RawMessage(`{"profile_id":"fixture","access_mode":"full_access"}`)}
	for _, action := range []string{"start_environment_check", "stop_environment_check"} {
		e.Action = action
		if action == "stop_environment_check" {
			e.RequestID = strings.Repeat("b", 32)
			e.TargetID = strings.Repeat("a", 32)
			e.ExpectedVersion = 3
			e.Payload = json.RawMessage(`{}`)
		}
		body, _ := json.Marshal(e)
		before := client.calls
		if w := request(s, "POST", base+"/commands", string(body), cookie, ""); w.Code != 403 || client.calls != before {
			t.Fatal("check mutation bypassed CSRF")
		}
		if w := request(s, "POST", base+"/commands", string(body), cookie, csrf); w.Code != 200 || client.calls != before+1 || client.envelope.Action != e.Action || client.envelope.TargetID != e.TargetID || client.envelope.ExpectedVersion != e.ExpectedVersion || string(client.envelope.Payload) != string(e.Payload) {
			t.Fatal("check authorization changed", w.Code, client.envelope)
		}
	}
}

func TestTRQueryBrowserRoutesAndReadOnlyBoundary(t *testing.T) {
	s, client := fixture(t)
	if w := request(s, "GET", "/projects/"+client.identity.ProjectID+"/tests?test=rules", "", nil, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "real build") {
		t.Fatal("test deep link did not serve SPA", w.Code, w.Body.String())
	}
	cookie, _ := login(t, s)
	base := "/api/v2/application/projects/" + client.identity.ProjectID
	for _, path := range []string{"/tests/catalogue", "/tests/records?test_id=rules&outcome=failed", "/tests/records/" + strings.Repeat("a", 32), "/tests/records/" + strings.Repeat("a", 32) + "/evidence"} {
		if w := request(s, "GET", base+path, "", cookie, ""); w.Code != 200 || client.route != "/projects/"+client.identity.ProjectID+path {
			t.Fatal("record query not forwarded", w.Code, w.Body.String(), client.route)
		}
		if w := request(s, "GET", base+path, "", nil, ""); w.Code != 401 {
			t.Fatal("record query bypassed authentication", w.Code)
		}
	}
	if w := request(s, "POST", base+"/tests/records", "{}", cookie, ""); w.Code < 400 || client.calls != 0 {
		t.Fatal("record query created work", w.Code, client.calls)
	}
}
func TestWebOriginHostAndStaticBoundaries(t *testing.T) {
	s, _ := fixture(t)
	for _, change := range []func(*http.Request){func(r *http.Request) { r.Host = "attacker.test" }, func(r *http.Request) { r.RemoteAddr = "192.168.1.2:1234" }, func(r *http.Request) { r.Header.Set("Origin", "http://evil.test") }, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }} {
		r := httptest.NewRequest("GET", "http://"+s.options.Address+"/", nil)
		r.RemoteAddr = "127.0.0.1:12"
		change(r)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("source policy passed: %d", w.Code)
		}
	}
	for route, status := range map[string]int{"/": 200, "/projects/" + strings.Repeat("a", 64) + "/dag": 200, "/projects/" + strings.Repeat("a", 64) + "/runs": 200, "/projects/" + strings.Repeat("a", 64) + "/settings": 200, "/assets/main.js": 200, "/assets/missing.js": 404, "/auth/missing": 404, "/../secret": 400, "/assets%2fmain.js": 400} {
		w := request(s, "GET", route, "", nil, "")
		if w.Code != status {
			t.Fatalf("%s => %d want %d", route, w.Code, status)
		}
		if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("missing document policy")
		}
	}
}
func TestWebTicketsAreSingleUseAndExpire(t *testing.T) {
	s, _ := fixture(t)
	s.tickets["one"] = time.Now().Add(time.Minute)
	w := request(s, "POST", "/auth/session", `{"ticket":"one"}`, nil, "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if request(s, "POST", "/auth/session", `{"ticket":"one"}`, nil, "").Code != 403 {
		t.Fatal("ticket reused")
	}
	s.tickets["expired"] = time.Now().Add(-time.Second)
	if request(s, "POST", "/auth/session", `{"ticket":"expired"}`, nil, "").Code != 403 {
		t.Fatal("expired ticket accepted")
	}
	cookie, csrf := login(t, s)
	s.sessions[cookie.Value] = session{CSRF: csrf, Expires: time.Now().Add(-time.Second)}
	if request(s, "GET", "/auth/status", "", cookie, "").Code != 401 {
		t.Fatal("expired browser session accepted")
	}
}
func TestWebAddressAndPortConflict(t *testing.T) {
	for _, value := range []string{"127.0.0.1:0", "localhost:8080", "0.0.0.0:8080", "127.0.0.1:65536", "127.0.0.1:123x"} {
		if ValidateAddress(value) == nil {
			t.Fatal("unstable/nonloopback address accepted", value)
		}
	}
	s, c := fixture(t)
	other, err := Prepare(Options{Address: s.options.Address, Home: s.options.Home, LauncherDir: t.TempDir(), Client: c, Assets: s.assets})
	if err == nil {
		other.Close(context.Background())
		t.Fatal("conflict silently switched port")
	}
}
func TestWebLauncherPrivateDiscoveryAndInstance(t *testing.T) {
	s, c := fixture(t)
	s.Start()
	url, err := OpeningURL(context.Background(), s.options.Home, "")
	if err != nil || !strings.HasPrefix(url, "http://"+s.options.Address+"/#ticket=") {
		t.Fatalf("opening failed: %s %v", url, err)
	}
	if strings.Contains(url, s.secret) {
		t.Fatal("launcher secret leaked")
	}
	if err = privatefs.VerifyPrivateFile(filepath.Join(s.options.LauncherDir, "discovery.json")); err != nil {
		t.Fatal(err)
	}
	if _, err = localservice.Read(t.TempDir(), s.options.LauncherDir, contract.WebProtocol); err == nil {
		t.Fatal("foreign home accepted")
	}
	_ = c

}
func TestWebBoundedWritesAndNoBrowserStop(t *testing.T) {
	s, c := fixture(t)
	cookie, csrf := login(t, s)
	route := "/api/v2/application/projects/" + c.identity.ProjectID + "/commands"
	w := request(s, "POST", route, `{"payload":"`+strings.Repeat("x", (1<<20)+1)+`"}`, cookie, csrf)
	if w.Code != 413 || c.calls != 0 {
		t.Fatalf("bound => %d %s", w.Code, w.Body.String())
	}
	body := `{"request_id":"` + strings.Repeat("a", 32) + `","project_id":"` + c.identity.ProjectID + `","instance_id":"original-instance","action":"stop_service","target_id":"original-instance","expected_version":0,"payload":{}}`
	if request(s, "POST", route, body, cookie, csrf).Code != 403 || c.calls != 0 {
		t.Fatal("browser stop was guessed")
	}
}

func TestWebLiveAuthenticationUsesOriginalHeaders(t *testing.T) {
	s, _ := fixture(t)
	s.Start()
	r, _ := http.NewRequest("GET", "http://"+s.options.Address+"/auth/status", nil)
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport}
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 401 {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("unauthenticated real listener: %d %s", response.StatusCode, data)
	}
	if _, err = client.Post("http://"+s.options.Address+"/auth/session", "application/json", bytes.NewReader([]byte(`{"ticket":"x"}`))); err != nil {
		t.Fatal(err)
	}
}

func TestWebSwitchSessionAndNativeStop(t *testing.T) {
	s, _ := fixture(t)
	cookie, csrf := login(t, s)
	for _, route := range []string{"/native/shutdown", "/api/v2/application/shutdown", "/api/v2/application/projects/" + strings.Repeat("a", 64) + "/../../shutdown"} {
		if w := request(s, "POST", route, `{}`, cookie, csrf); w.Code < 400 {
			t.Fatalf("browser reached stop %s", route)
		}
	}
	if w := request(s, "GET", "/auth/status", "", cookie, ""); w.Code != 200 || strings.Contains(w.Body.String(), s.secret) {
		t.Fatal(w.Body.String())
	}
}
