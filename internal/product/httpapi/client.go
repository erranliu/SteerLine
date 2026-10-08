package httpapi

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

var ErrNotReady = errors.New("local SteerLine service is not ready")

type ClientError struct {
	Status int
	Code   string
	Reason string
}

func (e *ClientError) Error() string {
	if e.Reason == "" {
		return e.Code
	}
	return e.Code + ": " + e.Reason
}

type Client struct {
	baseURL   string
	token     string
	project   string
	identity  Identity
	http      *http.Client
	transport *http.Transport
	mu        sync.RWMutex
}

func Open(ctx context.Context, runDir, projectIdentity string) (*Client, error) {
	apiDir := filepath.Join(runDir, "local-api")
	if err := verifyPrivateDirectory(apiDir); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("service discovery is unavailable; start the existing service explicitly: %w", err)
		}
		return nil, fmt.Errorf("verify local API directory: %w", err)
	}
	if err := verifyPrivateDirectory(filepath.Join(apiDir, "credentials")); err != nil {
		return nil, fmt.Errorf("verify local API credential directory: %w", err)
	}
	discovery, err := readDiscoveryFile(filepath.Join(apiDir, "discovery.json"))
	if err != nil {
		return nil, fmt.Errorf("read local API discovery: %w", err)
	}
	if projectIdentity == "" || discovery.RepositoryIdentity != projectIdentity {
		return nil, &api.Error{Kind: api.IdentityConflict, Message: "discovered service belongs to a different project"}
	}
	if !discovery.Ready && (discovery.Address == "" || discovery.CredentialPath == "") {
		return nil, fmt.Errorf("%w: %s", ErrNotReady, diagnosticOrStatus(discovery.Diagnostic, discovery.Status))
	}
	if !privatePath(filepath.Join(apiDir, "credentials"), discovery.CredentialPath) {
		return nil, &api.Error{Kind: api.IdentityConflict, Message: "discovery credential path is outside the local API directory"}
	}
	if err := verifyPrivateFile(discovery.CredentialPath); err != nil {
		return nil, fmt.Errorf("verify local API credential ACL: %w", err)
	}
	tokenBytes, err := os.ReadFile(discovery.CredentialPath)
	if err != nil || len(tokenBytes) != 64 {
		if err == nil {
			err = errors.New("credential has an invalid length")
		}
		return nil, fmt.Errorf("read local API credential: %w", err)
	}
	host, err := localAddress(discovery.Address)
	if err != nil {
		return nil, err
	}
	identity := Identity{Protocol: discovery.Protocol, ProjectID: discovery.ProjectID, RepositoryIdentity: discovery.RepositoryIdentity,
		InstanceID: discovery.InstanceID, Process: discovery.Process, Address: discovery.Address, Ready: discovery.Ready,
		Status: discovery.Status, Diagnostic: discovery.Diagnostic, SampledAt: discovery.UpdatedAt}
	transport := &http.Transport{Proxy: nil, MaxIdleConns: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second}
	client := &Client{baseURL: "http://" + host, token: string(tokenBytes), project: discovery.ProjectID, identity: identity,
		transport: transport, http: &http.Client{Timeout: 30 * time.Second, Transport: transport}}
	var live Identity
	if err = client.get(ctx, "/v1/identity", &live); err != nil {
		return nil, err
	}
	if live.Protocol != Protocol || live.ProjectID != discovery.ProjectID || live.RepositoryIdentity != projectIdentity ||
		live.InstanceID != discovery.InstanceID || live.Process != discovery.Process || live.Address != discovery.Address {
		return nil, &api.Error{Kind: api.IdentityConflict, Message: "live local API identity differs from discovery"}
	}
	client.identity = live
	return client, nil
}

func localAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || portNumber < 1 || portNumber > 65535 || host != "127.0.0.1" || net.ParseIP(host) == nil || strings.Contains(host, ":") {
		return "", errors.New("local API discovery address is not an IPv4 loopback endpoint")
	}
	return net.JoinHostPort(host, port), nil
}

func (c *Client) Identity() Identity {
	c.mu.RLock()
	defer c.mu.RUnlock()
	identity := c.identity
	identity.Capabilities = append([]string(nil), identity.Capabilities...)
	return identity
}

func (c *Client) RefreshIdentity(ctx context.Context) (Identity, error) {
	var live Identity
	if err := c.get(ctx, "/v1/identity", &live); err != nil {
		return Identity{}, err
	}
	current := c.Identity()
	if live.Protocol != current.Protocol || live.ProjectID != current.ProjectID || live.RepositoryIdentity != current.RepositoryIdentity ||
		live.InstanceID != current.InstanceID || live.Process != current.Process || live.Address != current.Address {
		return Identity{}, &api.Error{Kind: api.IdentityConflict, Message: "live local API identity changed; reopen the current discovery file"}
	}
	live.Capabilities = append([]string(nil), live.Capabilities...)
	c.mu.Lock()
	c.identity = live
	c.mu.Unlock()
	return live, nil
}

func (c *Client) Close() error {
	if c != nil && c.transport != nil {
		c.transport.CloseIdleConnections()
	}
	return nil
}

func (c *Client) ProjectPath(path string) string {
	return "/v1/projects/" + url.PathEscape(c.project) + path
}

func (c *Client) Get(ctx context.Context, path string, target any) error {
	return c.get(ctx, c.ProjectPath(path), target)
}

