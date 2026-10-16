package runnerclient_test

import (
	"context"
	"encoding/json"
	"errors"
	rc "github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os"
	"path/filepath"
	"reflect"
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
		if err != nil || v.Check.Result != "failed" {
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
	if rc.ValidateCheckSchema(f.Capabilities.Definition("input"), f.Input) != nil {
		t.Fatal("fixture schema differs")
	}
	f.Input.Execution.Settings = json.RawMessage(`{"executable":"codex","access_mode":"auto_review"}`)
	if f.Capabilities.Supports(f.Input.Execution) {
		t.Fatal("approval mode accepted")
	}
}

func TestEnvironmentCheckPublishedContract(t *testing.T) {
	exe := os.Getenv("STEERRUNNER_EXE")
	if exe == "" {
		t.Skip("published Runner check/v1 handoff not provided; fixture is not real integration evidence")
	}
	selection := rc.Selection{Executable: exe, Version: os.Getenv("STEERRUNNER_VERSION"), SHA256: os.Getenv("STEERRUNNER_SHA256")}
	c, version, openErr := selection.Open(context.Background(), t.TempDir(), 30*time.Second)
	if openErr != nil {
		t.Fatal("published EXE/version/SHA pin required", openErr)
	}
	info, err := c.StartService(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.StopService(context.Background(), info.InstanceID, "check-contract-stop"); err != nil {
			t.Error(err)
		}
	})
	cat, err := c.EnvironmentCheckCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cat.Capabilities.EnvironmentCheck == nil || cat.Capabilities.EnvironmentCheck.Contract != runnercontract.EnvironmentCheckContract {
		t.Fatal("published check/v1 missing")
	}
	f := checkFixture(t)
	caps := cat.Capabilities.EnvironmentCheck
	if !caps.Supports(f.Input.Execution) {
		t.Fatal("published local sandbox mechanism missing")
	}
	remote := f.Input.Execution
	remote.Adapter, remote.Target.Kind = "orca-terminal", "orca_environment"
	if caps.Supports(remote) {
		t.Fatal("unsupported Orca target advertised as usable")
	}
	for _, p := range []struct {
		schema json.RawMessage
		value  any
	}{{caps.Definition("input"), f.Input}, {caps.Definition("receipt"), f.Receipt}, {caps.Definition("inspection"), f.Inspection}} {
		if err = rc.ValidateCheckSchema(p.schema, p.value); err != nil {
			t.Fatal("published schema and caller fixture differ", err)
		}
	}
	frozen, err := os.ReadFile(filepath.Join("..", "testkit", "checkfixture", "published-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	assertSameJSON(t, frozen, cat.Capabilities.EnvironmentCheckFixtures)
	validatePublishedSamples(t, *caps, cat.Capabilities.EnvironmentCheckFixtures)
	t.Logf("public contract check using an isolated idle service: version=%s SHA=%s resource=%s; no native Agent or permission success implied", selection.Version, selection.SHA256, cat.Capabilities.ResourceDigest)
}

func assertSameJSON(t *testing.T, a, b []byte) {
	t.Helper()
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil || !reflect.DeepEqual(x, y) {
		t.Fatal("published wire document changed or lost fields")
	}
}

func validatePublishedSamples(t *testing.T, caps rc.CheckCapabilities, raw []byte) {
	t.Helper()
	var fixtures struct {
		Samples map[string]struct {
			Data json.RawMessage `json:"data"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	count := 0
	for name, sample := range fixtures.Samples {
		if !strings.HasPrefix(name, "inspect_") && name != "preparation_failure" && name != "rejection_without_check" && name != "unverified_original" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			var inspection rc.CheckInspection
			if err := json.Unmarshal(sample.Data, &inspection); err != nil {
				t.Fatal(err)
			}
			if err := rc.ValidateCheckSchema(caps.Definition("inspection"), inspection); err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(inspection)
			if err != nil {
				t.Fatal(err)
			}
			assertSameJSON(t, sample.Data, body)
		})
		count++
	}
	if count != 6 {
		t.Fatalf("expected six published inspection samples, got %d", count)
	}
}

func TestEnvironmentCheckPublishedSamples(t *testing.T) {
	f := checkFixture(t)
	raw, err := os.ReadFile(filepath.Join("..", "testkit", "checkfixture", "published-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	validatePublishedSamples(t, f.Capabilities, raw)
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
	f.Inspection.Check.Input = f.Input
	return f
}
