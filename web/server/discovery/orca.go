package discovery

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/coder/websocket"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/google/uuid"
	"golang.org/x/crypto/nacl/box"
)

// This narrow codec implements Orca's paired RPC wire contract. It does not
// import the Orca installation, launch terminal agents, or write pairing state.
type accessEndpoint struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
	Token    string `json:"deviceToken"`
	Key      string `json:"publicKeyB64"`
}
type nativeEnvironment struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	PairingRevision int64            `json:"pairingRevision"`
	PairedDevice    string           `json:"pairedDeviceId"`
	Preferred       string           `json:"preferredEndpointId"`
	Endpoints       []accessEndpoint `json:"endpoints"`
	SSH             json.RawMessage  `json:"sshAccess"`
	PendingSSH      json.RawMessage  `json:"pendingSshAccessOperation"`
}

func (e nativeEnvironment) preferred() (accessEndpoint, error) {
	for _, ep := range e.Endpoints {
		if ep.ID == e.Preferred {
			return ep, nil
		}
	}
	if len(e.Endpoints) == 1 {
		return e.Endpoints[0], nil
	}
	return accessEndpoint{}, errors.New("Orca environment has no unambiguous preferred endpoint")
}
func (e nativeEnvironment) pairingKey() string { b, _ := json.Marshal(e); return cfg.Digest(b) }
func (m *Manager) readEnvironments() ([]nativeEnvironment, error) {
	data := m.dataPath()
	if !filepath.IsAbs(data) {
		return nil, errors.New("Orca environment directory must be absolute")
	}
	path := filepath.Join(data, "orca-environments.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return []nativeEnvironment{}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("Orca environment store is unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot read Orca registered environments")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("Orca environment store exceeds its limit")
	}
	var store struct {
		Version      int                 `json:"version"`
		Environments []nativeEnvironment `json:"environments"`
	}
	if json.Unmarshal(raw, &store) != nil || store.Version != 1 {
		return nil, errors.New("Orca environment store format is unsupported")
	}
	seen := map[string]bool{}
	for _, e := range store.Environments {
		if e.ID == "" || seen[e.ID] {
			return nil, errors.New("invalid Orca environment identities")
		}
		seen[e.ID] = true
	}
	return store.Environments, nil
}
func (m *Manager) environments(ctx context.Context, executable string) ([]Environment, error) {
	// Read the registered pairing directory without requiring Orca's local UI
	// runtime to be online. Only display identities cross the browser boundary.
	entries, err := m.readEnvironments()
	if err != nil {
		return nil, err
	}
	out := []Environment{}
	for _, e := range entries {
		out = append(out, Environment{e.ID, e.Name})
	}
	return out, ctx.Err()
}
func (m *Manager) environment(selector string) (nativeEnvironment, error) {
	entries, err := m.readEnvironments()
	if err != nil {
		return nativeEnvironment{}, err
	}
	var matches []nativeEnvironment
	for _, e := range entries {
		if e.ID == selector || e.Name == selector {
			matches = append(matches, e)
		}
	}
	if len(matches) != 1 {
		return nativeEnvironment{}, errors.New("Orca environment is absent or ambiguous; refresh the machine list")
	}
	e := matches[0]
	if len(e.SSH) > 0 && string(e.SSH) != "null" || len(e.PendingSSH) > 0 && string(e.PendingSSH) != "null" {
		return nativeEnvironment{}, errors.New("Orca SSH tunnel environment discovery is unsupported")
	}
	return e, nil
}

type orcaConnection struct {
	socket    *websocket.Conn
	shared    [32]byte
	token     string
	bytes     int
	runtimeID string
}