func (c *Client) Post(ctx context.Context, path string, body, target any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return c.request(ctx, http.MethodPost, c.ProjectPath(path), data, target)
}

func (c *Client) ListTasks(ctx context.Context, cursor string, limit int) (api.TaskPage, error) {
	query := url.Values{}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
	}
	var page api.TaskPage
	err := c.Get(ctx, "/tasks?"+query.Encode(), &page)
	return page, err
}

func (c *Client) ShowTask(ctx context.Context, taskID string) (api.TaskPage, error) {
	var page api.TaskPage
	err := c.Get(ctx, "/tasks/"+url.PathEscape(taskID), &page)
	return page, err
}

func (c *Client) Graph(ctx context.Context, cursor string, limit int) (api.GraphPage, error) {
	query := url.Values{}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
	}
	var page api.GraphPage
	err := c.Get(ctx, "/graph?"+query.Encode(), &page)
	return page, err
}

func (c *Client) Snapshot(ctx context.Context, taskID string) (api.ProjectSnapshot, error) {
	query := url.Values{}
	if taskID != "" {
		query.Set("task_id", taskID)
	}
	var snapshot api.ProjectSnapshot
	err := c.Get(ctx, "/snapshot?"+query.Encode(), &snapshot)
	return snapshot, err
}

func (c *Client) Candidates(ctx context.Context) (api.TaskCandidates, error) {
	var candidates api.TaskCandidates
	err := c.Get(ctx, "/candidates", &candidates)
	return candidates, err
}

func (c *Client) ProjectPresentation(ctx context.Context) (ProjectPresentation, error) {
	var presentation ProjectPresentation
	err := c.Get(ctx, "/presentation", &presentation)
	return presentation, err
}

func (c *Client) Run(ctx context.Context, runID string) (api.CandidateSummary, error) {
	var candidate api.CandidateSummary
	err := c.Get(ctx, "/runs/"+url.PathEscape(runID), &candidate)
	return candidate, err
}

func (c *Client) Activity(ctx context.Context, runID, executionID, attemptID string, cursor api.ActivityCursor) (ActivityPage, error) {
	query := url.Values{"execution_id": {executionID}, "attempt_id": {attemptID}}
	if cursor.Initialized {
		data, _ := json.Marshal(cursor)
		query.Set("cursor", string(data))
	}
	var page ActivityPage
	err := c.Get(ctx, "/runs/"+url.PathEscape(runID)+"/activity?"+query.Encode(), &page)
	return page, err
}

func (c *Client) Conversation(ctx context.Context, runID string, filter api.ConversationFilter, cursor string, limit int) (ConversationPage, error) {
	query := url.Values{}
	query.Set("execution_id", filter.ExecutionID)
	query.Set("attempt_id", filter.AttemptID)
	if filter.TurnID != "" {
		query.Set("turn_id", filter.TurnID)
	}
	if filter.FinalsOnly {
		query.Set("finals_only", "true")
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
	}
	var page ConversationPage
	err := c.Get(ctx, "/runs/"+url.PathEscape(runID)+"/conversation?"+query.Encode(), &page)
	return page, err
}

func (c *Client) FinalReply(ctx context.Context, runID string) (FinalReply, error) {
	var reply FinalReply
	err := c.Get(ctx, "/runs/"+url.PathEscape(runID)+"/final", &reply)
	return reply, err
}

func (c *Client) Body(ctx context.Context, runID string, ref api.ConversationBodyRef, offset int64, limit int) (api.ConversationBodyPage, error) {
	refJSON, _ := json.Marshal(ref)
	query := url.Values{"ref": {hex.EncodeToString(refJSON)}, "offset": {fmt.Sprint(offset)}}
	if limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
	}
	var page api.ConversationBodyPage
	err := c.Get(ctx, "/runs/"+url.PathEscape(runID)+"/body?"+query.Encode(), &page)
	return page, err
}

func (c *Client) SubmitCommand(ctx context.Context, envelope CommandEnvelope) (CommandResponse, error) {
	var result CommandResponse
	err := c.Post(ctx, "/commands", envelope, &result)
	return result, err
}

func (c *Client) Command(ctx context.Context, requestID string) (CommandResponse, error) {
	var result CommandResponse
	err := c.Get(ctx, "/commands/"+url.PathEscape(requestID), &result)
	return result, err
}

func (c *Client) get(ctx context.Context, path string, target any) error {
	return c.request(ctx, http.MethodGet, path, nil, target)
}

func (c *Client) request(ctx context.Context, method, path string, body []byte, target any) error {
	if c == nil || c.http == nil {
		return errors.New("local API client is not initialized")
	}
	requestURL, err := url.Parse(c.baseURL + path)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reader)
	if err != nil {
		return err
	}
	req.Host = requestURL.Host
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connect to the discovered local service instance: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 32<<20 {
		return errors.New("local API response exceeds 32 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var value ErrorResponse
		if json.Unmarshal(data, &value) != nil {
			return &ClientError{Status: resp.StatusCode, Code: "invalid_error_response", Reason: string(data)}
		}
		return &ClientError{Status: resp.StatusCode, Code: value.Code, Reason: value.Reason}
	}
	if target == nil {
		return nil
	}
	if err = json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode local API response: %w", err)
	}
	return nil
}

func diagnosticOrStatus(diagnostic, status string) string {
	if diagnostic != "" {
		return diagnostic
	}
	return status
}
