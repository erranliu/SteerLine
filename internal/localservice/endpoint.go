// Package localservice owns private discovery and independent local process
// leases. It has no project, database, or worker lifecycle authority.
package localservice

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
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
	"time"

	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	fs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

type Discovery struct {
	Identity contract.Identity `json:"identity"`
	Token    string            `json:"token"`
}
type Endpoint struct {
	Discovery
	Listener  net.Listener
	Dir       string
	lock      *lease
	closeOnce sync.Once
}

func HomeID(home string) (string, error) {
	p, err := filepath.Abs(home)
	if err != nil {
		return "", err
	}
	if real, e := filepath.EvalSymlinks(p); e == nil {
		p = real
	}
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return httpcontract.ProjectID(p), nil
}
func Token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Address(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return false
	}
	var n int
	_, err = fmt.Sscanf(port, "%d", &n)
	return err == nil && n > 0 && n <= 65535 && fmt.Sprint(n) == port
}
func Prepare(home, dir, protocol, address string) (*Endpoint, error) {
	if err := fs.EnsurePrivateDirectory(dir); err != nil {
		return nil, err
	}
	lock, err := lockLease(dir)
	if err != nil {
		return nil, fmt.Errorf("service already running or lease unavailable: %w", err)
	}
	success := false
	defer func() {
		if !success {
			lock.Close()
		}
	}()
	l, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, err
	}
	id, err := httpclient.CurrentProcessIdentity()
	if err != nil {
		l.Close()
		return nil, err
	}
	homeID, err := HomeID(home)
	if err != nil {
		l.Close()
		return nil, err
	}
	e := &Endpoint{Discovery: Discovery{Identity: contract.Identity{Protocol: protocol, HomeID: homeID, InstanceID: Token(), Process: httpcontract.ProcessIdentity{Kind: id.Kind, PID: id.PID, Created: id.Created, Executable: id.Executable}, Address: l.Addr().String(), Ready: true}, Token: Token()}, Listener: l, Dir: dir, lock: lock}
	if err = Write(dir, "discovery.json", e.Discovery); err != nil {
		l.Close()
		return nil, err
	}
	success = true
	return e, nil
}
func Write(dir, name string, value any) error {
	if err := fs.EnsurePrivateDirectory(dir); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := fs.CreatePrivateTemp(dir, ".publish-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return fs.ReplacePrivateFile(f.Name(), filepath.Join(dir, name))
}
func (e *Endpoint) Close() error {
	var err error
	e.closeOnce.Do(func() {
		err = e.Listener.Close()
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		os.Remove(filepath.Join(e.Dir, "discovery.json"))
		e.lock.Close()
	})
	return err
}
func Equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (e *Endpoint) Authorized(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && host == "127.0.0.1" && r.Host == e.Identity.Address && r.Header.Get("Origin") == "" && Equal(r.Header.Get("Authorization"), "Bearer "+e.Token)
}
func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func Error(w http.ResponseWriter, status int, code, reason string) {
	JSON(w, status, httpcontract.ErrorResponse{Code: code, Reason: reason})
}
func BackendError(w http.ResponseWriter, err error) {
	var ce *httpclient.ClientError
	if errors.As(err, &ce) {
		Error(w, ce.Status, ce.Code, ce.Reason)
	} else {
		Error(w, 503, "backend_unavailable", err.Error())
	}
}
func Read(home, dir, protocol string) (Discovery, error) {
	var d Discovery
	if err := fs.VerifyPrivateDirectory(dir); err != nil {
		return d, err
	}
	p := filepath.Join(dir, "discovery.json")
	if err := fs.VerifyPrivateFile(p); err != nil {
		return d, err
	}
	f, err := os.Open(p)
	if err != nil {
		return d, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return d, err
	}
	if len(data) > 65536 {
		return d, errors.New("discovery exceeds bound")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&d); err != nil {
		return d, err
	}
	if dec.Decode(new(any)) != io.EOF {
		return d, errors.New("one discovery required")
	}
	hid, err := HomeID(home)
	if err != nil {
		return d, err
	}
	if d.Identity.Protocol != protocol || d.Identity.HomeID != hid || len(d.Identity.InstanceID) != 64 || !Address(d.Identity.Address) || len(d.Token) != 64 || !Alive(d.Identity) {
		return d, errors.New("discovery home/protocol/process identity rejected")
	}
	return d, nil
}
func Alive(id contract.Identity) bool {
	exited, err := httpclient.ProcessExited(id.Process)
	return err == nil && !exited
}

type Client struct {
	Discovery
	http      *http.Client
	transport *http.Transport
}

func Open(ctx context.Context, home, dir, protocol, identityPath string) (*Client, error) {
	d, err := Read(home, dir, protocol)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{Proxy: nil, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second}
	c := &Client{Discovery: d, transport: tr, http: &http.Client{Transport: tr, Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("local redirects forbidden") }}}
	var live contract.Identity
	if err = c.Request(ctx, "GET", identityPath, nil, &live); err != nil {
		c.Close()
		return nil, err
	}
	if live != d.Identity {
		c.Close()
		return nil, errors.New("live identity differs from discovery")
	}
	return c, nil
}
func (c *Client) Close() error { c.transport.CloseIdleConnections(); return nil }
func (c *Client) Request(ctx context.Context, method, path string, body, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+c.Identity.Address+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 32<<20 {
		return errors.New("response exceeds bound")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e httpcontract.ErrorResponse
		if json.Unmarshal(data, &e) != nil {
			e.Code = "invalid_error_response"
		}
		return &httpclient.ClientError{Status: resp.StatusCode, Code: e.Code, Reason: e.Reason}
	}
	if target == nil {
		return nil
	}
	return json.Unmarshal(data, target)
}
func Stop(ctx context.Context, home, dir, protocol, identityPath, shutdownPath string) error {
	c, err := Open(ctx, home, dir, protocol, identityPath)
	if err != nil {
		return err
	}
	defer c.Close()
	if err = c.Request(ctx, "POST", shutdownPath, nil, nil); err != nil {
		return err
	}
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		exited, probeErr := httpclient.ProcessExited(c.Identity.Process)
		if probeErr != nil {
			return probeErr
		}
		if exited {
			l, e := lockLease(dir)
			if e == nil {
				l.Close()
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("service exit/lease release unverified: %w", ctx.Err())
		case <-tick.C:
		}
	}
}
