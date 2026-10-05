package client

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

const nativeTaskGraphProtocol = productapi.TaskGraphProtocol

type TaskGraphExportError struct {
	Code    string
	Message string
}

func (e *TaskGraphExportError) Error() string { return e.Code + ": " + e.Message }

func taskGraphExportError(code, message string) error {
	return &TaskGraphExportError{Code: code, Message: message}
}

// WriteTaskGraphExport atomically publishes a graph in the destination
// directory. A destination without overwrite uses a hard-link publication to
// make the no-replace check race-safe; the existing file is never removed first.
func WriteTaskGraphExport(path string, graph productapi.GraphPage, overwrite bool) (productapi.TaskGraphExportReceipt, error) {
	if graph.Protocol != nativeTaskGraphProtocol || graph.ProjectID == "" || graph.Filters.ProjectID != graph.ProjectID {
		return productapi.TaskGraphExportReceipt{}, taskGraphExportError("invalid_export", "export requires a native task graph query result")
	}
	if len(graph.Nodes) > productapi.MaxTaskGraphNodes || graph.Boundary.Truncated && !graph.Filters.Filtered() {
		return productapi.TaskGraphExportReceipt{}, taskGraphExportError("export_limit_exceeded", "complete task graph exceeds the export node limit")
	}
	if strings.TrimSpace(path) == "" {
		return productapi.TaskGraphExportReceipt{}, taskGraphExportError("invalid_output", "output path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return productapi.TaskGraphExportReceipt{}, err
	}
	existed := false
	if _, statErr := os.Lstat(absolute); statErr == nil {
		existed = true
		if !overwrite {
			return productapi.TaskGraphExportReceipt{}, taskGraphExportError("output_exists", "output already exists; pass -overwrite to replace it")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return productapi.TaskGraphExportReceipt{}, statErr
	}
	body, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return productapi.TaskGraphExportReceipt{}, err
	}
	body = append(body, '\n')
	dir := filepath.Dir(absolute)
	base := filepath.Base(absolute)
	temp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return productapi.TaskGraphExportReceipt{}, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = temp.Write(body); err != nil {
		_ = temp.Close()
		return productapi.TaskGraphExportReceipt{}, err
	}
	if err = temp.Sync(); err != nil {
		_ = temp.Close()
		return productapi.TaskGraphExportReceipt{}, err
	}
	if err = temp.Close(); err != nil {
		return productapi.TaskGraphExportReceipt{}, err
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
				return productapi.TaskGraphExportReceipt{}, taskGraphExportError("output_exists", "output appeared before publication; existing file was preserved")
			}
		}
		return productapi.TaskGraphExportReceipt{}, err
	}
	return productapi.TaskGraphExportReceipt{
		Output: absolute, ProjectID: graph.ProjectID, NodeCount: len(graph.Nodes),
		GraphRevision: graph.GraphRevision, DatabaseID: graph.DatabaseID,
		DatabaseRevision: graph.DatabaseRevision, SampledAt: graph.SampledAt.UTC().Format(time.RFC3339Nano),
		Overwritten: existed, Boundary: graph.Boundary,
	}, nil
}
