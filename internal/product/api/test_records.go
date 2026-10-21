package api

import (
	"context"
	"encoding/json"
)

const RegisterTestRecord = "register_test_record"

type TestRecordRegistration struct {
	Report     json.RawMessage `json:"report"`
	Summary    []byte          `json:"summary,omitempty"`
	Diagnostic string          `json:"diagnostic,omitempty"`
}
type TestRecordFilter struct {
	TestID      string `json:"test_id"`
	Search      string `json:"search"`
	Level       string `json:"level"`
	Outcome     string `json:"outcome"`
	Retest      string `json:"retest"`
	Environment string `json:"environment"`
	Source      string `json:"source"`
	From        string `json:"from"`
	To          string `json:"to"`
	Cursor      string `json:"cursor"`
	Limit       int    `json:"limit"`
}
type TestRecordsPort interface {
	RegisterTestRecord(context.Context, TestRecordRegistration) (json.RawMessage, error)
	// TestRecordReceipt reconciles an original registration without mutation.
	TestRecordReceipt(context.Context, TestRecordRegistration) (json.RawMessage, error)
	TestCatalogue(context.Context) (json.RawMessage, error)
	TestRecords(context.Context, TestRecordFilter) (json.RawMessage, error)
	TestRecord(context.Context, string) (json.RawMessage, error)
	TestRecordEvidence(context.Context, string) (json.RawMessage, error)
}