func connectOrca(ctx context.Context, e nativeEnvironment) (*orcaConnection, error) {
	ep, err := e.preferred()
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(ep.Endpoint)
	if err != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.User != nil || u.Host == "" || ep.Token == "" {
		return nil, errors.New("Orca paired endpoint is unsupported")
	}
	peer, err := base64.StdEncoding.DecodeString(ep.Key)
	if err != nil || len(peer) != 32 {
		return nil, errors.New("Orca paired host key is invalid")
	}
	pub, secret, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	var peerKey [32]byte
	copy(peerKey[:], peer)
	// Endpoint/token/key are read only from the already registered environment,
	// never accepted from a browser request and never included in public errors.
	socket, _, err := websocket.Dial(ctx, ep.Endpoint, nil)
	if err != nil {
		return nil, errors.New("无法连接已配对的 Orca 运行机")
	}
	socket.SetReadLimit(2 << 20)
	c := &orcaConnection{socket: socket, token: ep.Token}
	box.Precompute(&c.shared, &peerKey, secret)
	fail := func() (*orcaConnection, error) {
		socket.CloseNow()
		return nil, errors.New("Orca capability handshake failed")
	}
	hello, _ := json.Marshal(map[string]any{"type": "e2ee_hello", "publicKeyB64": base64.StdEncoding.EncodeToString(pub[:])})
	if socket.Write(ctx, websocket.MessageText, hello) != nil {
		return fail()
	}
	kind, ready, err := socket.Read(ctx)
	var frame struct {
		Type string `json:"type"`
	}
	if err != nil || kind != websocket.MessageText || json.Unmarshal(ready, &frame) != nil || frame.Type != "e2ee_ready" {
		return fail()
	}
	auth := map[string]any{"type": "e2ee_auth", "deviceToken": ep.Token, "clientCapabilities": []string{"agent-session.structured.v1", "agent-session.structured.registered-agents.v1"}}
	if c.write(ctx, auth) != nil {
		return fail()
	}
	raw, err := c.read(ctx)
	if err != nil || json.Unmarshal(raw, &frame) != nil || frame.Type != "e2ee_authenticated" {
		return fail()
	}
	return c, nil
}
func (c *orcaConnection) write(ctx context.Context, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var nonce [24]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	encrypted := box.SealAfterPrecomputation(nonce[:], raw, &nonce, &c.shared)
	return c.socket.Write(ctx, websocket.MessageText, []byte(base64.StdEncoding.EncodeToString(encrypted)))
}
func (c *orcaConnection) read(ctx context.Context) ([]byte, error) {
	kind, frame, err := c.socket.Read(ctx)
	if err != nil || kind != websocket.MessageText {
		return nil, errors.New("Orca capability connection closed or timed out")
	}
	b, err := base64.StdEncoding.DecodeString(string(frame))
	if err != nil || len(b) < 40 {
		return nil, errors.New("Orca capability response is not encrypted correctly")
	}
	var nonce [24]byte
	copy(nonce[:], b[:24])
	plain, ok := box.OpenAfterPrecomputation(nil, b[24:], &nonce, &c.shared)
	if !ok || len(plain) > 1<<20 {
		return nil, errors.New("Orca capability response failed authentication")
	}
	c.bytes += len(plain)
	if c.bytes > 1<<20 {
		return nil, errors.New("Orca capability response exceeds 1 MiB")
	}
	return plain, nil
}
func (c *orcaConnection) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	switch method {
	case "status.get", "agentSession.agents", "agentSession.modelCatalog":
	default:
		return nil, errors.New("unsupported capability method")
	}
	id := uuid.NewString()
	if err := c.write(ctx, map[string]any{"id": id, "deviceToken": c.token, "method": method, "params": params}); err != nil {
		return nil, errors.New("Orca capability request failed")
	}
	for {
		raw, err := c.read(ctx)
		if err != nil {
			return nil, err
		}
		var response struct {
			ID        string          `json:"id"`
			OK        bool            `json:"ok"`
			Result    json.RawMessage `json:"result"`
			Keepalive bool            `json:"_keepalive"`
			Meta      struct {
				RuntimeID string `json:"runtimeId"`
			} `json:"_meta"`
		}
		if json.Unmarshal(raw, &response) != nil {
			return nil, errors.New("invalid Orca capability response")
		}
		if response.Keepalive {
			continue
		}
		if response.ID != id || !response.OK || len(response.Result) == 0 {
			return nil, errors.New("Orca runtime rejected the capability query or returned a mismatched response")
		}
		if response.Meta.RuntimeID == "" || c.runtimeID != "" && response.Meta.RuntimeID != c.runtimeID {
			return nil, errors.New("Orca runtime identity changed during capability discovery")
		}
		c.runtimeID = response.Meta.RuntimeID
		return response.Result, nil
	}
}
func orcaCatalog(ctx context.Context, e nativeEnvironment, kind string, out Result) (Result, error) {
	c, err := connectOrca(ctx, e)
	if err != nil {
		return out, err
	}
	defer c.socket.CloseNow()
	return readOrcaCatalog(ctx, c, e.PairedDevice, kind, "codex", false, out)
}

type capabilityCaller interface {
	call(context.Context, string, any) (json.RawMessage, error)
}

