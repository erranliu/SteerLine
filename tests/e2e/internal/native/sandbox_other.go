//go:build !windows

package native

import "errors"

func validateSandboxAccounts(sandboxUsers) error {
	return errors.New("this live E2E environment requires native Windows sandbox")
}
