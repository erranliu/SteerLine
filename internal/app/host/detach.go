package host

import (
	"os/exec"

	"github.com/erranliu/SteerLine/internal/hostcontract"
)

// StartDetachedProduct uses the existing suspended, verified Windows Job
// isolation before permitting the product to execute startup instructions.
func StartDetachedProduct(cmd *exec.Cmd, facts hostcontract.ProcessFactsPort) (hostcontract.ProcessIdentity, error) {
	return startDetachedProductProcess(cmd, facts)
}
