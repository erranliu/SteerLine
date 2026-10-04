//go:build windows

package native

import (
	"errors"
	"fmt"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"golang.org/x/sys/windows"
)

// DisconnectCandidate terminates only the exact isolated app-server handle,
// inducing a real transport/resource loss. It is a test fault, never a product
// stop receipt. The original session/worktree/database records remain untouched.
func DisconnectCandidate(e executionmodel.ExecutionRecord, ownedRoot string) error {
	rel, err := filepath.Rel(ownedRoot, e.Workspace.Worktree)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) || e.Backend != "local" || e.Process == nil || e.Process.Kind != "os" || e.Process.PID <= 0 {
		return errors.New("fault target is not an owned local candidate")
	}
	bound := false
	for _, data := range e.ImplementationData {
		if data.ImplementationKey == codexworker.ExecutionBindingCodecKey {
			binding, err := codexworker.DecodeAppServerBindingEnvelope(data, codexworker.AppServerBindingIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest})
			if err != nil {
				return err
			}
			bound = binding.SessionID == e.SessionID && binding.SessionID != ""
		}
	}
	if !bound {
		return errors.New("fault target has no original native binding")
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(e.Process.PID))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return err
	}
	image := make([]uint16, 32768)
	size := uint32(len(image))
	if err := windows.QueryFullProcessImageName(h, 0, &image[0], &size); err != nil {
		return err
	}
	if fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime) != e.Process.Created || !strings.EqualFold(filepath.Clean(windows.UTF16ToString(image[:size])), filepath.Clean(e.Process.Executable)) {
		return errors.New("fault PID has changed its original identity")
	}
	if err := windows.TerminateProcess(h, 1); err != nil {
		return err
	}
	status, err := windows.WaitForSingleObject(h, 5000)
	if err != nil {
		return err
	}
	if status != windows.WAIT_OBJECT_0 {
		return errors.New("original fault target termination unverified")
	}
	return nil
}