func readOrcaCatalog(ctx context.Context, c capabilityCaller, pairedDevice, kind, agent string, local bool, out Result) (Result, error) {
	raw, err := c.call(ctx, "status.get", map[string]any{})
	if err != nil {
		return out, err
	}
	var status struct {
		Capabilities []string `json:"capabilities"`
		Protocol     int      `json:"runtimeProtocolVersion"`
		Minimum      int      `json:"minCompatibleRuntimeClientVersion"`
		Version      string   `json:"appVersion"`
		PairedDevice string   `json:"pairedDeviceId"`
	}
	if json.Unmarshal(raw, &status) != nil || status.Protocol != 3 || status.Minimum > 3 {
		return out, errors.New("Orca capability protocol is unsupported")
	}
	if pairedDevice != "" && status.PairedDevice != pairedDevice {
		return out, errors.New("Orca paired device identity differs from its registration")
	}
	out.Version = status.Version
	out.Protocol = status.Protocol
	has := func(value string) bool {
		for _, s := range status.Capabilities {
			if s == value {
				return true
			}
		}
		return false
	}
	if !has("agent-session.structured.v1") || !has("agent-session.structured.registered-agents.v1") {
		out.State = "unsupported"
		out.Diagnostic = "Orca 版本不支持 Agent 能力目录"
		return out, nil
	}
	raw, err = c.call(ctx, "agentSession.agents", map[string]any{})
	if err != nil {
		return out, err
	}
	var agents struct {
		Agents []struct {
			Agent string `json:"agent"`
		} `json:"agents"`
	}
	if json.Unmarshal(raw, &agents) != nil || agents.Agents == nil {
		return out, errors.New("Orca Agent directory is incomplete")
	}
	found := false
	for _, a := range agents.Agents {
		if a.Agent == "" {
			return out, errors.New("invalid Orca Agent identity")
		}
		supported := a.Agent == "codex" || local && (a.Agent == "pi" || a.Agent == "dsh")
		reason := ""
		if !supported {
			reason = "当前执行链路未支持"
		}
		out.Agents = append(out.Agents, Agent{ID: a.Agent, Supported: supported, Reason: reason, Directory: "registered"})
		found = found || a.Agent == agent
	}
	if local {
		for _, known := range []string{"codex", "pi", "dsh"} {
			registered := false
			for _, a := range out.Agents {
				registered = registered || a.ID == known
			}
			if !registered {
				out.Agents = append(out.Agents, Agent{ID: known, Supported: true, Directory: "unverified", Reason: "已映射；Orca 未注册目录，可手动填写未验证值"})
			}
		}
	}
	if kind == "agents" {
		out.State = "ready"
		out.Origin = "registered"
		return out, nil
	}
	if !found {
		out.State = "unsupported"
		out.Diagnostic = "目标运行机未注册所选 Agent 能力目录；可手动填写未验证值"
		return out, nil
	}
	query := map[string]any{"agent": agent}
	raw, err = c.call(ctx, "agentSession.modelCatalog", query)
	if err != nil {
		return out, err
	}
	type response struct {
		Origin    string `json:"origin"`
		FetchedAt int64  `json:"fetchedAt"`
		Listing   bool   `json:"listingInProgress"`
		Models    []struct {
			ID            string   `json:"id"`
			Label         string   `json:"label"`
			Default       bool     `json:"isDefault"`
			DefaultEffort string   `json:"defaultEffort"`
			Efforts       []Effort `json:"efforts"`
		} `json:"models"`
	}
	var catalog response
	if json.Unmarshal(raw, &catalog) != nil {
		return out, errors.New("invalid Orca model catalog")
	}
	if catalog.Listing {
		query["waitForListing"] = true
		raw, err = c.call(ctx, "agentSession.modelCatalog", query)
		if err != nil {
			return out, err
		}
		catalog = response{}
		if json.Unmarshal(raw, &catalog) != nil {
			return out, errors.New("invalid Orca model listing")
		}
	}
	out.Origin = catalog.Origin
	if out.Origin == "" {
		out.Origin = "unknown"
	}
	if catalog.FetchedAt > 0 {
		out.FetchedAt = time.UnixMilli(catalog.FetchedAt).UTC()
	}
	if catalog.Listing {
		out.State = "in_progress"
		return out, nil
	}
	if catalog.Models == nil {
		out.State = "unavailable"
		out.Diagnostic = "Orca 未提供完整模型目录"
		return out, nil
	}
	seen := map[string]bool{}
	out.Models = []Model{}
	for _, m := range catalog.Models {
		if m.ID == "" || seen[m.ID] {
			return out, errors.New("Orca returned duplicate or empty model identities")
		}
		seen[m.ID] = true
		efforts := m.Efforts
		if efforts == nil {
			efforts = []Effort{}
		}
		seenEffort := map[string]bool{}
		for _, effort := range efforts {
			if effort.Value == "" || seenEffort[effort.Value] {
				return out, errors.New("Orca returned invalid effort identities")
			}
			seenEffort[effort.Value] = true
		}
		out.Models = append(out.Models, Model{m.ID, m.Label, m.Default, m.DefaultEffort, efforts})
	}
	out.State = "ready"
	return out, nil
}
