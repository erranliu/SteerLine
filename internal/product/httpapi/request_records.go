package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// requestRecord is the private HTTP arbitration format. Product authority
// DTOs and response originals are reconstructed on read, never duplicated.
type requestRecord struct {
	Version   int                `json:"version"`
	Digest    string             `json:"digest"`
	Envelope  CommandEnvelope    `json:"envelope"`
	Control   *api.ControlInput  `json:"control,omitempty"`
	Discard   *api.DiscardInput  `json:"discard,omitempty"`
	Decision  *api.DecisionInput `json:"decision,omitempty"`
	Response  *requestOutcome    `json:"response,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

const (
	maxRequestRecordBytes  = maxCommandBytes + (32 << 10)
	maxRequestOutcomeBytes = 16 << 10
	maxOutcomeTextBytes    = 1024
)

type requestOutcome struct {
	Status     string    `json:"status"`
	Code       string    `json:"code,omitempty"`
	Diagnostic string    `json:"diagnostic,omitempty"`
	SampledAt  time.Time `json:"sampled_at"`
}

// requestRecordAccess is the only storage capability available to the
// command registry. Implementations own the private record representation.
type requestRecordAccess interface {
	Read(string) (requestRecord, error)
	Register(requestRecord) error
	Write(requestRecord) error
}

type fileRequestRecordOwner struct{ dir string }

func newFileRequestRecordOwner(dir string) *fileRequestRecordOwner {
	return &fileRequestRecordOwner{dir: dir}
}

func (o *fileRequestRecordOwner) Read(requestID string) (requestRecord, error) {
	if !requestIDPattern.MatchString(requestID) {
		return requestRecord{}, errors.New("request arbitration record identity is invalid")
	}
	return readRequestRecord(filepath.Join(o.dir, requestID+".json"))
}

func (o *fileRequestRecordOwner) Register(record requestRecord) error {
	if !requestIDPattern.MatchString(record.Envelope.RequestID) {
		return errors.New("request arbitration record identity is invalid")
	}
	data, err := marshalRequestRecord(record)
	if err != nil {
		return err
	}
	return writePrivateExclusive(o.dir, filepath.Join(o.dir, record.Envelope.RequestID+".json"), data)
}

func (o *fileRequestRecordOwner) Write(record requestRecord) error {
	if !requestIDPattern.MatchString(record.Envelope.RequestID) {
		return errors.New("request arbitration record identity is invalid")
	}
	data, err := marshalRequestRecord(record)
	if err != nil {
		return err
	}
	return writePrivateAtomic(o.dir, filepath.Join(o.dir, record.Envelope.RequestID+".json"), data)
}

func outcomeOf(response CommandResponse) *requestOutcome {
	return &requestOutcome{Status: response.Status, Code: boundedOutcomeText(response.Code),
		Diagnostic: boundedOutcomeText(response.Diagnostic), SampledAt: response.SampledAt}
}

func boundedOutcomeText(value string) string {
	if len(value) <= maxOutcomeTextBytes {
		return value
	}
	end := maxOutcomeTextBytes - len(" [truncated]")
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + " [truncated]"
}

func (record requestRecord) response() CommandResponse {
	envelope := record.Envelope
	return CommandResponse{RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID,
		Action: envelope.Action, TargetID: envelope.TargetID, ExpectedVersion: envelope.ExpectedVersion,
		Original: envelopePtr(envelope), Status: record.Response.Status, Code: record.Response.Code,
		Diagnostic: record.Response.Diagnostic, SampledAt: record.Response.SampledAt}
}

func readRequestRecord(path string) (requestRecord, error) {
	var record requestRecord
	if err := verifyPrivateFile(path); err != nil {
		return record, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if len(data) == 0 || len(data) > maxRequestRecordBytes {
		return record, errors.New("request arbitration record is empty or too large")
	}
	if err = decodeStrict(data, &record); err != nil {
		return record, err
	}
	if record.Version != 1 || !requestIDPattern.MatchString(record.Envelope.RequestID) || record.Envelope.RequestID+".json" != filepath.Base(path) || record.Digest == "" {
		return requestRecord{}, errors.New("request arbitration record identity is invalid")
	}
	canonical, err := json.Marshal(record.Envelope)
	if err != nil || digestBytes(canonical) != record.Digest {
		return requestRecord{}, errors.New("request arbitration record digest does not match its immutable envelope")
	}
	if _, err := marshalRequestRecord(record); err != nil {
		return requestRecord{}, err
	}
	return record, nil
}

func marshalRequestRecord(record requestRecord) ([]byte, error) {
	base := record
	base.Response = nil
	data, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	if len(data)+1 > maxRequestRecordBytes-maxRequestOutcomeBytes {
		return nil, &protocolError{status: http.StatusRequestEntityTooLarge, code: "request_record_too_large",
			reason: "encoded envelope and mapped intent exceed the 1064960-byte request record admission budget"}
	}
	if record.Response != nil {
		switch record.Response.Status {
		case "submitted", "accepted", "applied", "rejected", "outcome_unknown":
		default:
			return nil, errors.New("request arbitration outcome status is invalid")
		}
		outcome, err := json.Marshal(record.Response)
		if err != nil {
			return nil, err
		}
		if len(outcome)+len(`,"response":`) > maxRequestOutcomeBytes {
			return nil, errors.New("request arbitration outcome exceeds its reserved budget")
		}
		data, err = json.Marshal(record)
		if err != nil {
			return nil, err
		}
	}
	data = append(data, '\n')
	if len(data) > maxRequestRecordBytes {
		return nil, errors.New("request arbitration record exceeds its read budget")
	}
	return data, nil
}

func digestBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
