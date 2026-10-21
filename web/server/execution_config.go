package server

import (
	"context"
	"encoding/json"
	"errors"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/web/server/discovery"
	configrepo "github.com/erranliu/SteerLine/web/server/executionconfig"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) executionConfig(w http.ResponseWriter, r *http.Request, session session) {
	const base = "/api/web/execution-config"
	route := strings.TrimPrefix(r.URL.Path, base)
	if r.URL.RawQuery != "" || route != "" && route != "/discover" && route != "/validate" && route != "/status" {
		fail(w, 404, "route_not_found", "unknown execution configuration route")
		return
	}
	writeOK := (r.Method == "POST" || r.Method == "PUT") && r.Header.Get("Origin") == "http://"+s.options.Address && isJSON(r)
	if r.Method != "GET" && (!writeOK || !equal(r.Header.Get("X-CSRF-Token"), session.CSRF)) {
		fail(w, 403, "csrf_rejected", "same-origin JSON and session CSRF token required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	result := any(nil)
	var err error
	if r.Method == "GET" && route == "/status" {
		result, err = s.configuration.Status()
	} else if r.Method == "GET" && route == "" {
		var snapshot configrepo.Snapshot
		snapshot, err = s.configuration.Snapshot(ctx)
		result = snapshot
		if err == nil {
			w.Header().Set("ETag", "\""+snapshot.SavedDigest+"\"")
		}
	} else if r.Method == "POST" && route == "/discover" {
		var q discovery.Request
		var body []byte
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err == nil {
			err = cfg.Decode(body, &q)
		}
		if err == nil {
			result, err = s.discovery.Query(ctx, q)
		}
	} else if r.Method == "POST" && route == "/validate" || r.Method == "PUT" && route == "" {
		var raw []byte
		reader := http.MaxBytesReader(w, r.Body, 1<<20)
		defer reader.Close()
		raw, err = io.ReadAll(reader)
		if err == nil {
			if r.Method == "PUT" {
				result, err = s.configuration.Save(ctx, r.Header.Get("If-Match"), raw)
			} else {
				var config configrepo.Config
				config, err = s.configuration.Validate(ctx, raw)
				result = map[string]any{"config": config}
			}
		}
	} else {
		fail(w, 405, "method_not_allowed", "unsupported execution configuration method")
		return
	}
	if err != nil {
		var e *configrepo.Error
		var large *http.MaxBytesError
		switch {
		case errors.As(err, &e):
			fail(w, e.Status, e.Code, e.Message)
		case errors.As(err, &large):
			fail(w, 413, "request_body_too_large", "request exceeds 1 MiB")
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			fail(w, 504, "discovery_timeout", "能力查询超时或已取消")
		default:
			fail(w, 400, "configuration_request_failed", err.Error())
		}
		return
	}
	raw, marshalErr := json.Marshal(result)
	if marshalErr != nil || len(raw) > 1<<20 {
		fail(w, 413, "configuration_result_too_large", "configuration result exceeds 1 MiB")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}
