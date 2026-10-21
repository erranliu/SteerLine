package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/pathguard"
	privatefs "github.com/erranliu/SteerLine/internal/product/httpfs"
	"github.com/google/uuid"
)

// Web's narrow, read-only native RPC transport. No installed JavaScript, CLI,
// Runner helper, pairing state, or business-session API participates here.
type localRuntime struct {
	RuntimeID  string `json:"runtimeId"`
	Token      string `json:"authToken"`
	Transports []struct {
		Kind     string `json:"kind"`
		Endpoint string `json:"endpoint"`
	} `json:"transports"`
	Endpoint string `json:"-"`
}

func (r localRuntime) identity() string {
	raw, _ := json.Marshal([]string{r.RuntimeID, r.Endpoint, r.Token})
	return cfg.Digest(raw)
}
func (m *Manager) localRuntime() (localRuntime, error) {
	var r localRuntime
	if !filepath.IsAbs(m.dataPath()) {
		return r, errors.New("Orca userData must be absolute")
	}
	path := filepath.Join(m.dataPath(), "orca-runtime.json")
	resolved, err := pathguard.ResolveProspective(path)
	if err != nil || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(path)) {
		return r, errors.New("本机 Orca runtime metadata 不支持链接路径")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return r, errors.New("本机 Orca runtime metadata 不可用，请确认 Orca 已运行")
	}
	f, err := privatefs.OpenRegularSnapshot(path)
	if err != nil {
		return r, errors.New("无法读取本机 Orca runtime metadata")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &r) != nil || r.RuntimeID == "" || r.Token == "" {
		return localRuntime{}, errors.New("本机 Orca runtime metadata 无效")
	}
	for _, t := range r.Transports {
		if t.Kind != "named-pipe" {
			continue
		}
		if r.Endpoint != "" || !strings.HasPrefix(t.Endpoint, `\\.\pipe\`) || len(t.Endpoint) <= len(`\\.\pipe\`) || strings.ContainsAny(strings.TrimPrefix(t.Endpoint, `\\.\pipe\`), "\\/\x00\r\n") {
			return localRuntime{}, errors.New("本机 Orca named pipe 端点无效")
		}
		r.Endpoint = t.Endpoint
	}
	if r.Endpoint == "" {
		return localRuntime{}, errors.New("本机 Orca 未提供 named pipe；不会回退到远端环境")
	}
	return r, nil
}

type localConnection struct {
	conn      net.Conn
	reader    *bufio.Reader
	runtime   localRuntime
	remaining int64
}

func (c *localConnection) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	switch method {
	case "status.get", "agentSession.agents", "agentSession.modelCatalog":
	default:
		return nil, errors.New("unsupported capability method")
	}
	id := uuid.NewString()
	q := map[string]any{"id": id, "authToken": c.runtime.Token, "method": method, "params": params, "clientCapabilities": []string{"agent-session.structured.v1", "agent-session.structured.registered-agents.v1"}}
	if err := json.NewEncoder(c.conn).Encode(q); err != nil {
		return nil, errors.New("本机 Orca 目录请求失败")
	}
	for {
		// Reader is bounded across all methods, including keepalive frames.
		raw, err := c.reader.ReadBytes('\n')
		c.remaining -= int64(len(raw))
		if err != nil || c.remaining < 0 || ctx.Err() != nil {
			return nil, errors.New("本机 Orca 目录连接关闭、超时或响应超过 1 MiB")
		}
		var response struct {
			ID        string          `json:"id"`
			OK        bool            `json:"ok"`
			Keepalive bool            `json:"_keepalive"`
			Result    json.RawMessage `json:"result"`
			Meta      struct {
				RuntimeID string `json:"runtimeId"`
			} `json:"_meta"`
		}
		if json.Unmarshal(raw, &response) != nil {
			return nil, errors.New("本机 Orca 目录响应无效")
		}
		if response.Keepalive {
			continue
		}
		if response.ID != id || !response.OK || len(response.Result) == 0 || response.Meta.RuntimeID != c.runtime.RuntimeID {
			return nil, errors.New("本机 Orca 拒绝目录查询，或响应/runtime 身份不匹配")
		}
		return response.Result, nil
	}
}
func localOrcaCatalog(ctx context.Context, r localRuntime, kind, agent string, out Result) (Result, error) {
	conn, err := dialLocalOrca(ctx, r.Endpoint)
	if err != nil {
		return out, errors.New("无法连接本机 Orca named pipe，请确认 Orca 已运行")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	c := &localConnection{conn: conn, runtime: r, remaining: 1 << 20}
	c.reader = bufio.NewReader(io.LimitReader(conn, (1<<20)+1))
	out.Channel = "named-pipe"
	out.RuntimeDigest = cfg.Digest([]byte(r.RuntimeID))
	return readOrcaCatalog(ctx, c, "", kind, agent, true, out)
}
