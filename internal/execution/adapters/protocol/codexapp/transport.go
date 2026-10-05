package codexapp

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// codexSocket is a deliberately small RFC 6455 text client. Codex's loopback
// listener rejects Origin headers, which the x/net websocket client always
// sends. Each Client owns its own socket.
type codexSocket struct {
	conn    net.Conn
	read    *bufio.Reader
	writeMu sync.Mutex
}

const maxMessageSize = 16 << 20

func ValidateEndpoint(endpoint string) error {
	_, err := parseCodexEndpoint(endpoint)
	return err
}

func parseCodexEndpoint(endpoint string) (*url.URL, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "ws" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("Codex app-server endpoint must be a loopback WebSocket URL")
	}
	if port, err := strconv.Atoi(u.Port()); err != nil || port < 1 || port > 65535 {
		return nil, errors.New("Codex app-server endpoint has an invalid port")
	}
	return u, nil
}

func dialCodexSocket(ctx context.Context, endpoint string) (*codexSocket, error) {
	u, err := parseCodexEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, err
	}
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()
	good := false
	defer func() {
		if !good {
			_ = conn.Close()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(nonce)
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	request := "GET " + path + " HTTP/1.1\r\nHost: " + u.Host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err = io.WriteString(conn, request); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols || !strings.EqualFold(response.Header.Get("Upgrade"), "websocket") || !strings.EqualFold(response.Header.Get("Connection"), "Upgrade") {
		return nil, fmt.Errorf("Codex app-server rejected WebSocket upgrade: %s", response.Status)
	}
	accept := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(accept[:]) || response.Header.Get("Sec-WebSocket-Extensions") != "" {
		return nil, errors.New("Codex app-server WebSocket handshake is invalid")
	}
	_ = conn.SetDeadline(time.Time{})
	good = true
	return &codexSocket{conn: conn, read: reader}, nil
}

func (s *codexSocket) Close() error { return s.conn.Close() }

func (s *codexSocket) sendFrame(op byte, payload []byte) error {
	if len(payload) > maxMessageSize {
		return errors.New("Codex app-server message exceeds limit")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	frame := make([]byte, 0, len(payload)+14)
	frame = append(frame, 0x80|op)
	switch {
	case len(payload) < 126:
		frame = append(frame, 0x80|byte(len(payload)))
	case len(payload) <= 65535:
		frame = append(frame, 0xfe, byte(len(payload)>>8), byte(len(payload)))
	default:
		frame = append(frame, 0xff, 0, 0, 0, 0, byte(len(payload)>>24), byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload)))
	}
	frame = append(frame, mask...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%4])
	}
	_, err := s.conn.Write(frame)
	return err
}

func (s *codexSocket) Send(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.sendFrame(1, payload)
}

func (s *codexSocket) Receive() (json.RawMessage, error) {
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(s.read, header); err != nil {
			return nil, err
		}
		if header[0]&0x70 != 0 || header[0]&0x80 == 0 || header[1]&0x80 != 0 {
			return nil, errors.New("Codex app-server sent an unsupported WebSocket frame")
		}
		size := uint64(header[1] & 0x7f)
		if header[1]&0x7f == 126 {
			var n uint16
			if err := binary.Read(s.read, binary.BigEndian, &n); err != nil {
				return nil, err
			}
			size = uint64(n)
		} else if header[1]&0x7f == 127 {
			if err := binary.Read(s.read, binary.BigEndian, &size); err != nil {
				return nil, err
			}
		}
		if size > maxMessageSize {
			return nil, errors.New("Codex app-server frame exceeds limit")
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(s.read, payload); err != nil {
			return nil, err
		}
		switch header[0] & 0xf {
		case 1:
			if !json.Valid(payload) {
				return nil, errors.New("Codex app-server sent invalid JSON")
			}
			return payload, nil
		case 8:
			return nil, io.EOF
		case 9:
			if err := s.sendFrame(10, payload); err != nil {
				return nil, err
			}
		case 10:
		default:
			return nil, errors.New("Codex app-server sent an unsupported WebSocket opcode")
		}
	}
}
