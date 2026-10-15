package host

import (
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	"os/exec"
)

// StartDetachedProduct uses the existing suspended, verified Windows Job
// isolation before permitting the product to execute startup instructions.
func StartDetachedProduct(cmd *exec.Cmd, facts backend.ProcessFactsPort) (backend.ProcessIdentity, error) {
	return startDetachedProductProcess(cmd, facts)
}
