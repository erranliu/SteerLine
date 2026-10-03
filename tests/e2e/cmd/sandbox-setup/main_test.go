package main

import (
	"errors"
	"io"
	"testing"
)

func TestSetupGateReusesValidStateAndKeepsChecksNoninteractive(t *testing.T) {
	for _, tc := range []struct {
		name     string
		check    bool
		readyErr error
		wantErr  bool
	}{
		{"already ready", false, nil, false},
		{"read-only failure", true, errors.New("missing setup"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := ensureSandbox("home", tc.check, func(string) error { return tc.readyErr }, func() error { calls++; return nil }, io.Discard)
			if (err != nil) != tc.wantErr || calls != 0 {
				t.Fatalf("unexpected elevation attempt: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestSetupGateProvisionsOnceAndRequiresPostcheck(t *testing.T) {
	for _, tc := range []struct {
		name         string
		provisionErr error
		postErr      error
		wantChecks   int
		wantErr      bool
	}{
		{"successful repair", nil, nil, 2, false},
		{"cancelled authorization", errors.New("cancelled"), nil, 1, true},
		{"unusable repaired state", nil, errors.New("logon failed"), 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checks, provisions := 0, 0
			err := ensureSandbox("home", false, func(string) error {
				checks++
				if checks == 1 {
					return errors.New("stale state")
				}
				return tc.postErr
			}, func() error { provisions++; return tc.provisionErr }, io.Discard)
			if checks != tc.wantChecks || provisions != 1 || (err != nil) != tc.wantErr {
				t.Fatalf("checks=%d provisions=%d err=%v", checks, provisions, err)
			}
		})
	}
}
