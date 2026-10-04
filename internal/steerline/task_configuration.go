package steerline

import (
	"errors"
)

func requireNativeTaskAuthority(c Config) error {
	if c.TaskAuthority != "native" {
		return errors.New("task_authority=native is required; Beads is not an active product task source")
	}
	return nil
}
