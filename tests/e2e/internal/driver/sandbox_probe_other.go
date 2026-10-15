//go:build !windows

package driver

import "errors"

func NativeSandboxProbe([]string) error { return errors.New("MXC sandbox probe requires Windows") }
