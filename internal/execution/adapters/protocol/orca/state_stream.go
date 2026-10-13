package orca

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/coder/websocket"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

type statePairing struct {
	Version   int    `json:"v"`
	Endpoint  string `json:"endpoint"`
	Token     string `json:"deviceToken"`
	PublicKey string `json:"publicKeyB64"`
	Scope     string `json:"scope"`
}

func parseStatePairing(code string) (statePairing, error) {
	var pairing statePairing
	u, err := url.Parse(code)
	if err != nil || u.Scheme != "orca" || u.Host != "pair" {
		return pairing, errors.New("invalid Orca state pairing link")
	}
	encoded := u.Query().Get("code")
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(encoded)
	}
	if err != nil || json.Unmarshal(data, &pairing) != nil || pairing.Version != 2 || pairing.Scope != "runtime" {
		return pairing, errors.New("Orca state pairing requires a runtime-scope v2 link")
	}
	return validateStatePairing(pairing)
}

func validateStatePairing(pairing statePairing) (statePairing, error) {
	u, err := url.Parse(pairing.Endpoint)
	key, keyErr := base64.StdEncoding.DecodeString(pairing.PublicKey)
	if err != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" || u.User != nil || pairing.Token == "" || keyErr != nil || len(key) != 32 {
		return statePairing{}, errors.New("Orca state pairing connection is incomplete")
	}
	return pairing, nil
}

// Credentials stay in the native environment store or process environment,
// never in execution/profile envelopes. An explicit environment never falls
// back to a local pairing.
func resolveStatePairing(environment string) (statePairing, error) {
	if environment == "" {
		code := os.Getenv("ORCA_PAIRING_CODE")
		if code == "" {
			code = os.Getenv("ORCA_REMOTE_PAIRING")
		}
		if code == "" {
			return statePairing{}, errors.New("Orca native state subscription requires native pairing authentication")
		}
		return parseStatePairing(code)
	}
	dir, err := stateUserDataDir()
	if err != nil {
		return statePairing{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "orca-environments.json"))
	if err != nil {
		return statePairing{}, errors.New("fixed Orca environment authentication is unavailable")
	}
	var store struct {
		Environments []struct {
			ID, Name, PreferredEndpointID string
			Endpoints                     []struct {
				ID, Endpoint, DeviceToken, PublicKeyB64 string
			}
		}
	}
	if json.Unmarshal(data, &store) != nil {
		return statePairing{}, errors.New("fixed Orca environment authentication is invalid")
	}
	for _, entry := range store.Environments {
		if entry.ID != environment && entry.Name != environment && "id:"+entry.ID != environment && "name:"+entry.Name != environment {
			continue
		}
		for _, endpoint := range entry.Endpoints {
			if endpoint.ID == entry.PreferredEndpointID {
				return validateStatePairing(statePairing{Endpoint: endpoint.Endpoint, Token: endpoint.DeviceToken, PublicKey: endpoint.PublicKeyB64})
			}
		}
	}
	return statePairing{}, errors.New("fixed Orca environment endpoint is unavailable")
}

type stateSocket struct {
	conn    *websocket.Conn
	key     [32]byte
	token   string
	runtime string
	mu      sync.Mutex
}

func openStateSocket(ctx context.Context, pairing statePairing, expectedRuntime string) (*stateSocket, error) {
	conn, _, err := websocket.Dial(ctx, pairing.Endpoint, nil)
	if err != nil {
		return nil, errors.New("Orca state streaming connection failed")
	}
	conn.SetReadLimit(4 << 20)
	socket := &stateSocket{conn: conn, token: pairing.Token, runtime: expectedRuntime}
	fail := func(err error) (*stateSocket, error) { _ = conn.CloseNow(); return nil, err }
	pub, private, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return fail(err)
	}
	key, err := base64.StdEncoding.DecodeString(pairing.PublicKey)
	if err != nil || len(key) != 32 {
		return fail(errors.New("invalid Orca state host key"))
	}
	var host [32]byte
	copy(host[:], key)
	box.Precompute(&socket.key, &host, private)
	hello, _ := json.Marshal(map[string]string{"type": "e2ee_hello", "publicKeyB64": base64.StdEncoding.EncodeToString(pub[:])})
	if err := conn.Write(ctx, websocket.MessageText, hello); err != nil {
		return fail(err)
	}
	_, data, err := conn.Read(ctx)
	var ready struct{ Type string }
	if err != nil || json.Unmarshal(data, &ready) != nil || ready.Type != "e2ee_ready" {
		return fail(errors.New("Orca state host handshake is unverified"))
	}
	if err := socket.write(ctx, map[string]any{"type": "e2ee_auth", "deviceToken": pairing.Token,
		"clientCapabilities": []string{"agent-session.session-boundary.v1"}}); err != nil {
		return fail(err)
	}
	data, err = socket.read(ctx)
	var auth struct{ Type string }
	if err != nil || json.Unmarshal(data, &auth) != nil || auth.Type != "e2ee_authenticated" {
		return fail(errors.New("Orca state pairing authentication was rejected"))
	}
	if _, err := socket.call(ctx, "status.get", nil); err != nil {
		return fail(err)
	}
	return socket, nil
}

func (s *stateSocket) write(ctx context.Context, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	encrypted := secretbox.Seal(nonce[:], data, &nonce, &s.key)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.Write(ctx, websocket.MessageText, []byte(base64.StdEncoding.EncodeToString(encrypted)))
}

func (s *stateSocket) read(ctx context.Context) ([]byte, error) {
	kind, data, err := s.conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	if kind != websocket.MessageText {
		return nil, errors.New("unexpected binary Orca state frame")
	}
	encrypted, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil || len(encrypted) < 40 {
		return nil, errors.New("invalid encrypted Orca state frame")
	}
	var nonce [24]byte
	copy(nonce[:], encrypted[:24])
	plain, ok := secretbox.Open(nil, encrypted[24:], &nonce, &s.key)
	if !ok {
		return nil, errors.New("Orca state host identity could not be verified")
	}
	return plain, nil
}

func (s *stateSocket) request(ctx context.Context, method string, params any) (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	requestID := hex.EncodeToString(id[:])
	return requestID, s.write(ctx, map[string]any{"id": requestID, "deviceToken": s.token, "method": method, "params": params})
}

func (s *stateSocket) response(data []byte, requestID string) (json.RawMessage, error) {
	var response struct {
		ID        string                     `json:"id"`
		OK        bool                       `json:"ok"`
		Result    json.RawMessage            `json:"result"`
		Error     *StateRPCError             `json:"error"`
		Keepalive bool                       `json:"_keepalive"`
		Meta      struct{ RuntimeID string } `json:"_meta"`
	}
	if json.Unmarshal(data, &response) != nil {
		return nil, errors.New("invalid Orca state envelope")
	}
	if response.Keepalive {
		return nil, nil
	}
	if response.ID != requestID || response.Meta.RuntimeID != s.runtime {
		return nil, stateIdentityError("request/runtime identity changed")
	}
	if !response.OK {
		if response.Error != nil {
			return nil, response.Error
		}
		return nil, errors.New("Orca state request was rejected")
	}
	return response.Result, nil
}

func (s *stateSocket) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id, err := s.request(ctx, method, params)
	if err != nil {
		return nil, err
	}
	for {
		data, err := s.read(ctx)
		if err != nil {
			return nil, err
		}
		result, err := s.response(data, id)
		if err != nil || result != nil {
			return result, err
		}
	}
}
