package httpapi

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
	"strings"
	"sync"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
)

const maxDiscoveryBytes = 64 << 10

type Options struct {
	RunDir          string
	ProjectIdentity string
	InstanceID      string
	Process         ProcessIdentity
	Address         string
	Capabilities    []string
}

type Server struct {
	product     *productclient.Client
	options     Options
	apiDir      string
	commandsDir string
	credential  string
	token       string
	leaseID     string
	listener    net.Listener
	http        *http.Server
	identity    Identity
	discovery   Discovery
	mu          sync.RWMutex
	commandMu   sync.Mutex
	started     bool
	closed      bool
}

// Prepare binds only the loopback interface and creates the per-instance
// credential/discovery pair before the service begins accepting commands.
func Prepare(ctx context.Context, product *productclient.Client, options Options) (*Server, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if product == nil || options.RunDir == "" {
		return nil, errors.New("local API requires a product client and run directory")
	}
	project := product.Project()
	if project.Identity == "" || options.ProjectIdentity != "" && options.ProjectIdentity != project.Identity {
		return nil, &api.Error{Kind: api.IdentityConflict, Message: "local API project identity does not match the product client"}
	}
	options.ProjectIdentity = project.Identity
	if options.InstanceID == "" {
		return nil, errors.New("local API requires the service instance identity")
	}
	if options.Address == "" {
		options.Address = "127.0.0.1:0"
	}
	addr, err := net.ResolveTCPAddr("tcp4", options.Address)
	if err != nil || addr.IP == nil || addr.IP.To4() == nil || !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		return nil, errors.New("local API address must be 127.0.0.1")
	}
	apiDir := filepath.Join(options.RunDir, "local-api")
	commandsDir := filepath.Join(apiDir, "commands")
	credentialsDir := filepath.Join(apiDir, "credentials")
	if err = ensurePrivateDirectory(apiDir); err == nil {
		err = ensurePrivateDirectory(commandsDir)
	}
	if err == nil {
		err = ensurePrivateDirectory(credentialsDir)
	}
	if err == nil {
		err = removeStaleCredentials(credentialsDir)
	}
	if err != nil {
		return nil, fmt.Errorf("secure local API state directory: %w", err)
	}
	leaseID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		failure := Discovery{Version: 1, Protocol: Protocol, ProjectID: ProjectID(project.Identity), RepositoryIdentity: project.Identity,
			InstanceID: options.InstanceID, Process: options.Process, Ready: false, Status: "failed",
			Diagnostic: fmt.Sprintf("bind local API loopback listener at %s: %v", options.Address, err), LeaseID: leaseID, UpdatedAt: time.Now().UTC()}
		failureBody, marshalErr := json.Marshal(failure)
		if marshalErr == nil {
			marshalErr = writePrivateAtomic(apiDir, filepath.Join(apiDir, "discovery.json"), append(failureBody, '\n'))
		}
		return nil, errors.Join(fmt.Errorf("bind local API loopback listener at %s: %w", options.Address, err), marshalErr)
	}
	token, err := randomHex(32)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	credential := filepath.Join(credentialsDir, leaseID+".token")
	if err = writePrivateExclusive(credentialsDir, credential, []byte(token)); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("write local API credential: %w", err)
	}

	server := &Server{product: product, options: options, apiDir: apiDir, commandsDir: commandsDir,
		credential: credential, token: token, leaseID: leaseID, listener: listener,
		identity: Identity{Protocol: Protocol, ProjectID: ProjectID(project.Identity), RepositoryIdentity: project.Identity,
			InstanceID: options.InstanceID, Process: options.Process, Address: listener.Addr().String(), Status: "starting",
			Capabilities: append([]string(nil), options.Capabilities...), SampledAt: time.Now().UTC()},
	}
	server.discovery = Discovery{Version: 1, Protocol: Protocol, ProjectID: server.identity.ProjectID,
		RepositoryIdentity: project.Identity, InstanceID: options.InstanceID, Process: options.Process,
		Address: listener.Addr().String(), CredentialPath: credential, Ready: false, Status: "starting",
		LeaseID: leaseID, UpdatedAt: time.Now().UTC()}
	server.http = &http.Server{Handler: server, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	if err = server.writeDiscovery(); err != nil {
		_ = listener.Close()
		cleanupErr := os.Remove(credential)
		if os.IsNotExist(cleanupErr) {
			cleanupErr = nil
		}
		return nil, errors.Join(fmt.Errorf("publish local API startup discovery: %w", err), cleanupErr)
	}
	return server, nil
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("local API server is closed")
	}
	if s.started {
		return nil
	}
	s.started = true
	go func() {
		if err := s.http.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.identity.Ready, s.identity.Status, s.identity.Diagnostic = false, "failed", err.Error()
			s.discovery.Ready, s.discovery.Status, s.discovery.Diagnostic = false, "failed", err.Error()
			_ = s.writeDiscoveryLocked()
			s.mu.Unlock()
		}
	}()
	return nil
}

