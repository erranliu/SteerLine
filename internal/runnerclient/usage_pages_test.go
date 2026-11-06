package runnerclient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func init() {
	if os.Getenv("STEERLINE_RUNNER_CLI_FIXTURE") != "usage-pages" {
		return
	}
	if len(os.Args) < 2 || os.Args[1] != "run-usage-events" {
		os.Exit(2)
	}
	cursor := ""
	for i, arg := range os.Args {
		if arg == "--after" && i+1 < len(os.Args) {
			cursor = os.Args[i+1]
		}
	}
	raw, err := os.ReadFile(os.Getenv("STEERLINE_USAGE_PAGES"))
	if err != nil {
		os.Exit(2)
	}
	var pages map[string]json.RawMessage
	if json.Unmarshal(raw, &pages) != nil || pages[cursor] == nil {
		os.Exit(2)
	}
	f, err := os.OpenFile(os.Getenv("STEERLINE_RUNNER_CLI_CALL"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(2)
	}
	_, _ = f.WriteString(strings.Join(os.Args[1:], "\n") + "\n")
	_ = f.Close()
	_, _ = os.Stdout.Write(pages[cursor])
	var response struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(pages[cursor], &response)
	if !response.OK {
		os.Exit(5)
	}
	os.Exit(0)
}

func TestRunUsageCompletePagesAndSummaryTruth(t *testing.T) {
	c := takeoverCLI(t)
	c.InstanceID, c.BuildVersion, c.ResourceDigest = "original-instance", "fixture", strings.Repeat("a", 64)
	c.FullBindingCommands = []string{"run-usage-events"}
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "usage-pages")
	file := filepath.Join(t.TempDir(), "pages.json")
	t.Setenv("STEERLINE_USAGE_PAGES", file)
	one, two, zero := int64(1), int64(2), int64(0)
	u := usageFixture()
	u.Coverage.CountedResponses = 2
	u.Tokens = &runnercontract.UsageTokens{InputTotal: &two, InputUncached: &two, CacheRead: &zero, CacheWrite: &zero, Output: &zero, Total: &two}
	first := runnercontract.UsageEvent{ID: "first", RunID: u.RunID, Decision: "counted", Tokens: runnercontract.UsageTokens{InputTotal: &one, InputUncached: &one, CacheRead: &zero, CacheWrite: &zero, Output: &zero, Total: &one}}
	second := first
	second.ID = "second"
	for _, mode := range []string{"complete", "sum-disagrees", "unknown-promoted", "snapshot-expired"} {
		t.Run(mode, func(t *testing.T) {
			summary := u
			summary.Tokens = new(runnercontract.UsageTokens)
			*summary.Tokens = *u.Tokens
			if mode == "sum-disagrees" {
				summary.Tokens.Output = &one
				three := int64(3)
				summary.Tokens.Total = &three
			}
			if mode == "unknown-promoted" {
				summary.Tokens.Reasoning = &zero
			}
			wrap := func(data any) any {
				return map[string]any{"protocol": Protocol, "ok": true, "request_id": nil, "data": data, "error": nil}
			}
			pages := map[string]any{"": wrap(runnercontract.UsagePage{RunID: u.RunID, Revision: u.Revision, Events: []runnercontract.UsageEvent{first}, NextCursor: "next"}), "next": wrap(runnercontract.UsagePage{RunID: u.RunID, Revision: u.Revision, Events: []runnercontract.UsageEvent{second}})}
			if mode == "snapshot-expired" {
				pages["next"] = map[string]any{"protocol": Protocol, "ok": false, "request_id": nil, "data": nil, "error": map[string]string{"code": "snapshot_expired", "message": "fixed snapshot expired"}}
			}
			raw, _ := json.Marshal(pages)
			if err := os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			_ = os.Remove(os.Getenv("STEERLINE_RUNNER_CLI_CALL"))
			events, err := c.RunUsageEvents(context.Background(), summary)
			if mode == "complete" {
				if err != nil || len(events) != 2 || events[1].ID != "second" {
					t.Fatal(events, err)
				}
			} else if err == nil || events != nil {
				t.Fatal("incomplete or contradictory set published", mode, events, err)
			}
			wire, _ := os.ReadFile(os.Getenv("STEERLINE_RUNNER_CLI_CALL"))
			if strings.Count(string(wire), "--snapshot-revision\n7\n") != 2 || strings.Count(string(wire), "--expected-instance-id\noriginal-instance\n") != 2 || !strings.Contains(string(wire), "--after\nnext\n") {
				t.Fatal("paging lost original snapshot/binding", string(wire))
			}
		})
	}
	_ = os.Remove(os.Getenv("STEERLINE_RUNNER_CLI_CALL"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if events, err := c.RunUsageEvents(ctx, u); err == nil || events != nil {
		t.Fatal("canceled query returned data", events, err)
	}
	if _, err := os.Stat(os.Getenv("STEERLINE_RUNNER_CLI_CALL")); !os.IsNotExist(err) {
		t.Fatal("canceled query invoked CLI", err)
	}
}
