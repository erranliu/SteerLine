package app

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func (p *runtimeProjectPort) readAsyncActivities(ctx context.Context, values url.Values) (any, error) {
	invalid := func() (any, error) {
		return nil, &httpclient.ClientError{Status: 400, Code: "invalid_query", Reason: "invalid activities query"}
	}
	if values.Get("mode") == "abandoned" {
		source := values.Get("source")
		ids := strings.Split(values.Get("source_ids"), ",")
		if len(values) != 3 || len(values["mode"]) != 1 || len(values["source"]) != 1 || len(values["source_ids"]) != 1 || !slices.Contains(query.AsyncActivitySources, source) || len(ids) > 100 {
			return invalid()
		}
		for _, id := range ids {
			if id == "" || len(id) > 128 {
				return invalid()
			}
		}
		store, ok := p.store.(ports.ActivityAbandonmentStore)
		if !ok {
			return nil, &httpclient.ClientError{Status: 503, Code: "activities_unavailable", Reason: "activity reader unavailable"}
		}
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return store.ReadAbandonedActivities(ctx, p.config.Identity(), source, ids)
	}
	for key, list := range values {
		if len(list) != 1 || key != "filter" && key != "cursor" && key != "limit" && key != "request_ids" && key != "mode" {
			return invalid()
		}
	}
	mode := values.Get("mode")
	if mode != "" && mode != "links" {
		return invalid()
	}
	if mode == "links" && (values.Has("filter") || values.Has("cursor") || values.Has("limit") || values.Get("request_ids") == "") {
		return invalid()
	}
	limit := 50
	if text := values.Get("limit"); text != "" {
		n, e := strconv.Atoi(text)
		if e != nil || n < 1 || n > 200 {
			return invalid()
		}
		limit = n
	}
	reader, ok := p.store.(ports.AsyncActivityReader)
	if !ok {
		return nil, &httpclient.ClientError{Status: 503, Code: "activities_unavailable", Reason: "activity reader unavailable"}
	}
	options := query.AsyncActivityOptions{PublicProjectID: string(httpcontract.ProjectID(p.config.Identity())), Filter: values.Get("filter"), Cursor: values.Get("cursor"), Limit: limit, RequestIDs: query.ParseAsyncActivityRequestIDs(values.Get("request_ids"))}
	var result any
	var err error
	if mode == "links" {
		result, err = query.ReadAsyncActivityLinks(ctx, reader, p.config.Identity(), p.instance, options)
	} else {
		result, err = query.ReadAsyncActivities(ctx, reader, p.config.Identity(), p.instance, options)
	}
	if err != nil {
		return invalid()
	}
	return result, nil
}
