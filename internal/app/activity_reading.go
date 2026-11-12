package app

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func (p *runtimeProjectPort) readActivity(ctx context.Context, route string, values url.Values) (any, error) {
	bad := func(reason string) (any, error) {
		return nil, &httpclient.ClientError{Status: 400, Code: "invalid_reading_query", Reason: reason}
	}
	parts := strings.Split(strings.TrimPrefix(route, "/activities/"), "/")
	if len(parts) > 2 || parts[0] == "" {
		return bad("invalid activity route")
	}
	if len(parts) == 2 && parts[1] == "conversation" {
		return p.readCandidateConversation(ctx, parts[0], values)
	}
	allowed := map[string]bool{}
	if len(parts) == 2 {
		switch parts[1] {
		case "interactions":
			allowed = map[string]bool{"cursor": true, "limit": true}
		case "changes":
		case "body":
			allowed = map[string]bool{"item": true, "sha256": true, "offset": true}
		default:
			return bad("unknown activity reading route")
		}
	}
	for key, v := range values {
		if !allowed[key] || len(v) != 1 || len(v[0]) > 4096 {
			return bad("invalid or repeated reading parameter")
		}
	}
	reader, ok := p.store.(interface {
		ports.AsyncActivityReader
		ports.ActivityReadingReader
	})
	if !ok {
		return nil, &httpclient.ClientError{Status: 503, Code: "reading_unavailable", Reason: "activity reading unavailable"}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p.readingOnce.Do(func() {
		if p.readingBodies == nil {
			p.readingBodies = query.NewReadingBodyCache()
		}
	})
	service := query.ReadingService{Reader: reader, Project: p.config.Identity(), Instance: p.instance, Bodies: p.readingBodies}
	var result any
	var err error
	if len(parts) == 1 {
		result, err = query.ReadOneActivity(ctx, reader, p.config.Identity(), p.instance, parts[0], string(httpcontract.ProjectID(p.config.Identity())))
	} else {
		switch parts[1] {
		case "changes":
			var version string
			version, err = service.Version(ctx, parts[0])
			result = struct {
				Version string `json:"version"`
			}{version}
		case "interactions":
			limit := 50
			if values.Has("limit") {
				limit, err = strconv.Atoi(values.Get("limit"))
				if err == nil && (limit < 1 || limit > 200) {
					return bad("reading limit outside 1..200")
				}
			}
			if err == nil {
				result, err = service.Page(ctx, parts[0], values.Get("cursor"), limit)
			}
		case "body":
			offset := 0
			if values.Has("offset") {
				offset, err = strconv.Atoi(values.Get("offset"))
			}
			if err == nil {
				result, err = service.Body(ctx, parts[0], values.Get("item"), values.Get("sha256"), offset)
			}
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &httpclient.ClientError{Status: 404, Code: "activity_not_found", Reason: "activity or recorded content not found"}
	}
	if err != nil {
		return bad(err.Error())
	}
	return result, nil
}
