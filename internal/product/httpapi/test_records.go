package httpapi

import (
	"context"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"net/url"
	"strconv"
)

func (s *Server) readTestRecords(ctx context.Context, path []string, q url.Values) (any, error) {
	p, ok := s.query.(api.TestRecordsPort)
	if !ok {
		return nil, &api.Error{Kind: api.Unsupported, Message: "test records unavailable"}
	}
	if len(path) == 2 && path[1] == "records" {
		allowed := map[string]bool{"test_id": true, "search": true, "level": true, "outcome": true, "environment": true, "source": true, "from": true, "to": true, "cursor": true, "limit": true}
		for k, values := range q {
			if !allowed[k] || len(values) != 1 {
				return nil, invalidCommand("unsupported or repeated test record filter")
			}
		}
		limit := 0
		if q.Has("limit") {
			var err error
			limit, err = strconv.Atoi(q.Get("limit"))
			if err != nil || limit < 1 || limit > 100 {
				return nil, invalidCommand("invalid limit")
			}
		}
		return p.TestRecords(ctx, api.TestRecordFilter{TestID: q.Get("test_id"), Search: q.Get("search"), Level: q.Get("level"), Outcome: q.Get("outcome"), Environment: q.Get("environment"), Source: q.Get("source"), From: q.Get("from"), To: q.Get("to"), Cursor: q.Get("cursor"), Limit: limit})
	}
	if len(q) != 0 {
		return nil, invalidCommand("test detail query accepts no parameters")
	}
	if len(path) == 2 && path[1] == "catalogue" {
		return p.TestCatalogue(ctx)
	}
	if len(path) >= 3 && path[1] == "records" && requestIDPattern.MatchString(path[2]) {
		if len(path) == 3 {
			return p.TestRecord(ctx, path[2])
		}
	}
	return nil, &protocolError{status: 404, code: "route_not_found", reason: "unknown test record route"}
}
