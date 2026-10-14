package httpclient

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"

	api "github.com/erranliu/SteerLine/internal/product/api"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
	fs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

type outgoing struct {
	Version      int                   `json:"version"`
	Envelope     wire.CommandEnvelope  `json:"envelope"`
	Response     *wire.CommandResponse `json:"response,omitempty"`
	StopIdentity *wire.Identity        `json:"stop_identity,omitempty"`
}

// requestJournal owns only the client's durable original requests and the
// observations collected for those requests.
type requestJournal struct {
	home     string
	identity string
}

func newRequestJournal(ref api.ProjectRef) *requestJournal {
	return &requestJournal{home: ref.ApplicationHome, identity: ref.Identity}
}

func (j *requestJournal) recordPath(id string) (string, error) {
	if !validRequestID(id) {
		return "", errors.New("invalid request ID")
	}
	return filepath.Join(j.home, "ui", "http-requests", wire.ProjectID(j.identity), id+".json"), nil
}

func (j *requestJournal) lockPath(id string) (string, error) {
	if !validRequestID(id) {
		return "", errors.New("invalid request ID")
	}
	return filepath.Join(j.home, "ui", "http-request-locks", wire.ProjectID(j.identity), id+".lock"), nil
}

func validRequestID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && hex.EncodeToString(b) == id
}

func (j *requestJournal) read(id string) (outgoing, error) {
	var value outgoing
	path, err := j.recordPath(id)
	if err != nil {
		return value, err
	}
	if err = fs.VerifyPrivateFile(path); err != nil {
		return value, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if len(data) > 4<<20 {
		return value, errors.New("outgoing envelope too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&value)
	if err == nil && decoder.Decode(new(any)) != io.EOF {
		err = errors.New("outgoing record has trailing JSON")
	}
	if err == nil {
		err = j.validate(id, value)
	}
	return value, err
}

func (j *requestJournal) validate(id string, value outgoing) error {
	if value.Version != 1 || value.Envelope.RequestID != id || value.Envelope.ProjectID != wire.ProjectID(j.identity) {
		return errors.New("outgoing envelope identity/format invalid")
	}
	if value.Response != nil {
		if err := validateResponse(value.Envelope, *value.Response); err != nil {
			return err
		}
	}
	if value.Envelope.Action == "stop_service" {
		if value.StopIdentity == nil || value.StopIdentity.InstanceID != value.Envelope.InstanceID || value.StopIdentity.ProjectID != value.Envelope.ProjectID || value.StopIdentity.RepositoryIdentity != j.identity {
			return errors.New("original stop identity does not match outgoing intent")
		}
	} else if value.StopIdentity != nil {
		return errors.New("non-stop request contains an original stop identity")
	}
	return nil
}

func (j *requestJournal) save(ctx context.Context, value outgoing) (outgoing, error) {
	var retained outgoing
	path, err := j.recordPath(value.Envelope.RequestID)
	if err != nil {
		return retained, err
	}
	lockPath, err := j.lockPath(value.Envelope.RequestID)
	if err != nil {
		return retained, err
	}
	if err = fs.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
		return retained, err
	}
	if err = fs.EnsurePrivateDirectory(filepath.Dir(lockPath)); err != nil {
		return retained, err
	}
	err = withRequestJournalLock(ctx, lockPath, func() error {
		var saveErr error
		retained, saveErr = j.saveLocked(ctx, value)
		return saveErr
	})
	if err != nil {
		return retained, err
	}
	return retained, nil
}

// saveLocked performs the re-read, immutable identity check and atomic publish.
// Callers must hold the stable request-ID OS lock for the entire method.
func (j *requestJournal) saveLocked(ctx context.Context, value outgoing) (outgoing, error) {
	var retained outgoing
	path, err := j.recordPath(value.Envelope.RequestID)
	if err != nil {
		return retained, err
	}
	if err = fs.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
		return retained, err
	}
	if err = ctx.Err(); err != nil {
		return retained, err
	}
	if err = j.validate(value.Envelope.RequestID, value); err != nil {
		return retained, err
	}
	previous, previousErr := j.read(value.Envelope.RequestID)
	if previousErr != nil && !os.IsNotExist(previousErr) {
		return retained, previousErr
	}
	if previousErr == nil {
		if !reflect.DeepEqual(previous.Envelope, value.Envelope) || !reflect.DeepEqual(previous.StopIdentity, value.StopIdentity) {
			return retained, errors.New("request ID conflict: durable outgoing intent differs")
		}
		if previous.Response != nil && terminalResponse(previous.Response.Status) {
			if value.Response == nil || !terminalResponse(value.Response.Status) || value.Response.Status == previous.Response.Status {
				return previous, nil
			}
			return previous, errors.New("request ID conflict: terminal response status differs")
		}
		if value.Response == nil {
			return previous, nil
		}
	}
	if err = ctx.Err(); err != nil {
		return retained, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return retained, err
	}
	if len(data) > 4<<20 {
		return retained, errors.New("outgoing envelope too large")
	}
	file, err := fs.CreatePrivateTemp(filepath.Dir(path), ".outgoing-")
	if err != nil {
		return retained, err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return retained, err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return retained, err
	}
	if err = file.Close(); err != nil {
		return retained, err
	}
	if err = ctx.Err(); err != nil {
		return retained, err
	}
	if err = fs.ReplacePrivateFile(name, path); err != nil {
		return retained, err
	}
	return value, nil
}

func terminalResponse(status string) bool { return status == "applied" || status == "rejected" }
