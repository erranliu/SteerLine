package applicationclient

import (
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"net/url"
	"strconv"
)

func (p *Project) TestCatalogue(ctx context.Context) (out json.RawMessage, err error) {
	err = p.get(ctx, "/tests/catalogue", &out)
	return
}
func (p *Project) TestRecords(ctx context.Context, f api.TestRecordFilter) (out json.RawMessage, err error) {
	q := url.Values{}
	fields := map[string]string{"test_id": f.TestID, "search": f.Search, "level": f.Level, "outcome": f.Outcome, "environment": f.Environment, "source": f.Source, "from": f.From, "to": f.To, "cursor": f.Cursor}
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
