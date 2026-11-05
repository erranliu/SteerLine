package server

import (
	"bytes"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/billing"
	"net/http/httptest"
	"testing"
)

func TestBillingConfigIsIndependentOfRuntimeAndRequiresCSRF(t *testing.T) {
	s := &Server{options: Options{Home: t.TempDir(), Address: "127.0.0.1:12345"}}
	session := session{CSRF: "csrf"}
	read := httptest.NewRecorder()
	s.billingConfig(read, httptest.NewRequest("GET", "/api/web/billing-config", nil), session)
	if read.Code != 200 {
		t.Fatal(read.Code, read.Body.String())
	}
	// No client/Runner is configured: even a missing project must allow billing reads.
	var v map[string]any
	if json.Unmarshal(read.Body.Bytes(), &v) != nil || v["configured"] != false {
		t.Fatal(v)
	}
	p := billing.Policy{Source: "synthetic", Currency: "USD", Basis: billing.Basis, Rules: []billing.Rule{}}
	raw, _ := json.Marshal(p)
	req := httptest.NewRequest("PUT", "/api/web/billing-config", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	s.billingConfig(w, req, session)
	if w.Code != 403 {
		t.Fatal("write without CSRF accepted", w.Code)
	}
	req = httptest.NewRequest("PUT", "/api/web/billing-config", bytes.NewReader(raw))
	req.Header.Set("Origin", "http://127.0.0.1:12345")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "csrf")
	req.Header.Set("If-None-Match", "*")
	w = httptest.NewRecorder()
	s.billingConfig(w, req, session)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	req = httptest.NewRequest("PUT", "/api/web/billing-config", bytes.NewReader(raw))
	req.Header.Set("Origin", "http://127.0.0.1:12345")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "csrf")
	req.Header.Set("If-None-Match", "*")
	w = httptest.NewRecorder()
	s.billingConfig(w, req, session)
	if w.Code != 412 {
		t.Fatal("create overwrote existing settings", w.Code, w.Body.String())
	}
}
