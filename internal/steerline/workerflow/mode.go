package workerflow

import "errors"

// Mode selects the worker protocol fixed by the execution record.
// It does not own persistence or platform processes.
type Mode uint8

const (
	LocalCodex Mode = iota + 1
	NativeOrca
	SupervisedOrca
)

func SelectMode(protocol, host string) (Mode, error) {
	switch {
	case protocol == "local_codex_app_server_v1" && host == "local":
		return LocalCodex, nil
	case protocol == "orca_terminal_v2" && host == "orca":
		return NativeOrca, nil
	case protocol == "orca_supervised" && host == "orca_supervised":
		return SupervisedOrca, nil
	default:
		return 0, errors.New("fixed worker protocol does not match its execution host")
	}
}

// PendingIdentity distinguishes a command intent from a proven native turn.
func (m Mode) PendingIdentity(id string) (turnID, commandID string) {
	if m == LocalCodex {
		return "", id
	}
	return id, ""
}
