// Package environmentcheck owns only the caller's durable requests, project
// binding and public Runner snapshots. It never drives a native worker.
package environmentcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

const StartAction = runnerstate.StartEnvironmentCheck
const StopAction = runnerstate.StopEnvironmentCheck

var requestPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var ErrNotFound = runnerstate.ErrEnvironmentCheckNotFound

type Error = runnerstate.EnvironmentCheckError

func failure(code, message string) error { return &Error{Code: code, Message: message} }

type StartPayload = runnerstate.EnvironmentCheckStartPayload
type Command = runnerstate.EnvironmentCheckCommand
type Operation = runnerstate.EnvironmentCheckOperation
type Record = runnerstate.EnvironmentCheckRecord
type Profile = runnerstate.EnvironmentCheckProfile
type Catalog = runnerstate.EnvironmentCheckCatalog

type Store interface {
	Check(context.Context, string, string) (Record, error)
	CheckRequest(context.Context, string, string) (Record, error)
	Checks(context.Context, string) ([]Record, error)
	SaveCheck(context.Context, Record, uint64) error
}
type Runner interface {
	EnvironmentCheckCatalog(context.Context) (*runnerclient.Catalog, error)
	StartEnvironmentCheck(context.Context, runnerclient.Request) (runnerclient.CheckReceipt, error)
	StopEnvironmentCheck(context.Context, runnerclient.Request) (runnerclient.CheckReceipt, error)
	InspectEnvironmentCheck(context.Context, string, string) (runnerclient.CheckInspection, error)
}
type Service struct {
	Project      string
	Selection    runnerclient.Selection
	Store        Store
	Profiles     []Profile
	Open         func(context.Context, runnerclient.Selection) (Runner, error)
	ResolveInput func(context.Context, runnerclient.ExecutionInput) (runnerclient.CheckInput, error)
	// Admit verifies the project's original Runner instance. It never starts
	// a service or checks Task/DAG eligibility.
	Admit func(context.Context) error
	mu    sync.Mutex
}

