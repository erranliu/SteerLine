package runnerclient_test

import (
	"context"
	"encoding/json"
	"errors"
	rc "github.com/erranliu/SteerLine/internal/runnerclient"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvironmentCheckCLIContract(t *testing.T) {
	ctx := context.Background()
	exe, _ := os.Executable()
	path := filepath.Join(t.TempDir(), "argv")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", path)
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "check-response")
	c := rc.Client{Executable: exe, StateDir: t.TempDir(), Timeout: 5 * time.Second}
	f := checkFixture(t)
	response := func(data any, query bool) {
		b, _ := json.Marshal(data)
		id := `"__REQUEST_ID__"`
		if query {
			id = "null"
		}
		t.Setenv("STEERLINE_CHECK_RESPONSE", `{"protocol":"steerrunner-cli/v1","ok":true,"request_id":`+id+`,"data":`+string(b)+`,"error":null}`)
	}
	body, _ := json.Marshal(f.Input)
	q := rc.Request{Protocol: rc.Protocol, RequestID: strings.Repeat("1", 32), Input: body}
	for _, action := range []string{"start", "stop"} {
		response(f.Receipt, false)
		var err error
		if action == "start" {
			_, err = c.StartEnvironmentCheck(ctx, q)
		} else {
			q.Input = json.RawMessage(`{"check_id":"check-fixture","expected_revision":1}`)
			_, err = c.StopEnvironmentCheck(ctx, q)
		}
		if err != nil {
			t.Fatal(err)
		}
		argv, _ := os.ReadFile(path)
		if !strings.HasPrefix(string(argv), "environment-check\n"+action+"\n") || !strings.Contains(string(argv), "--input\n-\n") {
			t.Fatal(string(argv))
		}
	}
	for _, selector := range []string{"request-id", "check-id"} {
		id := f.Inspection.Check.CheckID
		if selector == "request-id" {
			id = f.Inspection.Command.RequestID
		}
		inspection := f.Inspection
		if selector == "check-id" {
			inspection.Command = nil
		}
		response(inspection, true)
		v, err := c.InspectEnvironmentCheck(ctx, selector, id)
		if err != nil || v.Check.CapabilityStatus != "failed" {
			t.Fatal("check failure misclassified as transport error", v, err)
		}
	}
	rejected := f.Inspection
	rejected.Check = nil
	rejected.Command.ReceiptStatus = "rejected"
	rejected.Command.CheckID = ""
	response(rejected, true)
	if _, err := c.InspectEnvironmentCheck(ctx, "request-id", rejected.Command.RequestID); err != nil {
		t.Fatal("rejected without Check lost", err)
	}
	response(f.Receipt, true)
	if _, err := c.StartEnvironmentCheck(ctx, q); err == nil || strings.Contains(err.Error(), "retry") {
		t.Fatal("mutation accepted null outer request id")
	}
	t.Setenv("STEERLINE_CHECK_ERROR", "1")
	t.Setenv("STEERLINE_CHECK_RESPONSE", `{"protocol":"steerrunner-cli/v1","ok":false,"request_id":null,"data":null,"error":{"code":"check_not_found","message":"missing"}}`)
	_, err := c.InspectEnvironmentCheck(ctx, "check-id", "missing")
	var failure *rc.Failure
	if !errors.As(err, &failure) || failure.Exit != 5 || failure.Unknown {
		t.Fatal(err)
	}
	if rc.ValidateCheckSchema(f.Capabilities.InputSchema, f.Input) != nil {
		t.Fatal("fixture schema differs")
	}
	f.Input.Execution.Settings = json.RawMessage(`{"executable":"codex","access_mode":"auto_review"}`)
	if rc.ValidateCheckSchema(f.Capabilities.InputSchema, f.Input) == nil {
		t.Fatal("approval mode accepted")
	}
}

func TestEnvironmentCheckPublishedContract(t *testing.T) {
	exe := os.Getenv("STEERRUNNER_EXE")
	if exe == "" {
		t.Skip("published Runner check/v1 handoff not provided; fixture is not real integration evidence")
	}
	selection := rc.Selection{Executable: exe, Version: os.Getenv("STEERRUNNER_VERSION"), SHA256: os.Getenv("STEERRUNNER_SHA256")}
	c, _, openErr := selection.Open(context.Background(), t.TempDir(), 10*time.Second)
	if openErr != nil {
		t.Fatal("published EXE/version/SHA pin required", openErr)
	}
	cat, err := c.EnvironmentCheckCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cat.Capabilities.EnvironmentCheck == nil || cat.Capabilities.EnvironmentCheck.Version != 1 {
		t.Fatal("published check/v1 missing")
	}
	f := checkFixture(t)
	caps := cat.Capabilities.EnvironmentCheck
	for _, p := range []struct {
		schema json.RawMessage
		value  any
	}{{caps.InputSchema, f.Input}, {caps.ReceiptSchema, f.Receipt}, {caps.InspectionSchema, f.Inspection}} {
		if err = rc.ValidateCheckSchema(p.schema, p.value); err != nil {
			t.Fatal("published handoff and provisional fixture differ", err)
		}
	}
	t.Logf("read-only public contract check: version=%s SHA=%s resource=%s; no native Agent or permission success implied", selection.Version, selection.SHA256, cat.Capabilities.ResourceDigest)
}

func checkFixture(t *testing.T) (f struct {
	Capabilities rc.CheckCapabilities `json:"capabilities"`
	Input        rc.CheckInput        `json:"input"`
	Receipt      rc.CheckReceipt      `json:"receipt"`
	Inspection   rc.CheckInspection   `json:"inspection"`
}) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "testkit", "checkfixture", "v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(body, &f); err != nil {
		t.Fatal(err)
	}
	f.Inspection.Check.Context = f.Input
	return f
}
