package applicationclient

import (
	"context"
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"net/url"
	"strconv"
)

// RecordRegistrationStatus distinguishes a known rejection from an unverified
// transport result. Retrying explicitly always reuses the immutable report ID.
func RecordRegistrationStatus(err error) string {
	if err == nil {
		return "registered"
	}
	var local *recordRegistrationError
	if errors.As(err, &local) {
		if local.status == "rejected" {
			return "rejected"
		}
		return "unknown"
	}
	var remote *httpclient.ClientError
	if errors.As(err, &remote) && remote.Status >= 400 && remote.Status < 500 {
		return "rejected"
	}
	return "unknown"
}

type recordRegistrationError struct{ status, message string }

func (e *recordRegistrationError) Error() string { return e.message }

func (p *Project) RegisterTestRecord(ctx context.Context, in api.TestRecordRegistration, requestID string) (json.RawMessage, error) {
	var r struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(in.Report, &r) != nil {
		return nil, &recordRegistrationError{"rejected", "invalid report"}
	}
	envelope := p.envelope(requestID, api.RegisterTestRecord, r.ID, 0, 0, in)
	wire, err := json.Marshal(envelope)
	if err != nil || len(wire) > 1<<20 {
		return nil, &recordRegistrationError{"rejected", "encoded registration exceeds 1 MiB command budget"}
	}
	out, err := p.submit(ctx, envelope)
	if err != nil {
		return nil, err
	}
	if out.Status != "applied" {
		return nil, &recordRegistrationError{out.Status, out.Status + ": " + out.Diagnostic}
	}
	return out.TestRecord, nil
}
func (p *Project) TestCatalogue(ctx context.Context) (out json.RawMessage, err error) {
	err = p.get(ctx, "/tests/catalogue", &out)
	return
}
func (p *Project) TestRecords(ctx context.Context, f api.TestRecordFilter) (out json.RawMessage, err error) {
	q := url.Values{}
	fields := map[string]string{"test_id": f.TestID, "search": f.Search, "level": f.Level, "outcome": f.Outcome, "retest": f.Retest, "environment": f.Environment, "source": f.Source, "from": f.From, "to": f.To, "cursor": f.Cursor}
	for k, v := range fields {
		if v != "" {
			q.Set(k, v)
		}
	}
	if f.Limit != 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	err = p.get(ctx, "/tests/records?"+q.Encode(), &out)
	return
}
func (p *Project) TestRecord(ctx context.Context, id string) (out json.RawMessage, err error) {
	err = p.get(ctx, "/tests/records/"+url.PathEscape(id), &out)
	return
}
