package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"time"
)

func (p *testManagementPort) RegisterTestRecord(ctx context.Context, in api.TestRecordRegistration) (json.RawMessage, error) {
	r, err := testmanagement.ParseReport(in.Report)
	if err != nil {
		return nil, recordError(fmt.Errorf("%w: %v", tm.ErrInvalidRecord, err))
	}
	if canonicalConfigPath(r.Repository) != canonicalConfigPath(p.service.Project) {
		return nil, &api.Error{Kind: api.InvalidInput, Message: "report repository differs from selected project"}
	}
	v, err := p.records.Register(ctx, tm.RegisterRecord(in), tm.RecordSource{Kind: "script"})
	if err != nil {
		return nil, recordError(err)
	}
	return json.Marshal(testmanagement.ReceiptOf(v))
}
func recordError(err error) error {
	if errors.Is(err, tm.ErrInvalidRecord) {
		return &api.Error{Kind: api.InvalidInput, Message: err.Error()}
	}
	if errors.Is(err, tm.ErrRecordConflict) {
		return &api.Error{Kind: api.IdentityConflict, Message: err.Error()}
	}
	return generationError(err)
}
func (p *testManagementPort) TestRecordReceipt(ctx context.Context, in api.TestRecordRegistration) (json.RawMessage, error) {
	v, err := p.records.ReconcileRegistration(ctx, tm.RegisterRecord(in))
	if err != nil {
		return nil, recordError(err)
	}
	return json.Marshal(v)
}
func (p *testManagementPort) TestRecords(ctx context.Context, in api.TestRecordFilter) (json.RawMessage, error) {
	v, e := p.records.List(ctx, tm.RecordFilter(in))
	if e != nil {
		if errors.Is(e, testmanagement.ErrInvalidRecordFilter) {
			return nil, &api.Error{Kind: api.InvalidInput, Message: e.Error()}
		}
		return nil, e
	}
	return json.Marshal(v)
}
func (p *testManagementPort) TestRecord(ctx context.Context, id string) (json.RawMessage, error) {
	v, e := p.records.Detail(ctx, id)
	if e != nil {
		return nil, generationError(e)
	}
	return json.Marshal(v)
}
func (p *testManagementPort) TestRecordEvidence(ctx context.Context, id string) (json.RawMessage, error) {
	v, e := p.records.Summary(ctx, id)
	if e != nil {
		return nil, generationError(e)
	}
	return json.Marshal(v)
}
func (p *testManagementPort) TestCatalogue(ctx context.Context) (json.RawMessage, error) {
	type entry struct {
		tm.Definition
		Current bool `json:"current"`
	}
	out := struct {
		Tests      []entry   `json:"tests"`
		Commit     string    `json:"commit"`
		Path       string    `json:"path"`
		ObservedAt time.Time `json:"observed_at"`
		Diagnostic string    `json:"diagnostic,omitempty"`
	}{Tests: []entry{}, Path: tm.CataloguePath, ObservedAt: time.Now().UTC()}
	var definitions []tm.Definition
	o := p.maintenance.current()
	out.Commit = o.Commit
	if out.Commit == "" {
		out.Diagnostic = "主分支基线尚未核实"
	} else {
		raw, e := p.maintenance.read(ctx, out.Commit, tm.CataloguePath)
		if e != nil {
			out.Diagnostic = e.Error()
		} else {
			c, e := testmanagement.ParseCatalogue(raw)
			if e != nil {
				out.Diagnostic = e.Error()
			} else {
				definitions = c.Tests
			}
		}
	}
	seen := map[string]bool{}
	for _, d := range definitions {
		out.Tests = append(out.Tests, entry{d, true})
		seen[d.ID] = true
	}
	historic, e := p.store.TestRecordDefinitions(ctx, p.service.Project)
	if e != nil {
		return nil, e
	}
	for _, d := range historic {
		if !seen[d.ID] {
			out.Tests = append(out.Tests, entry{d, false})
		}
	}
	return json.Marshal(out)
}
