package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Error carries the native task error code used by command callers.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func newError(code, message string) error { return &Error{Code: code, Message: message} }

var canonicalNativeTaskIDPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

func CanonicalDecimalID(value string) bool {
	if !canonicalNativeTaskIDPattern.MatchString(value) {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 63)
	return err == nil
}

func ValidateID(id string) error {
	if !CanonicalDecimalID(id) {
		return newError("contract_mismatch", "task ID must be a canonical nonnegative decimal number")
	}
	return nil
}

func ValidSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func ValidCommit(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func ContractDigest(v TaskContract) (string, error) {
	v.Digest = ""
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	canonical, err := CanonicalJSON(b)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func ValidateContract(v TaskContract) error {
	if v.ContractID == "" || v.Repository == "" || v.Remote == "" || v.Branch == "" {
		return newError("contract_mismatch", "contract ID and repository target are required")
	}
	if v.SpecRef.Path == v.PlanRef.Path {
		return newError("contract_mismatch", "contract must pin exactly one independent Spec and one distinct Plan")
	}
	if len(v.AcceptanceRefs) == 0 {
		return newError("contract_mismatch", "contract requires at least one acceptance reference")
	}
	refs := []FileReference{v.SpecRef, v.PlanRef}
	refs = append(refs, v.CommonRefs...)
	refs = append(refs, v.AcceptanceRefs...)
	for _, ref := range refs {
		if err := ValidateFileReference(ref); err != nil {
			return err
		}
	}
	for _, ref := range v.CommonRefs {
		if ref.Path == v.SpecRef.Path || ref.Path == v.PlanRef.Path || !sharedContractPath(ref.Path) {
			return newError("contract_mismatch", "common_refs may contain only shared constraints or protocols")
		}
	}
	for _, ref := range v.AcceptanceRefs {
		if !strings.HasSuffix(strings.ToLower(ref.Path), ".md") {
			return newError("contract_mismatch", "acceptance reference must point to a Markdown document")
		}
		if ref == v.SpecRef || ref == v.PlanRef {
			continue
		}
		if ref.Path == v.SpecRef.Path || ref.Path == v.PlanRef.Path || (taskDocumentPath(ref.Path) && !sharedContractPath(ref.Path)) {
			return newError("contract_mismatch", "acceptance_refs cannot add another task's Spec/Plan or another version of the fixed documents")
		}
	}
	digest, err := ContractDigest(v)
	if err != nil {
		return err
	}
	if !strings.EqualFold(digest, v.Digest) {
		return newError("contract_mismatch", "contract canonical digest does not match its content")
	}
	return nil
}

func taskDocumentPath(name string) bool {
	name = strings.ToLower(name)
	return strings.HasSuffix(name, ".spec.md") || strings.HasSuffix(name, ".plan.md")
}

// Shared documentation uses constraint/protocol paths; the execution
// architecture is the repository's established shared .spec.md document.
// This structural convention does not prove semantic specification quality.
func sharedContractPath(name string) bool {
	name = strings.ToLower(name)
	if path.Base(name) == "execution-architecture.spec.md" {
		return true
	}
	if taskDocumentPath(name) {
		return false
	}
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '-' || r == '_' || r == '.' }) {
		if part == "constraint" || part == "constraints" || part == "protocol" || part == "protocols" {
			return true
		}
	}
	return false
}

func ValidateFileReference(ref FileReference) error {
	if ref.Path == "" || strings.Contains(ref.Path, "\\") || path.IsAbs(ref.Path) || filepath.IsAbs(ref.Path) || regexp.MustCompile(`^[A-Za-z]:`).MatchString(ref.Path) || path.Clean(ref.Path) != ref.Path || ref.Path == "." || strings.HasPrefix(ref.Path, "../") || ref.Path == ".." {
		return newError("contract_mismatch", "specification path must be a normalized repository-relative path")
	}
	if !ValidCommit(ref.Commit) || !ValidSHA256(ref.SHA256) {
		return newError("contract_mismatch", "file reference requires a full commit and SHA-256 blob digest")
	}
	return nil
}

func ValidateContractJSON(data []byte) (TaskContract, error) {
	if _, err := CanonicalJSON(data); err != nil {
		return TaskContract{}, newError("contract_mismatch", err.Error())
	}
	var v TaskContract
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return v, newError("contract_mismatch", err.Error())
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return v, newError("contract_mismatch", "expected one JSON value")
	}
	return v, ValidateContract(v)
}

// CanonicalJSON rejects duplicate object keys and emits sorted-key, UTF-8 JSON
// with no insignificant whitespace. Array order is preserved.
func CanonicalJSON(data []byte) ([]byte, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("canonical JSON input must be valid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, err := readJSONValue(d)
	if err != nil {
		return nil, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	var out bytes.Buffer
	e := json.NewEncoder(&out)
	e.SetEscapeHTML(false)
	if err = e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(out.Bytes(), []byte{'\n'}), nil
}

func readJSONValue(d *json.Decoder) (any, error) {
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return t, nil
	}
	switch delim {
	case '{':
		m := map[string]any{}
		for d.More() {
			kt, err := d.Token()
			if err != nil {
				return nil, err
			}
			key, ok := kt.(string)
			if !ok {
				return nil, errors.New("object key is not a string")
			}
			if _, exists := m[key]; exists {
				return nil, fmt.Errorf("duplicate JSON object key %q", key)
			}
			value, err := readJSONValue(d)
			if err != nil {
				return nil, err
			}
			m[key] = value
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, errors.New("invalid JSON object")
		}
		return m, nil
	case '[':
		a := []any{}
		for d.More() {
			value, err := readJSONValue(d)
			if err != nil {
				return nil, err
			}
			a = append(a, value)
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, errors.New("invalid JSON array")
		}
		return a, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}
