//go:build runneracceptance

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/billingconfig"
	"github.com/erranliu/SteerLine/internal/runnerclient"
)

// The Runner's tagged fixture owner prepares this isolated ledger. This caller
// reads it exclusively through the pinned public CLI, never through its database.
func TestBillingUsagePublishedFixture(t *testing.T) {
	root := os.Getenv("STEERRUNNER_BILLING_FIXTURE")
	if root == "" || !strings.HasPrefix(strings.ToLower(filepath.Clean(root)), `c:\steerlinedebug\`) {
		t.Fatal("explicit isolated Runner-owned billing fixture required")
	}
	marker, err := os.ReadFile(filepath.Join(root, "fixture.json"))
	require(t, err)
	if string(marker) != `{"schema":"billing-acceptance/v1","runs":["priced","completion"]}` {
		t.Fatal("unrecognized fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	selection := runnerclient.Selection{Executable: os.Getenv("STEERRUNNER_EXE"), Version: os.Getenv("STEERRUNNER_VERSION"), SHA256: os.Getenv("STEERRUNNER_SHA256")}
	cli, version, err := selection.Open(ctx, filepath.Join(root, "state"), 30*time.Second)
	require(t, err)
	cli.UsagePolicy = json.RawMessage(`{"enabled":false}`)
	info, err := cli.StartService(ctx, version)
	require(t, err)
	cli.InstanceID = info.InstanceID
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		stopErr := cli.StopService(cleanupCtx, info.InstanceID, "billing-fixture-stop")
		settledErr := cli.VerifyServiceSettlement(cleanupCtx, info.InstanceID, "billing-fixture-stop")
		if stopErr != nil || settledErr != nil {
			t.Error("fixture cleanup", stopErr, settledErr)
		}
		body, _ := json.Marshal(map[string]bool{"verified": stopErr == nil && settledErr == nil})
		if e := os.WriteFile(filepath.Join(root, "cleanup.json"), body, 0600); e != nil {
			t.Error(e)
		}
	})
	catalog, err := cli.Catalog(ctx, version)
	require(t, err)
	cli = cli.WithCatalog(catalog)
	if !cli.RunUsageSupported || !cli.RunUsageEventsSupported {
		t.Fatal("usage facts contract missing")
	}
	home := filepath.Join(root, "home")
	policy := billing.Policy{Source: "synthetic acceptance rates, not a vendor quote", Currency: "USD", Basis: billing.Basis, Rules: []billing.Rule{{ID: "fixture", Provider: "synthetic", Model: "fixture", From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Rates: billing.Rates{Input: "1", Read: "0.1", Write: "3", Output: "2"}}}}
	settings, err := (billingconfig.Store{Home: home}).Save("*", policy)
	require(t, err)
	for _, run := range []string{"priced", "completion"} {
		view, readErr := readRunnerUsage(ctx, cli, "candidate-"+run, run, home)
		require(t, readErr)
		if view.Usage == nil || view.Usage.Tokens.Total == nil || *view.Usage.Tokens.Total != 120 || view.Cost == nil || view.Cost.Generation != settings.Generation {
			t.Fatal("public facts lost", view)
		}
		if run == "priced" {
			// 100 uncached input at 1 + 20 output at 2, per million.
			if view.Cost.Status != "estimated" || view.Cost.Total == nil || *view.Cost.Total != "0.000140000000000000" {
				t.Fatal("public fixture price mismatch", view.Cost)
			}
		} else if view.Cost.Status != "unavailable" || view.Cost.Total != nil || !containsString(view.Cost.Issues, "PRICE_PERIOD_UNKNOWN") {
			t.Fatal("completion time became a request time", view.Cost)
		}
		body, _ := json.MarshalIndent(view, "", "  ")
		require(t, os.WriteFile(filepath.Join(root, run+"-view.json"), body, 0600))
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
