package steerline

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/erranliu/SteerLine/internal/execution"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

const ControlProtocolVersion = 1

const maxControlRequestBytes = 3 << 20

var controlIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func controlRequestPath(runDir, id string) string {
	return filepath.Join(runDir, "control", "requests", id, "request.json")
}

func controlReceiptPath(runDir, id string) string {
	return filepath.Join(runDir, "control", "receipts", id+".json")
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("file exceeds size limit")
	}
	return b, nil
}

func publishControlRequest(runDir string, req productapi.ControlRequest) error {
	if !controlIDPattern.MatchString(req.RequestID) {
		return errors.New("request ID must be 32 lowercase hexadecimal characters")
	}
	b, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > maxControlRequestBytes {
		return errors.New("control request exceeds 3 MiB")
	}
	path := controlRequestPath(runDir, req.RequestID)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if existing, e := readBoundedFile(path, maxControlRequestBytes); e == nil {
		var old productapi.ControlRequest
		if decode(existing, &old) != nil || execution.RequestDigest(old) != execution.RequestDigest(req) {
			return errors.New("request ID conflict: an immutable request with different content already exists")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".request-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(b); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}

	if err = os.Link(tmpName, path); err != nil {
		if !os.IsExist(err) {
			return err
		}
		existing, readErr := readBoundedFile(path, maxControlRequestBytes)
		if readErr != nil {
			return readErr
		}
		var old productapi.ControlRequest
		if decode(existing, &old) != nil || execution.RequestDigest(old) != execution.RequestDigest(req) {
			return errors.New("request ID conflict: an immutable request with different content already exists")
		}
	}
	return nil
}

func readControlRequest(path string) (productapi.ControlRequest, error) {
	var req productapi.ControlRequest
	b, err := readBoundedFile(path, maxControlRequestBytes)
	if err != nil {
		return req, err
	}
	if len(b) == 0 {
		return req, errors.New("invalid control request size")
	}
	if err = decode(b, &req); err != nil {
		return req, err
	}
	return req, nil
}

func DefaultRequestedBy() string {
	return AnswererName()
}