// SetReady publishes the scheduler's real readiness. A false ready state
// keeps mutations diagnostic-only except for stopping this instance.
func (s *Server) SetReady(instanceID string, ready bool, diagnostic string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("local API server is closed")
	}
	if instanceID == "" || instanceID != s.options.InstanceID {
		return &api.Error{Kind: api.IdentityConflict, Message: "local API service instance identity changed"}
	}
	status := "starting"
	if ready {
		status = "ready"
	} else if diagnostic != "" {
		status = "not-ready"
	}
	s.identity.InstanceID, s.identity.Ready, s.identity.Status = instanceID, ready, status
	s.identity.Diagnostic, s.identity.SampledAt = diagnostic, time.Now().UTC()
	s.discovery.InstanceID, s.discovery.Ready, s.discovery.Status = instanceID, ready, status
	s.discovery.Diagnostic, s.discovery.UpdatedAt = diagnostic, time.Now().UTC()
	return s.writeDiscoveryLocked()
}

func (s *Server) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.identity.Ready, s.identity.Status = false, "stopped"
	s.discovery.Ready, s.discovery.Status, s.discovery.UpdatedAt = false, "stopped", time.Now().UTC()
	httpServer, listener := s.http, s.listener
	s.mu.Unlock()

	var retErr error
	if httpServer != nil {
		retErr = httpServer.Shutdown(ctx)
	}
	if listener != nil {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			retErr = errors.Join(retErr, err)
		}
	}
	current, readErr := readDiscoveryFile(filepath.Join(s.apiDir, "discovery.json"))
	if readErr == nil && current.LeaseID == s.leaseID {
		retErr = errors.Join(retErr, os.Remove(filepath.Join(s.apiDir, "discovery.json")))
	} else if readErr != nil && !os.IsNotExist(readErr) {
		retErr = errors.Join(retErr, readErr)
	}
	if err := os.Remove(s.credential); err != nil && !os.IsNotExist(err) {
		retErr = errors.Join(retErr, err)
	}
	return retErr
}

func (s *Server) writeDiscovery() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeDiscoveryLocked()
}

func (s *Server) writeDiscoveryLocked() error {
	body, err := json.Marshal(s.discovery)
	if err != nil {
		return err
	}
	return writePrivateAtomic(s.apiDir, filepath.Join(s.apiDir, "discovery.json"), append(body, '\n'))
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func writePrivateExclusive(dir, path string, body []byte) error {
	return writePrivateContents(dir, path, bytes.NewReader(body), true)
}

func writePrivateAtomic(dir, path string, body []byte) error {
	return writePrivateContents(dir, path, bytes.NewReader(body), false)
}

func writePrivateContents(dir, path string, body io.Reader, exclusive bool) error {
	prefix := ".discovery-"
	if exclusive {
		prefix = ".credential-"
	}
	tmp, err := createPrivateTemp(dir, prefix)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = io.Copy(tmp, body); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = verifyPrivateFile(tmpName); err != nil {
		return err
	}
	if exclusive {
		if err = os.Link(tmpName, path); err != nil {
			return err
		}
		if err = os.Remove(tmpName); err != nil {
			return err
		}
	} else if err = os.Rename(tmpName, path); err != nil {
		return err
	}
	return verifyPrivateFile(path)
}

func removeStaleCredentials(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		staleToken := !entry.IsDir() && requestIDPattern.MatchString(strings.TrimSuffix(name, ".token")) && strings.HasSuffix(name, ".token")
		staleTemporary := !entry.IsDir() && strings.HasPrefix(name, ".credential-") && strings.HasSuffix(name, ".tmp")
		if !staleToken && !staleTemporary {
			continue
		}
		path := filepath.Join(dir, name)
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("stale local API credential entry is not a regular file: %s", name)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove stale local API credential %s: %w", name, err)
		}
	}
	return nil
}

func readDiscoveryFile(path string) (Discovery, error) {
	var discovery Discovery
	if err := verifyPrivateFile(path); err != nil {
		return discovery, err
	}
	file, err := os.Open(path)
	if err != nil {
		return discovery, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDiscoveryBytes+1))
	if err != nil {
		return discovery, err
	}
	if len(data) == 0 || len(data) > maxDiscoveryBytes {
		return discovery, errors.New("local API discovery file is empty or too large")
	}
	if err = decodeStrict(data, &discovery); err != nil {
		return discovery, err
	}
	failedWithoutListener := !discovery.Ready && discovery.Status == "failed" && discovery.Address == "" && discovery.CredentialPath == ""
	if discovery.Version != 1 || discovery.Protocol != Protocol || discovery.ProjectID != ProjectID(discovery.RepositoryIdentity) ||
		(!failedWithoutListener && (discovery.Address == "" || discovery.CredentialPath == "")) || discovery.LeaseID == "" {
		return Discovery{}, errors.New("local API discovery has an unsupported or incomplete identity")
	}
	return discovery, nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("JSON must contain one value")
	}
	return nil
}
