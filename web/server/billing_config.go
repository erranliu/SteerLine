package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/billingconfig"
)

func (s *Server) billingConfig(w http.ResponseWriter, r *http.Request, session session) {
	route := strings.TrimPrefix(r.URL.Path, "/api/web/billing-config")
	if r.URL.RawQuery != "" {
		fail(w, 404, "route_not_found", "canonical billing route required")
		return
	}
	if r.Method != "GET" && (r.Header.Get("Origin") != "http://"+s.options.Address || !isJSON(r) || !equal(r.Header.Get("X-CSRF-Token"), session.CSRF)) {
		fail(w, 403, "csrf_rejected", "same-origin JSON and session CSRF token required")
		return
	}
	store := billingconfig.Store{Home: s.options.Home}
	var result any
	var err error
	switch {
	case r.Method == "GET" && (route == "" || route == "/status"):
		var v billingconfig.Snapshot
		v, err = store.Read()
		result = v
		if err == nil {
			w.Header().Set("ETag", "\""+v.Digest+"\"")
			if route == "/status" {
				result = map[string]any{"saved_digest": v.Digest, "generation": v.Generation, "active_policy_version": v.Active}
			}
		}
	case route == "" && r.Method == "PUT" || route == "/validate" && r.Method == "POST":
		var p billing.Policy
		err = decode(w, r, &p, billingconfig.MaxBytes)
		if err == nil {
			if route == "/validate" {
				err = billingconfig.ValidateDraft(p)
				result = map[string]bool{"valid": err == nil}
			} else {
				expected := r.Header.Get("If-Match")
				if r.Header.Get("If-None-Match") == "*" {
					if expected != "" {
						fail(w, 400, "invalid_precondition", "use one conditional header")
						return
					}
					expected = "*"
				}
				result, err = store.Save(expected, p)
			}
		}
	case route == "/activate" && r.Method == "POST":
		var v struct {
			Version string `json:"version"`
		}
		err = decode(w, r, &v, 4096)
		if err == nil {
			result, err = store.Activate(r.Header.Get("If-Match"), v.Version)
		}
	default:
		fail(w, 404, "route_not_found", "unsupported billing configuration route")
		return
	}
	if err != nil {
		var e *billingconfig.Error
		if errors.As(err, &e) {
			fail(w, e.Status, e.Code, e.Message)
		} else {
			fail(w, 400, "billing_configuration_invalid", err.Error())
		}
		return
	}
	writeJSON(w, 200, result)
}
