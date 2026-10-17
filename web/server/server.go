// Package server hosts the browser client. It owns browser authentication and
// delegates product requests to the existing native HTTP client.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
)

type Client interface {
	Identity() contract.Identity
	RefreshIdentity(context.Context) (contract.Identity, error)
	Get(context.Context, string, any) error
	Post(context.Context, string, any, any) error
	Close() error
}
type Options struct {
	Address, Home, DevOrigin, LauncherDir string
	Client                                Client
	Assets                                fs.FS
}
type session struct {
	CSRF    string
	Expires time.Time
}
type Server struct {
	options        Options
	listener       net.Listener
	http           *http.Server
	identity       contract.Identity
	endpoint       *localservice.Endpoint
	stopped        chan struct{}
	stopOnce       sync.Once
	secret, cookie string
	assets         fs.FS
	dev            http.Handler
	mu             sync.Mutex
	tickets        map[string]time.Time
	sessions       map[string]session
}

func ValidateAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return errors.New("Web address must be a fixed 127.0.0.1:<port>")
	}
	var number int
	if _, err = fmt.Sscanf(port, "%d", &number); err != nil || number < 1 || number > 65535 || fmt.Sprint(number) != port {
		return errors.New("Web address requires a stable port from 1 to 65535")
	}
	return nil
}

