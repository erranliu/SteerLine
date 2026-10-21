package testmanagement

import (
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/pathguard"
	m "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func NewRecordID() string { return randomID() }

type RegistrationFile struct {
	ID    string
	Path  string
	Input m.RegisterRecord
	Error error
}

type RegistrationResult struct {
	Status     string          `json:"status"`
	Diagnostic string          `json:"diagnostic,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}
type FileRegistrationReceipt struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	RequestID string `json:"request_id"`
	RegistrationResult
}

// RegisterFiles is the common local registration journal for both script
// entry points. Publish the request before sending and preserve earlier
// attempts, including an unknown transport outcome, on explicit resubmission.
func RegisterFiles(dir string, items []RegistrationFile, send func(m.RegisterRecord, string) RegistrationResult) ([]FileRegistrationReceipt, error) {
	const maxReceiptBytes = 1 << 20
	receipts := []FileRegistrationReceipt{}
	raw, err := readRegistrationFile(dir, "registration.json", maxReceiptBytes)
	if err == nil {
		if err = Decode(raw, &receipts); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	publish := func() error {
		body, err := json.MarshalIndent(receipts, "", "  ")
		if err != nil {
			return err
		}
		if len(body)+1 > maxReceiptBytes {
			return errors.New("registration journal exceeds 1 MiB; original reports retained")
		}
		return WriteRecordJSON(filepath.Join(dir, "registration.json"), receipts)
	}
	start := len(receipts)
	for _, item := range items {
		receipt := FileRegistrationReceipt{ID: item.ID, Path: item.Path, RequestID: NewRecordID(), RegistrationResult: RegistrationResult{Status: "unknown"}}
		receipts = append(receipts, receipt)
		if err = publish(); err != nil {
			return nil, err
		}
		if item.Error != nil {
			receipt.RegistrationResult = RegistrationResult{Status: "rejected", Diagnostic: item.Error.Error()}
		} else {
			receipt.RegistrationResult = send(item.Input, receipt.RequestID)
		}
		receipts[len(receipts)-1] = receipt
		if err = publish(); err != nil {
			return nil, err
		}
	}
	return receipts[start:], nil
}

var errUnsafeRecordFile = errors.New("report/evidence must be a plain file inside report directory")

func readRegistrationFile(root, name string, max int) ([]byte, error) {
	target := filepath.Join(root, name)
	if filepath.IsAbs(name) || filepath.Base(name) != name || !pathguard.Within(root, target) {
		return nil, errUnsafeRecordFile
	}
	info, err := os.Lstat(target)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errUnsafeRecordFile
	}
	if err := pathguard.RejectHardLinkedFile(target); err != nil {
		return nil, errUnsafeRecordFile
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errUnsafeRecordFile
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(max+1)))
	if err != nil {
		return nil, err
	}
	if len(body) > max {
		return nil, errors.New("file exceeds registration byte budget")
	}
	return body, nil
}

// Read only the caller's new report directory and explicitly declared summary.
// A missing/large summary is an observation, not a reason to lose the record.
func ReadRegistrationDirectory(dir string) ([]RegistrationFile, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("report directory must be absolute")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []RegistrationFile{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "managed-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		item := RegistrationFile{Path: entry.Name()}
		raw, err := readRegistrationFile(dir, entry.Name(), m.MaxReportBytes)
		if err != nil {
			item.Error = err
			out = append(out, item)
			continue
		}
		r, err := ParseReport(raw)
		if err != nil {
			item.Error = err
			out = append(out, item)
			continue
		}
		item.ID = r.ID
		item.Input.Report = raw
		body, err := readRegistrationFile(dir, r.Evidence.Path, m.MaxSummaryBytes)
		if errors.Is(err, errUnsafeRecordFile) {
			item.Error = err
		} else if err != nil {
			item.Input.Diagnostic = err.Error()
		} else if len(body) != r.Evidence.Bytes || ContentDigest(body) != r.Evidence.SHA256 {
			item.Error = errors.New("summary digest differs from immutable declaration")
		} else {
			item.Input.Summary = body
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, errors.New("no managed test reports in this directory")
	}
	return out, nil
}

// Atomic publication is also used for per-report registration receipts. It
// never changes an already published report or its execution outcome.
func WriteRecordJSON(target string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".record-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(body, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), target)
}
