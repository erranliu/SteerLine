package api

import (
	"context"
	"encoding/json"
)

type TestRecordFilter struct {
	TestID      string `json:"test_id"`
	Search      string `json:"search"`
	Level       string `json:"level"`
	Outcome     string `json:"outcome"`
	Environment string `json:"environment"`
	Source      string `json:"source"`
	From        string `json:"from"`
	To          string `json:"to"`
	Cursor      string `json:"cursor"`
	Limit       int    `json:"limit"`
}
type TestRecordsPort interface {
	TestCatalogue(context.Context) (json.RawMessage, error)
	TestRecords(context.Context, TestRecordFilter) (json.RawMessage, error)
	TestRecord(context.Context, string) (json.RawMessage, error)
}