func Prepare(options Options) (*Server, error) {
	if err := ValidateAddress(options.Address); err != nil {
		return nil, err
	}
	if options.Client == nil || options.Home == "" || options.LauncherDir == "" {
		return nil, errors.New("Web requires the application client and home")
	}
	assets := options.Assets
	var dev http.Handler
	if options.DevOrigin != "" {
		target, err := url.Parse(options.DevOrigin)
		if err != nil || target.Scheme != "http" || target.User != nil || target.Path != "" || target.RawQuery != "" || target.Fragment != "" || ValidateAddress(target.Host) != nil {
			return nil, errors.New("Web development origin must be an explicit http://127.0.0.1:<port>")
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			fail(w, 502, "dev_assets_unavailable", "Vite development server is unavailable")
		}
		dev = proxy
	} else if assets == nil {
		var err error
		assets, err = EmbeddedAssets()
		if err != nil {
			return nil, err
		}
	}
	endpoint, err := localservice.Prepare(options.Home, options.LauncherDir, contract.WebProtocol, options.Address)
	if err != nil {
		return nil, err
	}
	listener := endpoint.Listener
	secret := endpoint.Token
	identity := endpoint.Identity
	appIdentity := options.Client.Identity()
	if appIdentity.HomeID != identity.HomeID || appIdentity.Protocol != contract.Protocol {
		endpoint.Close()
		return nil, errors.New("Web requires matching application home identity")
	}
	s := &Server{options: options, listener: listener, endpoint: endpoint, stopped: make(chan struct{}), identity: identity, secret: secret, assets: assets, dev: dev,
		cookie: "sl_web_" + identity.HomeID[:12] + "_" + strings.Split(options.Address, ":")[1], tickets: map[string]time.Time{}, sessions: map[string]session{}}
	s.http = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 125 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}

	return s, nil
}
func (s *Server) Start() {
	go func() {
		if err := s.http.Serve(s.listener); err != nil {
			s.stopOnce.Do(func() { close(s.stopped) })
		}
	}()
}
func (s *Server) Close(ctx context.Context) error {
	shutdown := s.http.Shutdown(ctx)
	if errors.Is(shutdown, context.DeadlineExceeded) {
		shutdown = nil
	}
	closed := s.http.Close()
	listener := s.listener.Close()
	if errors.Is(listener, net.ErrClosed) {
		listener = nil
	}
	return errors.Join(shutdown, closed, listener, s.options.Client.Close(), s.endpoint.Close())
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	policy := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"
	if s.dev != nil {
		policy = strings.Replace(policy, "script-src 'self'", "script-src 'self' 'unsafe-inline'", 1)
	}
	w.Header().Set("Content-Security-Policy", policy)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host != "127.0.0.1" || r.Host != s.options.Address {
		fail(w, 403, "source_rejected", "Web requires its exact loopback Host")
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		fail(w, 403, "origin_rejected", "cross-site Web requests are forbidden")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+s.options.Address {
		fail(w, 403, "origin_rejected", "Web Origin does not match")
		return
	}
	if r.URL.Path == "/native/identity" || r.URL.Path == "/native/shutdown" || r.URL.Path == "/native/backend" {
		if r.URL.RawPath != "" || !s.endpoint.Authorized(r) {
			fail(w, 403, "launcher_rejected", "native launcher credential required")
			return
		}
		if r.Method == "GET" && r.URL.Path == "/native/backend" {
			identity, err := s.options.Client.RefreshIdentity(r.Context())
			if err != nil {
				localservice.BackendError(w, err)
			} else {
				localservice.JSON(w, 200, identity)
			}
			return
		}
		if r.Method == "GET" && r.URL.Path == "/native/identity" {
			writeJSON(w, 200, s.identity)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/native/shutdown" {
			writeJSON(w, 200, map[string]string{"status": "stopping"})
			s.stopOnce.Do(func() { close(s.stopped) })
			return
		}
		fail(w, 405, "method_not_allowed", "native method rejected")
		return
	}
	if r.URL.Path == "/auth/issue" {
		s.issue(w, r)
		return
	}
	if r.URL.Path == "/auth/session" {
		s.exchange(w, r)
		return
	}
	if r.URL.Path == "/auth/status" {
		s.status(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.api(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/auth/") {
		fail(w, 404, "route_not_found", "unknown authentication route")
		return
	}
	s.static(w, r)
}
func isJSON(r *http.Request) bool {
	value, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && value == "application/json"
}
func (s *Server) browserWrite(r *http.Request) bool {
	return r.Method == "POST" && r.Header.Get("Origin") == "http://"+s.options.Address && isJSON(r)
}
func (s *Server) issue(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.Header.Get("Origin") != "" || !equal(r.Header.Get("Authorization"), "Bearer "+s.secret) {
		fail(w, 403, "launcher_rejected", "use the original local CLI to open Web")
		return
	}
	ticket, err := randomToken()
	if err != nil {
		fail(w, 500, "random_failed", err.Error())
		return
	}
	s.mu.Lock()
	s.prune()
	s.tickets[ticket] = time.Now().Add(time.Minute)
	s.mu.Unlock()
	writeJSON(w, 200, map[string]string{"ticket": ticket, "home_id": s.identity.HomeID, "instance_id": s.identity.InstanceID})
}
func (s *Server) exchange(w http.ResponseWriter, r *http.Request) {
	if !s.browserWrite(r) {
		fail(w, 403, "csrf_rejected", "session exchange requires same-origin JSON POST")
		return
	}
	var input struct {
		Ticket string `json:"ticket"`
	}
	if err := decode(w, r, &input, 4096); err != nil {
		fail(w, 400, "invalid_ticket", err.Error())
		return
	}
	token, err := randomToken()
	if err != nil {
		fail(w, 500, "random_failed", err.Error())
		return
	}
	csrf, err := randomToken()
	if err != nil {
		fail(w, 500, "random_failed", err.Error())
		return
	}
	s.mu.Lock()
	s.prune()
	expiry, ok := s.tickets[input.Ticket]
	delete(s.tickets, input.Ticket)
	if !ok || !expiry.After(time.Now()) {
		s.mu.Unlock()
		fail(w, 403, "ticket_expired", "opening ticket expired or was already used; open Web from the CLI again")
		return
	}
	s.sessions[token] = session{CSRF: csrf, Expires: time.Now().Add(12 * time.Hour)}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: s.cookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, map[string]string{"csrf_token": csrf})
}
func (s *Server) authenticated(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(s.cookie)
	if err != nil {
		return session{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	value, ok := s.sessions[cookie.Value]
	return value, ok
}
func (s *Server) prune() {
	now := time.Now()
	for k, v := range s.tickets {
		if !v.After(now) {
			delete(s.tickets, k)
		}
	}
	for k, v := range s.sessions {
		if !v.Expires.After(now) {
			delete(s.sessions, k)
		}
	}
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed", "GET required")
		return
	}
	session, ok := s.authenticated(r)
	if !ok {
		fail(w, 401, "session_required", "open Web with steerline-web open")
		return
	}
	_, err := s.options.Client.RefreshIdentity(r.Context())
	diagnostic := ""
	if err != nil {
		diagnostic = err.Error()
	}
	writeJSON(w, 200, struct {
		CSRF       string            `json:"csrf_token"`
		Identity   contract.Identity `json:"identity"`
		Diagnostic string            `json:"diagnostic,omitempty"`
	}{session.CSRF, s.identity, diagnostic})
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	session, ok := s.authenticated(r)
	if !ok {
		fail(w, 401, "session_required", "browser session is missing or expired")
		return
	}
	if r.URL.RawPath != "" || strings.Contains(r.URL.Path, "..") || strings.Contains(r.URL.Path, "\\") {
		fail(w, 400, "invalid_path", "canonical API path required")
		return
	}
	prefix := "/api/v2/application"
	route := strings.TrimPrefix(r.URL.Path, prefix)
	if route == r.URL.Path || !(route == "/projects" || strings.HasPrefix(route, "/projects/")) {
		fail(w, 404, "route_not_found", "application route required")
		return
	}
	parts := strings.Split(strings.TrimPrefix(route, "/projects/"), "/")
	if route != "/projects" && (len(parts) < 2 || len(parts[0]) != 64 || !(r.Method == "GET" && httpcontract.AllowedProjectRead("/"+strings.Join(parts[1:], "/")) || r.Method == "POST" && len(parts) == 2 && parts[1] == "commands")) {
		fail(w, 404, "route_not_found", "unsupported project route")
		return
	}
	if r.URL.RawQuery != "" {
		route += "?" + r.URL.RawQuery
	}
	var result json.RawMessage
	var err error
	switch r.Method {
	case "GET":
		err = s.options.Client.Get(r.Context(), route, &result)
	case "POST":
		if !s.browserWrite(r) || !equal(r.Header.Get("X-CSRF-Token"), session.CSRF) {
			fail(w, 403, "csrf_rejected", "same-origin JSON and session CSRF token required")
			return
		}
		if (len(parts) != 2 || parts[1] != "commands") || r.URL.RawQuery != "" {
			fail(w, 404, "route_not_found", "only formal command submission is writable")
			return
		}
		var envelope httpcontract.CommandEnvelope
		if err = decode(w, r, &envelope, 1<<20); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				fail(w, 413, "request_body_too_large", err.Error())
			} else {
				fail(w, 400, "invalid_command", err.Error())
			}
			return
		}
		if envelope.ProjectID != parts[0] {
			fail(w, 409, "project_mismatch", "command belongs to another route")
			return
		}
		if envelope.Action == "stop_service" {
			fail(w, 403, "unsupported", "browser stop verification is not available; use the native client")
			return
		}
		err = s.options.Client.Post(r.Context(), route, envelope, &result)
	default:
		fail(w, 405, "method_not_allowed", "GET or formal POST required")
		return
	}
	if err != nil {
		backendError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		fail(w, 405, "method_not_allowed", "static resources require GET/HEAD")
		return
	}
	if r.URL.RawPath != "" || strings.Contains(r.URL.Path, "\\") || path.Clean(r.URL.Path) != r.URL.Path && r.URL.Path != "/" {
		fail(w, 400, "invalid_path", "canonical static path required")
		return
	}
	if s.dev != nil {
		s.dev.ServeHTTP(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	body, err := fs.ReadFile(s.assets, name)
	if errors.Is(err, fs.ErrNotExist) && spaPath(r.URL.Path) {
		name = "index.html"
		body, err = fs.ReadFile(s.assets, name)
	}
	if err != nil {
		fail(w, 404, "asset_not_found", "Web resource not found")
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
	if r.Method == "GET" {
		w.Write(body)
	}
}
func randomToken() (string, error) {
	value := make([]byte, 32)
	_, err := rand.Read(value)
	return hex.EncodeToString(value), err
}
func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func decode(w http.ResponseWriter, r *http.Request, target any, max int64) error {
	reader := http.MaxBytesReader(w, r.Body, max)
	defer reader.Close()
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("one JSON value required")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, code, reason string) {
	writeJSON(w, status, httpcontract.ErrorResponse{Code: code, Reason: reason})
}
func backendError(w http.ResponseWriter, err error) {
	var clientErr *httpclient.ClientError
	if errors.As(err, &clientErr) {
		fail(w, clientErr.Status, clientErr.Code, clientErr.Reason)
	} else {
		fail(w, 503, "backend_unavailable", err.Error())
	}
}

func (s *Server) Done() <-chan struct{} { return s.stopped }
func spaPath(route string) bool {
	parts := strings.Split(strings.Trim(route, "/"), "/")
	return len(parts) == 3 && parts[0] == "projects" && len(parts[1]) == 64 && (parts[2] == "dag" || parts[2] == "runs" || parts[2] == "settings" || parts[2] == "environment-checks") || route == "/projects"
}
