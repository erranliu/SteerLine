package steerline

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/taskview"
)

type TaskGraphExportReceipt struct {
	Output           string            `json:"output"`
	ProjectID        string            `json:"project_id"`
	NodeCount        int               `json:"node_count"`
	GraphRevision    uint64            `json:"graph_revision"`
	DatabaseID       string            `json:"database_id"`
	DatabaseRevision uint64            `json:"database_revision"`
	SampledAt        string            `json:"sampled_at"`
	Overwritten      bool              `json:"overwritten"`
	Boundary         TaskGraphBoundary `json:"boundary"`
}

// WriteTaskGraphExport atomically publishes a graph in the destination
// directory. A destination without overwrite uses a hard-link publication to
// make the no-replace check race-safe; the existing file is never removed first.
func WriteTaskGraphExport(path string, graph TaskGraphPage, overwrite bool) (TaskGraphExportReceipt, error) {
	if graph.Protocol != nativeTaskGraphProtocol || graph.ProjectID == "" || graph.Filters.ProjectID != graph.ProjectID {
		return TaskGraphExportReceipt{}, nativeErr("invalid_export", "export requires a native task graph query result")
	}
	if len(graph.Nodes) > taskview.MaxNodes || graph.Boundary.Truncated && !graph.Filters.Filtered() {
		return TaskGraphExportReceipt{}, nativeErr("export_limit_exceeded", "complete task graph exceeds the export node limit")
	}
	if strings.TrimSpace(path) == "" {
		return TaskGraphExportReceipt{}, nativeErr("invalid_output", "output path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return TaskGraphExportReceipt{}, err
	}
	existed := false
	if _, statErr := os.Lstat(absolute); statErr == nil {
		existed = true
		if !overwrite {
			return TaskGraphExportReceipt{}, nativeErr("output_exists", "output already exists; pass -overwrite to replace it")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return TaskGraphExportReceipt{}, statErr
	}
	body, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return TaskGraphExportReceipt{}, err
	}
	body = append(body, '\n')
	dir := filepath.Dir(absolute)
	base := filepath.Base(absolute)
	temp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return TaskGraphExportReceipt{}, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = temp.Write(body); err != nil {
		_ = temp.Close()
		return TaskGraphExportReceipt{}, err
	}
	if err = temp.Sync(); err != nil {
		_ = temp.Close()
		return TaskGraphExportReceipt{}, err
	}
	if err = temp.Close(); err != nil {
		return TaskGraphExportReceipt{}, err
	}
	if overwrite {
		err = os.Rename(tempName, absolute)
	} else {
		err = os.Link(tempName, absolute)
		if err == nil {
			_ = os.Remove(tempName)
		}
	}
	if err != nil {
		if !overwrite && !errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Lstat(absolute); statErr == nil {
				return TaskGraphExportReceipt{}, nativeErr("output_exists", "output appeared before publication; existing file was preserved")
			}
		}
		return TaskGraphExportReceipt{}, err
	}
	return TaskGraphExportReceipt{
		Output: absolute, ProjectID: graph.ProjectID, NodeCount: len(graph.Nodes),
		GraphRevision: graph.GraphRevision, DatabaseID: graph.DatabaseID,
		DatabaseRevision: graph.DatabaseRevision, SampledAt: graph.SampledAt.UTC().Format(time.RFC3339Nano),
		Overwritten: existed, Boundary: graph.Boundary,
	}, nil
}
