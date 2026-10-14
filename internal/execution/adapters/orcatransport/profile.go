package orcatransport

import (
	"encoding/json"
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"strings"
	"time"
)

const ProfileKey = "orca.terminal-profile"
const ProfileVersion = 3
const StateKey = "orca.terminal-state"
const StateVersion = 5
const ServiceStopBehavior = b.ContinueExecutionOnServiceStop

// Environment is routing only; local and paired execution have the same ports.
type Profile struct {
	Executable  string   `json:"executable"`
	Args        []string `json:"args,omitempty"`
	Environment string   `json:"environment,omitempty"`
	WaitTimeout string   `json:"wait_timeout"`
}

func DecodeProfile(data []byte) (Profile, error) {
	var p Profile
	if err := b.DecodeStrictJSON(data, &p, "trailing Orca terminal profile"); err != nil {
		return p, err
	}
	if strings.TrimSpace(p.Executable) == "" || strings.ContainsAny(p.Environment, "\x00\r\n") {
		return p, errors.New("Orca executable is required and environment must be a single line")
	}
	if d, err := time.ParseDuration(p.WaitTimeout); err != nil || d <= 0 || d > time.Minute {
		return p, errors.New("Orca wait_timeout must be positive and at most one minute")
	}
	for _, arg := range p.Args {
		if strings.HasPrefix(arg, "--environment") || strings.HasPrefix(arg, "--pairing-code") {
			return p, errors.New("Orca routing belongs to the environment field")
		}
	}
	return p, nil
}
func NormalizeProfile(data []byte) ([]byte, []string, error) {
	p, err := DecodeProfile(data)
	if err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(p)
	target := "本机"
	if p.Environment != "" {
		target = p.Environment
	}
	return raw, []string{"Orca：终端通信 · " + target, "启动与权限：由执行主机的 Orca 原生设置管理", "结果：同一终端的普通回复"}, err
}
func (p Profile) Target() b.ExecutionTarget {
	wire, _ := json.Marshal(struct {
		Executable  string
		Args        []string
		Environment string
	}{p.Executable, append([]string(nil), p.Args...), p.Environment})
	host := p.Environment
	if host == "" {
		host = "local"
	}
	return b.ExecutionTarget{ID: "orca-terminal:" + digest(wire)[:24], ExternalRuntime: p.Environment, ExecutionHost: host}
}
