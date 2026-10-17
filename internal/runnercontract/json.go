package runnercontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// CanonicalJSON rejects duplicate keys before structs can erase ambiguity,
// preserves explicit values and array order, and sorts all object keys.
func CanonicalJSON(raw []byte) ([]byte, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("configuration must be valid UTF-8 JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 64 {
			return nil, errors.New("configuration JSON is too deeply nested")
		}
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch token {
		case json.Delim('{'):
			value := map[string]any{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return nil, err
				}
				name, ok := key.(string)
				if !ok {
					return nil, errors.New("invalid configuration key")
				}
				if _, exists := value[name]; exists {
					return nil, errors.New("duplicate configuration JSON key")
				}
				next, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				value[name] = next
			}
			_, err = d.Token()
			return value, err
		case json.Delim('['):
			value := []any{}
			for d.More() {
				next, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				value = append(value, next)
			}
			_, err = d.Token()
			return value, err
		default:
			return token, nil
		}
	}
	value, err := read(0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errors.New("expected exactly one configuration JSON value")
	}
	return json.Marshal(value)
}

func ConfigurationDigest(execution ExecutionInput, workflow Workflow) string {
	raw, err := json.Marshal(struct {
		Execution ExecutionInput `json:"execution"`
		Workflow  Workflow       `json:"workflow"`
	}{execution, workflow})
	if err != nil {
		return ""
	}
	raw, err = CanonicalJSON(raw)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
