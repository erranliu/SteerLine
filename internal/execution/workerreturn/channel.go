// Package workerreturn defines the shared compact terminal reply contract.
// It has no file channel, callback server, agent-specific parser or task authority.
package workerreturn

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

const Source = "steerline.worker.terminal-result.v1"
const MaxFrameBytes = 2048
const MaxPromptRunes = 2000
const MaxReplyBytes = 32 << 10
const frameStart = "SL_RESULT"
const frameEnd = "SL_END"

// Reply describes this input's outcome; it cannot complete a business Task.
type Reply struct {
	Outcome string `json:"outcome"`
	Text    string `json:"reply"`
}

func (r Reply) Validate() error {
	if r.Outcome != "succeeded" && r.Outcome != "failed" {
		return errors.New("invalid worker reply outcome")
	}
	if len(r.Text) > MaxReplyBytes || !utf8.ValidString(r.Text) || strings.TrimSpace(r.Text) == "" {
		return errors.New("worker reply is empty, invalid UTF-8 or oversized")
	}
	return nil
}

type WireReply struct {
	Nonce   string `json:"nonce"`
	Outcome string `json:"outcome"`
	Body    string `json:"reply_b64"`
}

func wireReply(command string, reply Reply) WireReply {
	return WireReply{Nonce: command, Outcome: reply.Outcome, Body: base64.StdEncoding.EncodeToString([]byte(reply.Text))}
}

func Digest(data []byte) string { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }
func validID(value string) bool {
	data, err := hex.DecodeString(value)
	return err == nil && len(data) == 16 && value == strings.ToLower(value)
}

// Prompt is called by the common worker layer before its exact input is saved.
// The instruction contains no complete example frame which a terminal could echo.
func Prompt(prompt, command string) (string, error) {
	if !validID(command) || !utf8.ValidString(prompt) || strings.TrimSpace(prompt) == "" {
		return "", errors.New("invalid compact worker input")
	}
	instruction := fmt.Sprintf("\nFinal reply: one line, prefix SL_RESULT, then compact JSON with nonce=%q, outcome=succeeded or failed, reply_b64=base64 of the exact UTF-8 step reply (encode in memory, no files), then suffix SL_END. Keep the whole frame within %d UTF-8 bytes. No other final text.", command, MaxFrameBytes)
	text := prompt
	if !strings.HasSuffix(prompt, instruction) {
		text += instruction
	}
	if utf8.RuneCountInString(text) > MaxPromptRunes {
		return "", fmt.Errorf("terminal input exceeds %d characters including its reply contract; shorten the input", MaxPromptRunes)
	}
	return text, nil
}
func Encode(command string, reply Reply) (string, error) {
	if !validID(command) {
		return "", errors.New("invalid reply command")
	}
	if err := reply.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(wireReply(command, reply))
	if err != nil {
		return "", err
	}
	if len(data)+len(frameStart)+len(frameEnd)+2 > MaxFrameBytes {
		return "", errors.New("terminal result exceeds compact frame limit")
	}
	return frameStart + " " + string(data) + " " + frameEnd, nil
}

// Decode rejects duplicate, unknown and trailing fields in the reply envelope.
func Decode(data []byte, target any) error {
	if len(data) > MaxFrameBytes || !utf8.Valid(data) {
		return errors.New("invalid compact reply bytes")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("reply must be an object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return errors.New("duplicate reply field")
		}
		seen[key] = true
		var field json.RawMessage
		if err = decoder.Decode(&field); err != nil {
			return err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("trailing reply data")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// Scan stores only an incomplete frame and the unique reply for one input.
// Repainted identical frames are harmless; conflicting frames remain unverified.
type Scan struct {
	Pending string `json:"pending,omitempty"`
	Result  *Reply `json:"result,omitempty"`
	Digest  string `json:"digest,omitempty"`
	Problem string `json:"problem,omitempty"`
}

var ansiCSI = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")

func (s *Scan) Append(text, command string) {
	if s.Problem != "" {
		return
	}
	data := s.Pending + strings.ReplaceAll(ansiCSI.ReplaceAllString(text, ""), "\r", "")
	s.Pending = ""
	for {
		start := strings.Index(data, frameStart)
		if start < 0 {
			for size := len(frameStart) - 1; size > 0; size-- {
				if strings.HasSuffix(data, frameStart[:size]) {
					s.Pending = data[len(data)-size:]
					break
				}
			}
			return
		}
		candidate := data[start:]
		rest := strings.TrimLeft(candidate[len(frameStart):], " \n\t")
		if rest == "" {
			s.Pending = candidate
			return
		}
		if rest[0] != '{' {
			data = candidate[len(frameStart):]
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(rest))
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			// A subsequent complete repaint supersedes an unfinished display fragment.
			if next := strings.Index(candidate[len(frameStart):], frameStart); next >= 0 {
				data = candidate[len(frameStart)+next:]
				continue
			}
			if len(candidate) > MaxFrameBytes {
				s.Problem = "compact result is oversized or incomplete"
			} else {
				s.Pending = candidate
			}
			return
		}
		offset := int(decoder.InputOffset())
		suffix := strings.TrimLeft(rest[offset:], " \n\t")
		if !strings.HasPrefix(suffix, frameEnd) {
			if suffix == "" || strings.HasPrefix(frameEnd, suffix) {
				if len(candidate) > MaxFrameBytes {
					s.Problem = "compact result is oversized"
				} else {
					s.Pending = candidate
				}
				return
			}
			data = candidate[len(frameStart):]
			continue
		}
		data = suffix[len(frameEnd):]
		var wire WireReply
		// Wire fields contain only ASCII tokens/base64. Remove display wrapping
		// before ownership validation as it can split the command nonce too.
		raw = []byte(strings.Join(strings.Fields(string(raw)), ""))
		// Ignore unrelated rounds without interpreting their outcome.
		var owner struct {
			Nonce string `json:"nonce"`
		}
		if json.Unmarshal(raw, &owner) != nil || owner.Nonce != command {
			continue
		}
		var decoded []byte
		decodeErr := Decode(raw, &wire)
		if decodeErr == nil {
			decoded, decodeErr = base64.StdEncoding.DecodeString(wire.Body)
		}
		reply := Reply{Outcome: wire.Outcome, Text: string(decoded)}
		if len(raw)+len(frameStart)+len(frameEnd)+2 > MaxFrameBytes || decodeErr != nil || reply.Validate() != nil {
			s.Problem = "original compact result is invalid or oversized"
			return
		}
		canonical, _ := json.Marshal(wire)
		digest := Digest(canonical)
		if s.Result != nil && s.Digest != digest {
			s.Problem = "original input produced conflicting compact results"
			return
		}
		s.Result = &reply
		s.Digest = digest
	}
}
func (s Scan) Validate(command string) error {
	if !validID(command) || len(s.Pending) > MaxFrameBytes || !utf8.ValidString(s.Pending) {
		return errors.New("invalid saved result scan")
	}
	if s.Result == nil {
		if s.Digest != "" {
			return errors.New("reply digest has no result")
		}
		return nil
	}
	if err := s.Result.Validate(); err != nil {
		return err
	}
	data, _ := json.Marshal(wireReply(command, *s.Result))
	if len(data)+len(frameStart)+len(frameEnd)+2 > MaxFrameBytes || s.Digest != Digest(data) {
		return errors.New("saved compact result changed")
	}
	return nil
}
