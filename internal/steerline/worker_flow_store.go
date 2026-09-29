package steerline

import (
	"context"
	"os"
	"path/filepath"
)

// preflightWorkerFlowState is read-only and runs before workspace preparation.
// A previous schema may represent a still-running external worker;
// it must not be converted or overwritten on startup.
func preflightWorkerFlowState(ctx context.Context, c Config) error {
	if c.WorkflowProtocol != WorkerFlowVersion {
		return nil
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	reader, err := OpenNativeTaskReader(ctx, &c)
	if err != nil {
		return err
	}
	defer reader.Close()
	payload, _, err := reader.LoadProjection(ctx, c.Identity())
	if err != nil {
		return err
	}
	if len(payload) != 0 {
		var runtime ServiceCheckpoint
		if err := decode(payload, &runtime); err != nil || runtime.SchemaVersion != WorkerFlowRuntimeSchema {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses the old runtime projection; preserve the original state and external session")
		}
	}
	snapshot, err := reader.Query(ctx, c.Identity())
	if err != nil {
		return err
	}
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.WorkflowVersion != WorkerFlowVersion {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses an old WorkflowRun; preserve the original state and external session")
		}
	}
	for _, execution := range snapshot.Executions {
		if execution.Backend == string(BackendLocal) && execution.Protocol != string(ProtocolLocalCodexAppServer) {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses an old local execution protocol; preserve its external process and state")
		}
	}
	return nil
}
