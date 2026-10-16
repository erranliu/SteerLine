package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
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
