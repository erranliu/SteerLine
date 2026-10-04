package steerline

import (
 "github.com/erranliu/SteerLine/internal/execution"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func (s *ServiceHost) WriteControlReceipt(r productapi.ControlReceipt) error {
	if !controlIDPattern.MatchString(r.RequestID) {
		return errors.New("invalid receipt request ID")
	}
	path := controlReceiptPath(s.Config.RunDir, r.RequestID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(b, '\n'))
}

func rawRequestDigest(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	h := sha256.New()
	limit := int64(maxControlRequestBytes)
	if info.Size() > limit {
		limit++
	} else {
		limit = info.Size()
	}
	if _, err = io.CopyN(h, f, limit); err != nil && err != io.EOF {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *ServiceHost) ControlRequestIDs() ([]string, error) {
	root := filepath.Join(s.Config.RunDir, "control", "requests")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() && controlIDPattern.MatchString(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func findStopRequestCached(c execution.Config, instanceID string, seen map[string]bool) (*productapi.ControlRequest, string, time.Time) {
	root := filepath.Join(c.RunDir, "control", "requests")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, "", time.Time{}
	}
	for _, e := range entries {
		if !e.IsDir() || !controlIDPattern.MatchString(e.Name()) {
			continue
		}
		if seen[e.Name()] {
			continue
		}
		path := controlRequestPath(c.RunDir, e.Name())
		req, err := readControlRequest(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			if seen != nil {
				if receipt, e := ReadControlReceipt(c, e.Name()); e == nil && execution.TerminalControlStatus(receipt.Status) {
					seen[receipt.RequestID] = true
				}
			}
			continue
		}
		if seen != nil {
			seen[e.Name()] = true
		}
		if err != nil || req.Action != "stop_service" || req.InstanceID != instanceID || req.RequestID != e.Name() {
			continue
		}
		if _, err = execution.DecodeControlPayload(req); err != nil {
			continue
		}
		checkedAt := now()
		if _, err = execution.ValidateControlRequest(req, instanceID, c.Identity(), 0, checkedAt); err != nil {
			continue
		}
		if _, err = ReadControlReceipt(c, req.RequestID); err == nil {
			continue
		}
		return &req, rawRequestDigest(path), checkedAt
	}
	return nil, "", time.Time{}
}
