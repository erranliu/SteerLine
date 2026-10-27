//go:build e2e

package remote

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestRemoteExecutionProfiles(t *testing.T)  { testenv.RunRemoteDiscovery(t) }
func TestRemoteProjectProgress(t *testing.T)    { testenv.RunProjectProgress(t) }
func TestRemoteProjectEnvironment(t *testing.T) { testenv.RunRemoteEnvironment(t) }
