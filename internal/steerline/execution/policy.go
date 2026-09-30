package execution

import (
	"bytes"
	"encoding/json"
	"errors"
)

// ExecutionPolicy is project authority, independent of a native sandbox name.
type ExecutionPolicy struct {
	Filesystem FilesystemPolicy `json:"filesystem"`
	Network    string           `json:"network"`
	Escalation string           `json:"escalation"`
}
type FilesystemPolicy struct {
	ReadScope        string `json:"read_scope"`
	WriteScope       string `json:"write_scope"`
	GitMetadataWrite bool   `json:"git_metadata_write"`
}

func (p *FilesystemPolicy) UnmarshalJSON(data []byte) error {
	type plain FilesystemPolicy
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["git_metadata_write"]; !ok || string(raw) == "null" {
		return errors.New("execution_policy.filesystem.git_metadata_write must be an explicit bool")
	}
	var value plain
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*p = FilesystemPolicy(value)
	return nil
}

func (p ExecutionPolicy) Validate() error {
	if p.Filesystem.ReadScope != "host" {
		return errors.New("execution_policy.filesystem.read_scope must be host")
	}
	switch p.Filesystem.WriteScope {
	case "none", "workspace", "host":
	default:
		return errors.New("execution_policy.filesystem.write_scope must be none, workspace or host")
	}
	if p.Filesystem.GitMetadataWrite && p.Filesystem.WriteScope != "workspace" {
		return errors.New("git_metadata_write requires workspace write_scope")
	}
	if p.Network != "allow" && p.Network != "deny" {
		return errors.New("execution_policy.network must be allow or deny")
	}
	if p.Escalation != "deny" {
		return errors.New("execution_policy.escalation must be deny")
	}
	return nil
}