func (s *Service) Catalog(ctx context.Context) Catalog {
	result := Catalog{Profiles: append([]Profile{}, s.Profiles...)}
	r, err := s.Open(ctx, s.Selection)
	if err == nil {
		var cat *runnerclient.Catalog
		cat, err = r.EnvironmentCheckCatalog(ctx)
		if err == nil {
			if cat.Capabilities.EnvironmentCheck != nil {
				value := runnercontract.CheckCapabilities(*cat.Capabilities.EnvironmentCheck)
				result.Capabilities = &value
			}
		}
	}
	if err != nil {
		result.UnavailableReason = diagnostic(err)
	} else if result.Capabilities == nil || result.Capabilities.Contract != runnercontract.EnvironmentCheckContract {
		result.UnavailableReason = "所选 Runner 未发布环境检查 v1 能力；现有任务执行方案不代表支持环境检查"
	}
	return result
}
func (s *Service) List(ctx context.Context) ([]Record, error) { return s.Store.Checks(ctx, s.Project) }
func (s *Service) Detail(ctx context.Context, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.Store.Check(ctx, s.Project, id)
	if err != nil {
		return r, err
	}
	return s.refresh(ctx, r, "")
}
func (s *Service) Request(ctx context.Context, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.Store.CheckRequest(ctx, s.Project, id)
	if err != nil {
		return r, err
	}
	return s.refresh(ctx, r, id)
}
func (s *Service) Submit(ctx context.Context, c Command) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !requestPattern.MatchString(c.RequestID) || len(c.Original) == 0 || !json.Valid(c.Original) {
		return Record{}, failure("invalid_request", "immutable request envelope required")
	}
	old, err := s.Store.CheckRequest(ctx, s.Project, c.RequestID)
	if err == nil {
		for _, op := range old.Operations {
			if op.RequestID == c.RequestID && !equalJSON(op.Original, c.Original) {
				return old, failure("request_conflict", "request ID already belongs to another immutable envelope")
			}
		}
		return s.refresh(ctx, old, c.RequestID)
	}
	if !errors.Is(err, ErrNotFound) {
		return Record{}, err
	}
	if c.Action == StartAction {
		return s.start(ctx, c)
	}
	if c.Action != StopAction {
		return Record{}, failure("invalid_request", "unknown environment check action")
	}
	r, err := s.Store.Check(ctx, s.Project, c.TargetID)
	if err != nil {
		return r, err
	}
	if r.Report == nil || r.CheckID == "" || r.Report.Revision != c.ExpectedRevision || !r.Report.CanStop() {
		return r, failure("stale_revision", "refresh the original check before explicitly requesting stop/cleanup")
	}
	for _, op := range r.Operations {
		if op.Action == StopAction && op.Status != "rejected" {
			var previous runnerclient.CheckStop
			if op.Status == "outcome_unknown" || json.Unmarshal(op.Request.Input, &previous) != nil || previous.ExpectedRevision >= c.ExpectedRevision {
				return r, failure("operation_unverified", "inspect the original stop request and wait for a newer cleanup snapshot")
			}
		}
	}
	if err := s.admit(ctx); err != nil {
		return r, err
	}
	runner, err := s.Open(ctx, runnerclient.Selection(r.Selection))
	if err != nil {
		return r, failure("runner_unavailable", diagnostic(err))
	}
	body, _ := json.Marshal(runnerclient.CheckStop{CheckID: r.CheckID, ExpectedRevision: c.ExpectedRevision})
	op := Operation{RequestID: c.RequestID, Action: c.Action, Original: c.Original, Request: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: c.RequestID, Input: body}, Status: "outcome_unknown"}
	r.Operations = append(r.Operations, op)
	if err = s.save(ctx, &r); err != nil {
		return r, err
	}
	receipt, callErr := runner.StopEnvironmentCheck(ctx, op.Request)
	return s.received(ctx, r, receipt, callErr)
}
func (s *Service) start(ctx context.Context, c Command) (Record, error) {
	if c.TargetID != s.Project || c.ExpectedRevision != 0 {
		return Record{}, failure("invalid_request", "check start must target this project with version zero")
	}
	var payload StartPayload
	decoder := json.NewDecoder(bytes.NewReader(c.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || payload.AccessMode != "sandbox" && payload.AccessMode != "full_access" || payload.Execution == nil && payload.ProfileID == "" || payload.Execution != nil && payload.ProfileID != "" {
		return Record{}, failure("invalid_request", "explicit sandbox or full_access required")
	}
	rows, err := s.Store.Checks(ctx, s.Project)
	if err != nil {
		return Record{}, err
	}
	for _, row := range rows {
		last := row.Operations[0]
		if last.Status == "rejected" {
			continue
		}
		if row.Report == nil || row.Report.CleanupStatus != "completed" {
			return Record{}, failure("operation_unverified", "an earlier check or its resources remain unverified; inspect it before creating another")
		}
		for _, op := range row.Operations {
			if op.Status == "outcome_unknown" {
				return Record{}, failure("operation_unverified", "an original check operation remains unverified")
			}
		}
	}
	var execution runnerclient.ExecutionInput
	if payload.Execution != nil {
		execution = *payload.Execution
	} else {
		for _, p := range s.Profiles {
			if p.ID == payload.ProfileID {
				execution = p.Execution
				break
			}
		}
	}
	var settings map[string]any
	if execution.Adapter == "" || json.Unmarshal(execution.Settings, &settings) != nil || settings == nil {
		return Record{}, failure("invalid_request", "a published execution configuration is required")
	}
	settings["access_mode"] = payload.AccessMode
	execution.Settings, _ = json.Marshal(settings)
	if err := s.admit(ctx); err != nil {
		return Record{}, err
	}
	r, err := s.Open(ctx, s.Selection)
	if err != nil {
		return Record{}, failure("runner_unavailable", diagnostic(err))
	}
	cat, err := r.EnvironmentCheckCatalog(ctx)
	if err != nil {
		return Record{}, failure("runner_unavailable", diagnostic(err))
	}
	caps := cat.Capabilities.EnvironmentCheck
	if !caps.Supports(execution) {
		return Record{}, failure("unsupported_capability", "Runner does not advertise this target/access mode for environment checks")
	}
	input, err := s.ResolveInput(ctx, execution)
	if err != nil {
		return Record{}, failure("input_unavailable", diagnostic(err))
	}
	if err = runnerclient.ValidateCheckSchema(caps.Definition("input"), input); err != nil {
		return Record{}, failure("invalid_request", "input does not match the selected Runner check schema")
	}
	if len(caps.Definition("receipt")) == 0 || len(caps.Definition("inspection")) == 0 {
		return Record{}, failure("unsupported_capability", "Runner check response schemas are missing")
	}

	body, _ := json.Marshal(input)
	record := Record{ID: c.RequestID, Project: s.Project, Selection: runnerstate.EnvironmentRunnerPin(s.Selection), ResourceDigest: cat.Capabilities.ResourceDigest, Input: input, Capabilities: runnercontract.CheckCapabilities(*caps), CreatedAt: time.Now().UTC(), Operations: []Operation{{RequestID: c.RequestID, Action: c.Action, Original: c.Original, Request: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: c.RequestID, Input: body}, Status: "outcome_unknown"}}}
	if err = s.save(ctx, &record); err != nil {
		return record, err
	}
	receipt, callErr := r.StartEnvironmentCheck(ctx, record.Operations[0].Request)
	return s.received(ctx, record, receipt, callErr)
}

func (s *Service) admit(ctx context.Context) error {
	if s.Admit == nil {
		return failure("runner_unavailable", "project Runner instance admission is not bound")
	}
	if err := s.Admit(ctx); err != nil {
		return failure("runner_unavailable", diagnostic(err))
	}
	return nil
}
func (s *Service) save(ctx context.Context, r *Record) error {
	expected := r.Revision
	r.Revision++
	if err := s.Store.SaveCheck(ctx, *r, expected); err != nil {
		r.Revision = expected
		return err
	}
	return nil
}
func (s *Service) received(ctx context.Context, r Record, receipt runnerclient.CheckReceipt, callErr error) (Record, error) {
	op := &r.Operations[len(r.Operations)-1]
	if callErr != nil {
		op.Diagnostic = diagnostic(callErr)
		var f *runnerclient.Failure
		if errors.As(callErr, &f) && !f.Unknown {
			op.Status = "rejected"
		}
	} else if receipt.CheckID == "" || receipt.CommandID == "" || receipt.ReceiptStatus != "accepted" || r.CheckID != "" && r.CheckID != receipt.CheckID || runnerclient.ValidateCheckSchema(r.Capabilities.Definition("receipt"), receipt) != nil {
		op.Diagnostic = "Runner receipt identity or public schema is unverified; inspect the original request"
	} else {
		r.CheckID = receipt.CheckID
		op.CommandID = receipt.CommandID
		op.Status = "applied"
		op.Diagnostic = ""
	}
	err := s.save(ctx, &r)
	return r, err
}

// A request read reconciles exactly that original intent. A detail read resolves
// the one unknown operation, then uses the bound check for subsequent snapshots.
// Accepted historical commands are not dependencies of unrelated stop recovery.
func (s *Service) refresh(ctx context.Context, r Record, requestID string) (Record, error) {
	if requestID == "" {
		for _, op := range r.Operations {
			if op.Status == "outcome_unknown" {
				requestID = op.RequestID
				break
			}
		}
	}
	if requestID != "" {
		for _, op := range r.Operations {
			if op.RequestID == requestID && op.Status == "rejected" {
				return r, nil
			}
		}
	} else if r.CheckID == "" {
		return r, nil
	}
	before := mustJSON(r)
	runner, err := s.Open(ctx, runnerclient.Selection(r.Selection))
	if err == nil {
		selector, id := "check-id", r.CheckID
		if requestID != "" {
			selector, id = "request-id", requestID
		}
		var inspection runnerclient.CheckInspection
		inspection, err = runner.InspectEnvironmentCheck(ctx, selector, id)
		if err == nil {
			err = applyInspection(&r, inspection, requestID)
		}
	}
	r.Diagnostic = diagnostic(err)
	if !bytes.Equal(before, mustJSON(r)) {
		if e := s.save(ctx, &r); e != nil {
			return r, e
		}
	}
	return r, nil // Cached source snapshot remains readable while Runner is offline.
}

func applyInspection(r *Record, inspection runnerclient.CheckInspection, requestID string) error {
	var document any = inspection
	if len(inspection.Raw) != 0 {
		document = inspection.Raw
	}
	if runnerclient.ValidateCheckSchema(r.Capabilities.Definition("inspection"), document) != nil {
		return errors.New("Runner check inspection violates the frozen schema")
	}
	if requestID == "" {
		if inspection.Check == nil || inspection.Check.CheckID != r.CheckID {
			return errors.New("bound check snapshot is missing or has another identity")
		}
		if err := validateReport(*r, *inspection.Check); err != nil {
			return err
		}
		r.Report = inspection.Check
		return nil
	}
	for i := range r.Operations {
		op := &r.Operations[i]
		if op.RequestID != requestID {
			continue
		}
		cmd := inspection.Command
		want := "environment-check start"
		if op.Action == StopAction {
			want = "environment-check stop"
		}
		if cmd == nil || cmd.RequestID != requestID || cmd.Command != want || cmd.CommandID == "" || op.CommandID != "" && op.CommandID != cmd.CommandID || r.CheckID != "" && cmd.CheckID != "" && r.CheckID != cmd.CheckID {
			return errors.New("original check command identity changed")
		}
		if cmd.ReceiptStatus == "rejected" {
			if op.Status == "applied" {
				return errors.New("accepted command receipt changed to rejected")
			}
			op.CommandID, op.Status, op.Diagnostic = cmd.CommandID, "rejected", commandDiagnostic(cmd)
			return nil
		}
		if cmd.ReceiptStatus != "accepted" || cmd.CheckID == "" {
			return errors.New("original command acceptance remains unverified")
		}
		if inspection.Check != nil {
			if inspection.Check.CheckID != cmd.CheckID {
				return errors.New("command and check identity differ")
			}
			if err := validateReport(*r, *inspection.Check); err != nil {
				return err
			}
			r.Report = inspection.Check
		}
		r.CheckID = cmd.CheckID
		op.CommandID, op.Status, op.Diagnostic = cmd.CommandID, "applied", ""
		return nil
	}
	return errors.New("original request is not bound to this check")
}
func validateReport(r Record, v runnerclient.Check) error {
	if v.CheckID == "" || r.CheckID != "" && v.CheckID != r.CheckID || v.Contract != runnercontract.EnvironmentCheckContract || v.Revision == 0 || !digestPattern.MatchString(v.InputDigest) || v.ResourceDigest != r.ResourceDigest || v.UpdatedAt.IsZero() || v.CreatedAt.IsZero() || v.Status == "" || v.Actual.AttemptID == "" || v.Actual.Generation == "" || len(v.Items) == 0 || !equalJSON(mustJSON(v.Input), mustJSON(r.Input)) {
		return errors.New("check snapshot is incomplete or belongs to another input/resource")
	}
	if prior := r.Report; prior != nil {
		if v.InputDigest != prior.InputDigest || v.Revision < prior.Revision || v.UpdatedAt.Before(prior.UpdatedAt) || !v.CreatedAt.Equal(prior.CreatedAt) || !v.ExecutionDeadline.Equal(prior.ExecutionDeadline) || !v.CleanupDeadline.Equal(prior.CleanupDeadline) {
			return errors.New("check snapshot digest or revision regressed")
		}
		if v.Revision == prior.Revision && !equalJSON(mustJSON(v), mustJSON(prior)) {
			return errors.New("conflicting check snapshot at the same revision")
		}
		if !reflect.DeepEqual(checkManifest(prior.Items), checkManifest(v.Items)) {
			return errors.New("frozen required check manifest changed")
		}
		before, after := reflect.ValueOf(prior.Actual), reflect.ValueOf(v.Actual)
		for i := 0; i < before.NumField(); i++ {
			if !before.Field(i).IsZero() && !reflect.DeepEqual(before.Field(i).Interface(), after.Field(i).Interface()) {
				return errors.New("actual check context changed")
			}
		}
		for _, pair := range [][2]string{{prior.Resources.BaseRef, v.Resources.BaseRef}, {prior.Resources.HeadRef, v.Resources.HeadRef}, {prior.Resources.BaseSHA, v.Resources.BaseSHA}, {prior.Resources.HeadSHA, v.Resources.HeadSHA}, {prior.Resources.PRURL, v.Resources.PRURL}} {
			if pair[0] != "" && pair[0] != pair[1] {
				return errors.New("check resource identity changed")
			}
		}
		if prior.Resources.PRNumber != 0 && prior.Resources.PRNumber != v.Resources.PRNumber {
			return errors.New("check PR identity changed")
		}
	}
	seen := map[string]bool{}
	for _, item := range v.Items {
		if item.ID == "" || seen[item.ID] {
			return errors.New("duplicate check item")
		}
		seen[item.ID] = true
		switch item.Status {
		case "passed", "failed", "unverified", "not_applicable":
		default:
			return errors.New("unknown check item status")
		}
		if item.Required && (item.Status == "not_applicable" || v.Result == "passed" && item.Status != "passed") {
			return errors.New("passed summary conflicts with required checks")
		}
	}
	switch v.Result {
	case "passed", "failed", "unverified":
	default:
		return errors.New("unknown check conclusion")
	}
	switch v.CleanupStatus {
	case "pending", "completed", "residual":
	default:
		return errors.New("resource settlement is missing")
	}
	if v.Resources.PRURL != "" && (r.Input.Publication == nil || v.Resources.BaseRef != "refs/heads/steerline-check/"+v.CheckID+"/base" || v.Resources.HeadRef != "refs/heads/steerline-check/"+v.CheckID+"/head") {
		return errors.New("test PR identity differs from check-owned branches")
	}
	if v.Result == "passed" && r.Input.Publication != nil && v.Resources.PRURL == "" {
		return errors.New("publication-required check lacks its test PR fact")
	}
	return nil
}
func checkManifest(items []runnercontract.CheckItem) map[string]bool {
	manifest := map[string]bool{}
	for _, item := range items {
		manifest[item.ID] = item.Required
	}
	return manifest
}
func commandDiagnostic(cmd *runnercontract.CheckCommand) string {
	if cmd.Error != nil {
		return cmd.Error.Code + ": " + cmd.Error.Message
	}
	return ""
}
func equalJSON(a, b []byte) bool {
	var x, y any
	left, right := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	left.UseNumber()
	right.UseNumber()
	return left.Decode(&x) == nil && right.Decode(&y) == nil && reflect.DeepEqual(x, y)
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func diagnostic(err error) string {
	if err == nil {
		return ""
	}
	s := fmt.Sprint(err)
	if len(s) > 2048 {
		s = s[:2048]
	}
	return s
}
